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
			go AcceptConnection(conn, server)
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
