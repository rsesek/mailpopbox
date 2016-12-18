package smtp

import (
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

type testServer struct {
	EmptyServerCallbacks
	blockList []string
}

func (s *testServer) Name() string {
	return "Test-Server"
}

func (s *testServer) VerifyAddress(addr mail.Address) ReplyLine {
	for _, block := range s.blockList {
		if block == addr.Address {
			return ReplyBadMailbox
		}
	}
	return ReplyOK
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
		{"MAIL FROM:<sender@foo.com>", 250, nil},
		{"RCPT TO:<banned.net>", 501, nil},
		{"QUIT", 221, nil},
	})
}

func TestCaseSensitivty(t *testing.T) {
	s := &testServer{}
	l := runServer(t, s)
	defer l.Close()

	conn := createClient(t, l.Addr())
	readCodeLine(t, conn, 220)

	runTableTest(t, conn, []requestResponse{
		{"nOoP", 250, nil},
		{"ehLO test.TEST", 0, func(t testing.TB, conn *textproto.Conn) { conn.ReadResponse(250) }},
		{"mail FROM:<sender@example.com>", 250, nil},
		{"RcPT tO:<receive@mail.com>", 250, nil},
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
