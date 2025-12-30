// mailpopbox
// Copyright 2025 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package pop3

import (
	"io"
	"net"
	"testing"

	"go.uber.org/zap"
)

// RFC 1939 § 10
func TestClientExampleSession(t *testing.T) {
	s := newTestServer()
	l := runServer(t, s)
	defer l.Close()

	s.mb.msgs[1] = &testMessage{1, 120, false, ""}
	s.mb.msgs[2] = &testMessage{2, 200, false, ""}

	dc, err := net.Dial(l.Addr().Network(), l.Addr().String())
	ok(t, err)

	c, err := Connect(dc, zap.L())
	ok(t, err)

	mb, err := c.OpenMailbox("u", "p")
	ok(t, err)

	msgs, err := mb.ListMessages()
	ok(t, err)
	if want, got := 2, len(msgs); want != got {
		t.Errorf("Expected %d messages, got %d", want, got)
	}

	if want, got := 1, msgs[0].ID(); want != got {
		t.Errorf("Expected message ID %d, got %d", want, got)
	}
	if want, got := 120, msgs[0].Size(); want != got {
		t.Errorf("Expected message size %d, got %d", want, got)
	}

	if want, got := 2, msgs[1].ID(); want != got {
		t.Errorf("Expected message ID %d, got %d", want, got)
	}
	if want, got := 200, msgs[1].Size(); want != got {
		t.Errorf("Expected message size %d, got %d", want, got)
	}

	ok(t, mb.Close())
}

func TestClientRetrieve(t *testing.T) {
	s := newTestServer()
	l := runServer(t, s)
	defer l.Close()

	body := `This is a test message.
<html>It contains HTML</html>

and ------
---.
.
Boundary items
`

	s.mb.msgs[1] = &testMessage{1, len(body), false, body}

	dc, err := net.Dial(l.Addr().Network(), l.Addr().String())
	ok(t, err)

	c, err := Connect(dc, zap.L())
	ok(t, err)

	mb, err := c.OpenMailbox("u", "p")
	ok(t, err)

	msgs, err := mb.ListMessages()
	ok(t, err)
	if want, got := 1, len(msgs); want != got {
		t.Errorf("Expected %d messages, got %d", want, got)
	}

	rc, err := mb.Retrieve(msgs[0])
	ok(t, err)

	got, err := io.ReadAll(rc)
	ok(t, err)
	rc.Close()

	if string(got) != body {
		t.Errorf("Expected body %q, got %q", body, string(got))
	}

	ok(t, mb.Close())
}

func TestClientErrors(t *testing.T) {
	s := newTestServer()
	l := runServer(t, s)
	defer l.Close()

	s.mb.msgs[1] = &testMessage{1, 12, false, "hello world"}

	dc, err := net.Dial(l.Addr().Network(), l.Addr().String())
	ok(t, err)

	c, err := Connect(dc, zap.L())
	ok(t, err)

	mb, err := c.OpenMailbox("bad", "p")
	if mb != nil || err == nil {
		t.Errorf("Expected error, got %v %v", mb, err)
	}

	mb, err = c.OpenMailbox("u", "bad")
	if mb != nil || err == nil {
		t.Errorf("Expected error, got %v %v", mb, err)
	}

	mb, err = c.OpenMailbox("u", "p")
	ok(t, err)

	_, err = c.OpenMailbox("bad", "x")
	if err == nil {
		t.Errorf("Shouldn't reopen mailbox")
	}

	msg := mb.GetMessage(100)
	if msg != nil {
		t.Errorf("Should have failed to get message")
	}

	msgs, err := mb.ListMessages()
	ok(t, err)

	msg = msgs[0]
	if msg.Deleted() {
		t.Errorf("Expected message to not be marked as deleted")
	}

	ok(t, mb.Delete(msg))

	if !msg.Deleted() {
		t.Errorf("Expected message to be marked as deleted")
	}

	body, err := mb.Retrieve(msg)
	if body != nil || err == nil {
		t.Errorf("Expected error, got %v %v", msg, err)
	}
	msg2 := mb.GetMessage(1)
	if msg2 != nil {
		t.Errorf("Shouldn't get deleted message")
	}

	mb.Reset()
	if msg.Deleted() {
		t.Errorf("Expected message to not be marked as deleted")
	}

	msg2 = mb.GetMessage(1)
	if msg2 == nil {
		t.Errorf("Failed to get message")
	}

	body, err = mb.Retrieve(msg)
	ok(t, err)
}
