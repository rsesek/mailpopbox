package smtp

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/mail"
	"net/textproto"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/uber-go/zap"
)

func _fl(depth int) string {
	_, file, line, _ := runtime.Caller(depth + 1)
	return fmt.Sprintf("[%s:%d]", filepath.Base(file), line)
}

func ok(t testing.TB, err error) {
	if err != nil {
		t.Errorf("%s unexpected error: %v", _fl(1), err)
	}
}

func readCodeLine(t testing.TB, conn *textproto.Conn, code int) string {
	_, message, err := conn.ReadCodeLine(code)
	if err != nil {
		t.Errorf("%s ReadCodeLine error: %v", _fl(1), err)
	}
	return message
}

// runServer creates a TCP socket, runs a listening server, and returns the connection.
// The server exits when the Conn is closed.
func runServer(t *testing.T, server Server) net.Listener {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
		return nil
	}

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go AcceptConnection(conn, server, zap.New(zap.NullEncoder()))
		}
	}()

	return l
}

type userAuth struct {
	authz, authc, passwd string
}

type testServer struct {
	EmptyServerCallbacks
	domain    string
	blockList []string
	tlsConfig *tls.Config
	*userAuth
	relayed []Envelope
}

func (s *testServer) Name() string {
	return "Test-Server"
}

func (s *testServer) TLSConfig() *tls.Config {
	return s.tlsConfig
}

func (s *testServer) VerifyAddress(addr mail.Address) ReplyLine {
	if DomainForAddress(addr) != s.domain {
		return ReplyBadMailbox
	}
	for _, block := range s.blockList {
		if strings.ToLower(block) == addr.Address {
			return ReplyBadMailbox
		}
	}
	return ReplyOK
}

func (s *testServer) Authenticate(authz, authc, passwd string) bool {
	return s.userAuth.authz == authz &&
		s.userAuth.authc == authc &&
		s.userAuth.passwd == passwd
}

func (s *testServer) RelayMessage(en Envelope) {
	s.relayed = append(s.relayed, en)
}

func createClient(t *testing.T, addr net.Addr) *textproto.Conn {
	conn, err := textproto.Dial(addr.Network(), addr.String())
	if err != nil {
		t.Fatal(err)
		return nil
	}
	return conn
}

type requestResponse struct {
	request      string
	responseCode int
	handler      func(testing.TB, *textproto.Conn)
}

func runTableTest(t testing.TB, conn *textproto.Conn, seq []requestResponse) {
	for i, rr := range seq {
		t.Logf("%s case %d", _fl(1), i)
		ok(t, conn.PrintfLine(rr.request))
		if rr.handler != nil {
			rr.handler(t, conn)
		} else {
			readCodeLine(t, conn, rr.responseCode)
		}
	}
}

// RFC 5321 § D.1
func TestScenarioTypical(t *testing.T) {
	s := testServer{
		domain:    "foo.com",
		blockList: []string{"Green@foo.com"},
	}
	l := runServer(t, &s)
	defer l.Close()

	conn := createClient(t, l.Addr())

	message := readCodeLine(t, conn, 220)
	if !strings.HasPrefix(message, s.Name()) {
		t.Errorf("Greeting does not have server name, got %q", message)
	}

	greet := "greeting.TestScenarioTypical"
	ok(t, conn.PrintfLine("EHLO "+greet))

	_, message, err := conn.ReadResponse(250)
	ok(t, err)
	if !strings.Contains(message, greet) {
		t.Errorf("EHLO response does not contain greeting, got %q", message)
	}

	ok(t, conn.PrintfLine("MAIL FROM:<Smith@bar.com>"))
	readCodeLine(t, conn, 250)

	ok(t, conn.PrintfLine("RCPT TO:<Jones@foo.com>"))
	readCodeLine(t, conn, 250)

	ok(t, conn.PrintfLine("RCPT TO:<Green@foo.com>"))
	readCodeLine(t, conn, 550)

	ok(t, conn.PrintfLine("RCPT TO:<Brown@foo.com>"))
	readCodeLine(t, conn, 250)

	ok(t, conn.PrintfLine("DATA"))
	readCodeLine(t, conn, 354)

	ok(t, conn.PrintfLine("Blah blah blah..."))
	ok(t, conn.PrintfLine("...etc. etc. etc."))
	ok(t, conn.PrintfLine("."))
	readCodeLine(t, conn, 250)

	ok(t, conn.PrintfLine("QUIT"))
	readCodeLine(t, conn, 221)
}

func TestVerifyAddress(t *testing.T) {
	s := testServer{
		domain:    "test.mail",
		blockList: []string{"banned@test.mail"},
	}
	l := runServer(t, &s)
	defer l.Close()

	conn := createClient(t, l.Addr())
	readCodeLine(t, conn, 220)

	runTableTest(t, conn, []requestResponse{
		{"EHLO test", 0, func(t testing.TB, conn *textproto.Conn) { conn.ReadResponse(250) }},
		{"VRFY banned@test.mail", 252, nil},
		{"VRFY allowed@test.mail", 252, nil},
		{"MAIL FROM:<sender@example.com>", 250, nil},
		{"RCPT TO:<banned@test.mail>", 550, nil},
		{"QUIT", 221, nil},
	})
}

func TestBadAddress(t *testing.T) {
	l := runServer(t, &testServer{})
	defer l.Close()

	conn := createClient(t, l.Addr())
	readCodeLine(t, conn, 220)

	runTableTest(t, conn, []requestResponse{
		{"EHLO test", 0, func(t testing.TB, conn *textproto.Conn) { conn.ReadResponse(250) }},
		{"MAIL FROM:<sender>", 501, nil},
		{"MAIL FROM:<sender@foo.com> SIZE=2163", 250, nil},
		{"RCPT TO:<banned.net>", 501, nil},
		{"QUIT", 221, nil},
	})
}

func TestCaseSensitivty(t *testing.T) {
	s := &testServer{
		domain:    "mail.com",
		blockList: []string{"reject@mail.com"},
	}
	l := runServer(t, s)
	defer l.Close()

	conn := createClient(t, l.Addr())
	readCodeLine(t, conn, 220)

	runTableTest(t, conn, []requestResponse{
		{"nOoP", 250, nil},
		{"ehLO test.TEST", 0, func(t testing.TB, conn *textproto.Conn) { conn.ReadResponse(250) }},
		{"mail FROM:<sender@example.com>", 250, nil},
		{"RcPT tO:<receive@mail.com>", 250, nil},
		{"RCPT TO:<reject@MAIL.com>", 550, nil},
		{"RCPT TO:<reject@mail.com>", 550, nil},
		{"DATa", 0, func(t testing.TB, conn *textproto.Conn) {
			readCodeLine(t, conn, 354)

			ok(t, conn.PrintfLine("."))
			readCodeLine(t, conn, 250)
		}},
		{"MAIL FR:", 501, nil},
		{"QUiT", 221, nil},
	})
}

func TestGetReceivedInfo(t *testing.T) {
	conn := connection{
		server:     &testServer{},
		remoteAddr: &net.IPAddr{net.IPv4(127, 0, 0, 1), ""},
	}

	now := time.Now()

	const crlf = "\r\n"
	const line1 = "Received: from remote.test. (localhost [127.0.0.1])" + crlf
	const line2 = "by Test-Server (mailpopbox) with "
	const msgId = "abcdef.hijk"
	lineLast := now.Format(time.RFC1123Z) + crlf

	type params struct {
		ehlo    string
		esmtp   bool
		tls     bool
		address string
	}

	tests := []struct {
		params params

		expect []string
	}{
		{params{"remote.test.", true, false, "foo@bar.com"},
			[]string{line1,
				line2 + "ESMTP id " + msgId + crlf,
				"for <foo@bar.com>" + crlf,
				"(using PLAINTEXT);" + crlf,
				lineLast, ""}},
	}

	for _, test := range tests {
		t.Logf("%#v", test.params)

		conn.ehlo = test.params.ehlo
		conn.esmtp = test.params.esmtp
		//conn.tls = test.params.tls

		envelope := Envelope{
			RcptTo:   []mail.Address{{"", test.params.address}},
			Received: now,
			ID:       msgId,
		}

		actual := conn.getReceivedInfo(envelope)
		actualLines := strings.SplitAfter(string(actual), crlf)

		if len(actualLines) != len(test.expect) {
			t.Errorf("wrong numbber of lines, expected %d, got %d", len(test.expect), len(actualLines))
			continue
		}

		for i, line := range actualLines {
			expect := test.expect[i]
			if expect != strings.TrimLeft(line, " ") {
				t.Errorf("Expected equal string %q, got %q", expect, line)
			}
		}
	}

}

func getTLSConfig(t *testing.T) *tls.Config {
	cert, err := tls.LoadX509KeyPair("../testtls/domain.crt", "../testtls/domain.key")
	if err != nil {
		t.Fatal(err)
		return nil
	}
	return &tls.Config{
		ServerName:         "localhost",
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,
	}
}

func setupTLSClient(t *testing.T, addr net.Addr) *textproto.Conn {
	nc, err := net.Dial(addr.Network(), addr.String())
	ok(t, err)

	conn := textproto.NewConn(nc)
	readCodeLine(t, conn, 220)

	ok(t, conn.PrintfLine("EHLO test-tls"))
	_, resp, err := conn.ReadResponse(250)
	ok(t, err)
	if !strings.Contains(resp, "STARTTLS\n") {
		t.Errorf("STARTTLS not advertised")
	}

	ok(t, conn.PrintfLine("STARTTLS"))
	readCodeLine(t, conn, 220)

	tc := tls.Client(nc, getTLSConfig(t))
	err = tc.Handshake()
	ok(t, err)

	conn = textproto.NewConn(tc)

	ok(t, conn.PrintfLine("EHLO test-tls-started"))
	_, resp, err = conn.ReadResponse(250)
	ok(t, err)
	if strings.Contains(resp, "STARTTLS\n") {
		t.Errorf("STARTTLS advertised when already started")
	}

	return conn
}

func b64enc(s string) string {
	return string(base64.StdEncoding.EncodeToString([]byte(s)))
}

func TestTLS(t *testing.T) {
	l := runServer(t, &testServer{tlsConfig: getTLSConfig(t)})
	defer l.Close()

	setupTLSClient(t, l.Addr())
}

func TestAuthWithoutTLS(t *testing.T) {
	l := runServer(t, &testServer{})
	defer l.Close()

	conn := createClient(t, l.Addr())
	readCodeLine(t, conn, 220)

	ok(t, conn.PrintfLine("EHLO test"))
	_, resp, err := conn.ReadResponse(250)
	ok(t, err)

	if strings.Contains(resp, "AUTH") {
		t.Errorf("AUTH should not be advertised over plaintext")
	}
}

func TestAuth(t *testing.T) {
	l := runServer(t, &testServer{
		tlsConfig: getTLSConfig(t),
		userAuth: &userAuth{
			authz:  "-authz-",
			authc:  "-authc-",
			passwd: "goats",
		},
	})
	defer l.Close()

	conn := setupTLSClient(t, l.Addr())

	runTableTest(t, conn, []requestResponse{
		{"AUTH", 501, nil},
		{"AUTH OAUTHBEARER", 504, nil},
		{"AUTH PLAIN", 334, nil},
		{b64enc("abc\x00def\x00ghf"), 535, nil},
		{"AUTH PLAIN", 334, nil},
		{b64enc("\x00"), 501, nil},
		{"AUTH PLAIN", 334, nil},
		{"this isn't base 64", 501, nil},
		{"AUTH PLAIN", 334, nil},
		{b64enc("-authz-\x00-authc-\x00goats"), 250, nil},
		{"AUTH PLAIN", 503, nil}, // already authenticated
		{"NOOP", 250, nil},
	})
}

func TestRelayRequiresAuth(t *testing.T) {
	l := runServer(t, &testServer{
		domain:    "example.com",
		tlsConfig: getTLSConfig(t),
		userAuth: &userAuth{
			authz:  "",
			authc:  "mailbox@example.com",
			passwd: "test",
		},
	})
	defer l.Close()

	conn := setupTLSClient(t, l.Addr())

	runTableTest(t, conn, []requestResponse{
		{"MAIL FROM:<apples@example.com>", 550, nil},
		{"MAIL FROM:<mailbox@example.com>", 550, nil},
		{"AUTH PLAIN", 334, nil},
		{b64enc("\x00mailbox@example.com\x00test"), 250, nil},
		{"MAIL FROM:<mailbox@example.com>", 250, nil},
	})
}

func setupRelayTest(t *testing.T) (server *testServer, l net.Listener, conn *textproto.Conn) {
	server = &testServer{
		domain:    "example.com",
		tlsConfig: getTLSConfig(t),
		userAuth: &userAuth{
			authz:  "",
			authc:  "mailbox@example.com",
			passwd: "test",
		},
	}
	l = runServer(t, server)
	conn = setupTLSClient(t, l.Addr())
	runTableTest(t, conn, []requestResponse{
		{"AUTH PLAIN", 334, nil},
		{b64enc("\x00mailbox@example.com\x00test"), 250, nil},
	})
	return
}

func TestBasicRelay(t *testing.T) {
	server, l, conn := setupRelayTest(t)
	defer l.Close()

	runTableTest(t, conn, []requestResponse{
		{"MAIL FROM:<mailbox@example.com>", 250, nil},
		{"RCPT TO:<dest@another.net>", 250, nil},
		{"DATA", 354, func(t testing.TB, conn *textproto.Conn) {
			readCodeLine(t, conn, 354)

			ok(t, conn.PrintfLine("From: <mailbox@example.com>"))
			ok(t, conn.PrintfLine("To: <dest@example.com>"))
			ok(t, conn.PrintfLine("Subject: Basic relay\n"))
			ok(t, conn.PrintfLine("This is a basic relay message"))
			ok(t, conn.PrintfLine("."))
			readCodeLine(t, conn, 250)
		}},
	})

	if len(server.relayed) != 1 {
		t.Errorf("Expected 1 relayed message, got %d", len(server.relayed))
	}
}

func TestNoInternalRelays(t *testing.T) {
	_, l, conn := setupRelayTest(t)
	defer l.Close()

	runTableTest(t, conn, []requestResponse{
		{"MAIL FROM:<mailbox@example.com>", 250, nil},
		{"RCPT TO:<valid@dest.xyz>", 250, nil},
		{"RCPT TO:<dest@example.com>", 550, nil},
		{"RCPT TO:<mailbox@example.com>", 550, nil},
	})
}

func TestSendAsRelay(t *testing.T) {
	server, l, conn := setupRelayTest(t)
	defer l.Close()

	runTableTest(t, conn, []requestResponse{
		{"MAIL FROM:<mailbox@example.com>", 250, nil},
		{"RCPT TO:<valid@dest.xyz>", 250, nil},
		{"RCPT TO:<sendas+source@example.com>", 250, nil},
		{"RCPT TO:<mailbox@example.com>", 550, nil},
		{"DATA", 354, func(t testing.TB, conn *textproto.Conn) {
			readCodeLine(t, conn, 354)

			ok(t, conn.PrintfLine("From: <mailbox@example.com>"))
			ok(t, conn.PrintfLine("To: <valid@dest.xyz>"))
			ok(t, conn.PrintfLine("Subject: Send-as relay\n"))
			ok(t, conn.PrintfLine("We've switched the senders!"))
			ok(t, conn.PrintfLine("."))
			readCodeLine(t, conn, 250)
		}},
	})

	if len(server.relayed) != 1 {
		t.Errorf("Expected 1 relayed message, got %d", len(server.relayed))
	}

	replaced := "source@example.com"
	original := "mailbox@example.com"

	en := server.relayed[0]
	if en.MailFrom.Address != replaced {
		t.Errorf("Expected mail to be from %q, got %q", replaced, en.MailFrom.Address)
	}

	if len(en.RcptTo) != 1 {
		t.Errorf("Expected 1 recipient, got %d", len(en.RcptTo))
	}
	if en.RcptTo[0].Address != "valid@dest.xyz" {
		t.Errorf("Unexpected RcptTo %q", en.RcptTo[0].Address)
	}

	msg := string(en.Data)

	if strings.Index(msg, original) != -1 {
		t.Errorf("Should not find %q in message %q", original, msg)
	}

	if strings.Index(msg, "\nFrom: <source@example.com>\n") == -1 {
		t.Errorf("Could not find From: header in message %q", msg)
	}
}

func TestSendMultipleRelay(t *testing.T) {
	server, l, conn := setupRelayTest(t)
	defer l.Close()

	runTableTest(t, conn, []requestResponse{
		{"MAIL FROM:<mailbox@example.com>", 250, nil},
		{"RCPT TO:<valid@dest.xyz>", 250, nil},
		{"RCPT TO:<sendas+source@example.com>", 250, nil},
		{"RCPT TO:<another@dest.org>", 250, nil},
		{"DATA", 354, func(t testing.TB, conn *textproto.Conn) {
			readCodeLine(t, conn, 354)

			ok(t, conn.PrintfLine("To: Cindy <valid@dest.xyz>, Sam <another@dest.org>"))
			ok(t, conn.PrintfLine("From: <mailbox@example.com>"))
			ok(t, conn.PrintfLine("Subject: Two destinations\n"))
			ok(t, conn.PrintfLine("And we've switched the senders!"))
			ok(t, conn.PrintfLine("."))
			readCodeLine(t, conn, 250)
		}},
	})

	if len(server.relayed) != 1 {
		t.Errorf("Expected 1 relayed message, got %d", len(server.relayed))
	}

	replaced := "source@example.com"
	original := "mailbox@example.com"

	en := server.relayed[0]
	if en.MailFrom.Address != replaced {
		t.Errorf("Expected mail to be from %q, got %q", replaced, en.MailFrom.Address)
	}

	if len(en.RcptTo) != 2 {
		t.Errorf("Expected 2 recipient, got %d", len(en.RcptTo))
	}
	if en.RcptTo[0].Address != "valid@dest.xyz" {
		t.Errorf("Unexpected RcptTo %q", en.RcptTo[0].Address)
	}

	msg := string(en.Data)

	if strings.Index(msg, original) != -1 {
		t.Errorf("Should not find %q in message %q", original, msg)
	}

	if strings.Index(msg, "\nFrom: <source@example.com>\n") == -1 {
		t.Errorf("Could not find From: header in message %q", msg)
	}
}
