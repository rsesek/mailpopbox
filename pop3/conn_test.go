// mailpopbox
// Copyright 2020 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package pop3

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/textproto"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"go.uber.org/zap"
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

func responseOK(t testing.TB, conn *textproto.Conn) string {
	line, err := conn.ReadLine()
	if err != nil {
		t.Errorf("%s responseOK: %v", _fl(1), err)
	}
	if !strings.HasPrefix(line, "+OK") {
		t.Errorf("%s expected +OK, got %q", _fl(1), line)
	}
	return line
}

func responseERR(t testing.TB, conn *textproto.Conn) string {
	line, err := conn.ReadLine()
	if err != nil {
		t.Errorf("%s responseERR: %v", _fl(1), err)
	}
	if !strings.HasPrefix(line, "-ERR") {
		t.Errorf("%s expected -ERR, got %q", _fl(1), line)
	}
	return line
}

func runServer(t *testing.T, po PostOffice) net.Listener {
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
			go AcceptConnection(conn, po, zap.New(zap.NullEncoder()))
		}
	}()
	return l
}

type testServer struct {
	user, pass string
	mb         testMailbox
}

func (s *testServer) Name() string {
	return "Test-Server"
}

func (s *testServer) OpenMailbox(user, pass string) (Mailbox, error) {
	if s.user == user && s.pass == pass {
		return &s.mb, nil
	}
	return nil, fmt.Errorf("bad username/pass")
}

type testMailbox struct {
	msgs map[int]*testMessage
}

type MessageList []Message

func (l MessageList) Len() int {
	return len(l)
}
func (l MessageList) Less(i, j int) bool {
	return l[i].ID() < l[j].ID()
}
func (l MessageList) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

func (mb *testMailbox) ListMessages() ([]Message, error) {
	msgs := make([]Message, 0, len(mb.msgs))
	for i, _ := range mb.msgs {
		msgs = append(msgs, mb.msgs[i])
	}
	sort.Sort(MessageList(msgs))
	return msgs, nil
}

func (mb *testMailbox) GetMessage(id int) Message {
	if msg, ok := mb.msgs[id]; ok {
		return msg
	}
	return nil
}

func (mb *testMailbox) Retrieve(msg Message) (io.ReadCloser, error) {
	r := strings.NewReader(msg.(*testMessage).body)
	return ioutil.NopCloser(r), nil
}

func (mb *testMailbox) Delete(msg Message) error {
	msg.(*testMessage).deleted = true
	return nil
}

func (mb *testMailbox) Close() error {
	return nil
}

func (mb *testMailbox) Reset() {
	for _, msg := range mb.msgs {
		msg.deleted = false
	}
}

type testMessage struct {
	id      int
	size    int
	deleted bool
	body    string
}

func (m *testMessage) UniqueID() string {
	return fmt.Sprintf("%p", m)
}

func (m *testMessage) ID() int {
	return m.id
}
func (m *testMessage) Size() int {
	return m.size
}
func (m *testMessage) Deleted() bool {
	return m.deleted
}

func newTestServer() *testServer {
	return &testServer{
		user: "u",
		pass: "p",
		mb: testMailbox{
			msgs: make(map[int]*testMessage),
		},
	}
}

// RFC 1939 § 10
func TestExampleSession(t *testing.T) {
	s := newTestServer()
	l := runServer(t, s)
	defer l.Close()

	s.mb.msgs[1] = &testMessage{1, 120, false, ""}
	s.mb.msgs[2] = &testMessage{2, 200, false, ""}

	conn, err := textproto.Dial(l.Addr().Network(), l.Addr().String())
	ok(t, err)

	line := responseOK(t, conn)
	if !strings.Contains(line, s.Name()) {
		t.Errorf("POP greeting did not include server name, got %q", line)
	}

	ok(t, conn.PrintfLine("USER u"))
	responseOK(t, conn)

	ok(t, conn.PrintfLine("PASS p"))
	responseOK(t, conn)

	ok(t, conn.PrintfLine("STAT"))
	line = responseOK(t, conn)
	expected := "+OK 2 320"
	if line != expected {
		t.Errorf("STAT expected %q, got %q", expected, line)
	}

	ok(t, conn.PrintfLine("LIST"))
	responseOK(t, conn)
	lines, err := conn.ReadDotLines()
	ok(t, err)
	if len(lines) != 2 {
		t.Errorf("LIST expected 2 lines, got %d", len(lines))
	}
	expected = "1 120"
	if lines[0] != expected {
		t.Errorf("LIST line 0 expected %q, got %q", expected, lines[0])
	}
	expected = "2 200"
	if lines[1] != expected {
		t.Errorf("LIST line 1 expected %q, got %q", expected, lines[1])
	}

	ok(t, conn.PrintfLine("QUIT"))
	responseOK(t, conn)
}

type requestResponse struct {
	command  string
	expecter func(testing.TB, *textproto.Conn) string
}

func expectOKResponse(predicate func(string) bool) func(testing.TB, *textproto.Conn) string {
	return func(t testing.TB, conn *textproto.Conn) string {
		line := responseOK(t, conn)
		if !predicate(line) {
			t.Errorf("%s Predicate failed, got %q", _fl(1), line)
		}
		return line
	}
}

func clientServerTest(t *testing.T, s *testServer, sequence []requestResponse) {
	l := runServer(t, s)
	defer l.Close()

	conn, err := textproto.Dial(l.Addr().Network(), l.Addr().String())
	ok(t, err)

	responseOK(t, conn)

	for _, pair := range sequence {
		ok(t, conn.PrintfLine(pair.command))
		pair.expecter(t, conn)
		if t.Failed() {
			t.Logf("command %q", pair.command)
		}
	}
}

func TestAuthStates(t *testing.T) {
	clientServerTest(t, newTestServer(), []requestResponse{
		{"STAT", responseERR},
		{"NOOP", responseOK},
		{"USER bad", responseOK},
		{"PASS bad", responseERR},
		{"USER", responseERR},
		{"USER x", responseOK},
		{"PASS", responseERR},
		{"LIST", responseERR},
		{"USER u", responseOK},
		{"PASS bad", responseERR},
		{"STAT", responseERR},
		{"PASS p", responseOK},
		{"QUIT", responseOK},
	})
}

func TestDeleted(t *testing.T) {
	s := newTestServer()
	s.mb.msgs[1] = &testMessage{1, 999, false, ""}
	s.mb.msgs[2] = &testMessage{2, 10, false, ""}

	clientServerTest(t, s, []requestResponse{
		{"USER u", responseOK},
		{"PASS p", responseOK},
		{"STAT", expectOKResponse(func(s string) bool {
			return s == "+OK 2 1009"
		})},
		{"DELE 1", responseOK},
		{"RETR 1", responseERR},
		{"DELE 1", responseERR},
		{"STAT", expectOKResponse(func(s string) bool {
			return s == "+OK 1 10"
		})},
		{"RSET", responseOK},
		{"STAT", expectOKResponse(func(s string) bool {
			return s == "+OK 2 1009"
		})},
		{"QUIT", responseOK},
	})
}

func TestCaseSensitivty(t *testing.T) {
	s := newTestServer()
	s.mb.msgs[999] = &testMessage{999, 1, false, "a"}

	clientServerTest(t, s, []requestResponse{
		{"user u", responseOK},
		{"PasS p", responseOK},
		{"sTaT", responseOK},
		{"retr 1", responseERR},
		{"dele 999", responseOK},
		{"QUIT", responseOK},
	})
}

func TestRetr(t *testing.T) {
	s := newTestServer()
	s.mb.msgs[1] = &testMessage{1, 5, false, "hello"}
	s.mb.msgs[2] = &testMessage{2, 69, false, "this\r\nis a\r\n.\r\ntest"}

	clientServerTest(t, s, []requestResponse{
		{"USER u", responseOK},
		{"PASS p", responseOK},
		{"STAT", responseOK},
		{"RETR 1", func(t testing.TB, tp *textproto.Conn) string {
			responseOK(t, tp)
			if t.Failed() {
				return ""
			}

			resp, err := tp.ReadDotLines()
			if err != nil {
				t.Error(err)
				return ""
			}

			expected := []string{"hello"}
			if !reflect.DeepEqual(resp, expected) {
				t.Errorf("Expected %v, got %v", expected, resp)
			}

			return ""
		}},
		{"RETR 2", func(t testing.TB, tp *textproto.Conn) string {
			responseOK(t, tp)
			if t.Failed() {
				return ""
			}

			resp, err := tp.ReadDotLines()
			if err != nil {
				t.Error(err)
				return ""
			}

			expected := []string{"this", "is a", ".", "test"}
			if !reflect.DeepEqual(resp, expected) {
				t.Errorf("Expected %v, got %v", expected, resp)
			}

			return ""
		}},
		{"QUIT", responseOK},
	})
}

func TestUidl(t *testing.T) {
	s := newTestServer()
	s.mb.msgs[1] = &testMessage{1, 3, false, "abc"}
	s.mb.msgs[2] = &testMessage{2, 1, true, "Z"}
	s.mb.msgs[3] = &testMessage{3, 4, false, "test"}

	clientServerTest(t, s, []requestResponse{
		{"USER u", responseOK},
		{"PASS p", responseOK},
		{"UIDL", func(t testing.TB, tp *textproto.Conn) string {
			responseOK(t, tp)
			if t.Failed() {
				return ""
			}

			resp, err := tp.ReadDotLines()
			if err != nil {
				t.Error(err)
				return ""
			}

			expected := []string{
				fmt.Sprintf("1 %p", s.mb.msgs[1]),
				fmt.Sprintf("3 %p", s.mb.msgs[3]),
			}
			if !reflect.DeepEqual(resp, expected) {
				t.Errorf("Expected %v, got %v", expected, resp)
			}

			return ""
		}},
		{"QUIT", responseOK},
	})
}

func TestDele(t *testing.T) {
	s := newTestServer()
	s.mb.msgs[1] = &testMessage{1, 3, false, "abc"}
	s.mb.msgs[2] = &testMessage{2, 1, false, "d"}

	clientServerTest(t, s, []requestResponse{
		{"USER u", responseOK},
		{"PASS p", responseOK},
		{"STAT", expectOKResponse(func(s string) bool {
			return s == "+OK 2 4"
		})},
		{"DELE 1", responseOK},
		{"STAT", expectOKResponse(func(s string) bool {
			return s == "+OK 1 1"
		})},
		{"RSET", responseOK},
		{"STAT", expectOKResponse(func(s string) bool {
			return s == "+OK 2 4"
		})},
		{"QUIT", responseOK},
	})

	if s.mb.msgs[1].Deleted() || s.mb.msgs[2].Deleted() {
		t.Errorf("RSET should not delete a message")
	}

	clientServerTest(t, s, []requestResponse{
		{"USER u", responseOK},
		{"PASS p", responseOK},
		{"STAT", expectOKResponse(func(s string) bool {
			return s == "+OK 2 4"
		})},
		{"DELE 1", responseOK},
		{"STAT", expectOKResponse(func(s string) bool {
			return s == "+OK 1 1"
		})},
		{"QUIT", responseOK},
	})

	if !s.mb.msgs[1].Deleted() {
		t.Errorf("DELE did not work")
	}
	if s.mb.msgs[2].Deleted() {
		t.Errorf("DELE the wrong message")
	}
}

func TestCapa(t *testing.T) {
	s := newTestServer()

	capaTest := func(t testing.TB, tp *textproto.Conn) string {
		responseOK(t, tp)
		if t.Failed() {
			return ""
		}

		resp, err := tp.ReadDotLines()
		if err != nil {
			t.Error(err)
			return ""
		}

		const (
			capNeeded = iota
			capSeen
			capOK
		)

		caps := map[string]int{
			"USER": capNeeded,
			"UIDL": capNeeded,
		}
		for _, line := range resp {
			if val, ok := caps[line]; ok {
				if val == capNeeded {
					caps[line] = capOK
				} else {
					t.Errorf("unxpected capa value %q", line)
				}
			} else {
				caps[line] = capSeen
			}
		}
		for c, val := range caps {
			if val != capOK {
				t.Errorf("unexpected capa value for %q: %d", c, val)
			}
		}
		return ""
	}

	clientServerTest(t, s, []requestResponse{
		{"CAPA", capaTest},
		{"USER u", responseOK},
		{"CAPA", capaTest},
		{"PASS p", responseOK},
		{"CAPA", capaTest},
		{"QUIT", responseOK},
	})
}
