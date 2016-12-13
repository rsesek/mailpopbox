package smtp

import (
	"net"
	"net/textproto"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func ok(t testing.TB, err error) {
	if err != nil {
		_, file, line, _ := runtime.Caller(1)
		t.Errorf("[%s:%d] unexpected error: %v", filepath.Base(file), line, err)
	}
}

func readCodeLine(t testing.TB, conn *textproto.Conn, code int) string {
	_, message, err := conn.ReadCodeLine(code)
	ok(t, err)
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
}

func (s *testServer) Name() string {
	return "Test-Server"
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
	s := testServer{}
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
	readCodeLine(t, conn, 250) // TODO: make this 55o by rejecting Green

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
