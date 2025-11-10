// mailpopbox
// Copyright 2025 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package pop3

import (
	"fmt"
	"io"
	"net"
	"net/textproto"
	"strings"

	"go.uber.org/zap"
)

type serverConn struct {
	name     string
	tp       *textproto.Conn
	log      *zap.Logger
	loggedIn bool
	deleted  map[int]struct{}
}

// Connect connects to a POP3 server and returns the `PostOffice` for accesing
// mailboxes.
func Connect(nc net.Conn, log *zap.Logger) (PostOffice, error) {
	log = log.With(zap.Stringer("address", nc.RemoteAddr()))
	conn := &serverConn{
		tp:      textproto.NewConn(nc),
		log:     log,
		deleted: make(map[int]struct{}),
	}
	var err error
	conn.name, err = conn.readReplyLine()
	if err != nil {
		return nil, fmt.Errorf("Failed to open connection: %w", err)
	}
	return conn, nil
}

func (sc *serverConn) Name() string {
	return sc.name
}

func (sc *serverConn) OpenMailbox(user, pass string) (Mailbox, error) {
	if sc.loggedIn {
		return nil, fmt.Errorf("Mailbox is already open")
	}
	if _, err := sc.transaction("USER %s", user); err != nil {
		return nil, err
	}
	if _, err := sc.transaction("PASS %s", pass); err != nil {
		return nil, err
	}
	sc.log.Info("Opened mailbox")
	sc.loggedIn = true
	return sc, nil
}

func (sc *serverConn) transaction(fmt string, args ...any) (string, error) {
	log := sc.log.With(zap.String("command", fmt))
	log.Debug("Sending transaction")
	if err := sc.tp.PrintfLine(fmt, args...); err != nil {
		log.Error("Failed to send command")
		return "", err
	}
	reply, err := sc.readReplyLine()
	if err != nil {
		log.Error("Command failed", zap.Error(err))
		return reply, err
	}
	log.Info("Command succeeded", zap.String("reply", reply))
	return reply, nil
}

func (sc *serverConn) readReplyLine() (string, error) {
	line, err := sc.tp.ReadLine()
	if err != nil {
		return line, err
	}
	if strings.HasPrefix(line, "+OK") {
		return strings.TrimPrefix(line[3:], " "), nil
	}
	if strings.HasPrefix(line, "-ERR") {
		return "", fmt.Errorf("Server error: %s", line[4:])
	}
	return "", fmt.Errorf("Unexpected server reply: %q", line)
}

func (sc *serverConn) ListMessages() ([]Message, error) {
	_, err := sc.transaction("LIST")
	if err != nil {
		return nil, err
	}
	lines, err := sc.tp.ReadDotLines()
	if err != nil {
		return nil, err
	}
	msgs := make([]Message, len(lines))
	for i, line := range lines {
		msg := sc.parseMessageListLine(line)
		if msg == nil {
			sc.log.Error("Bad server message line", zap.Int("index", i), zap.String("line", line))
			return nil, fmt.Errorf("Bad server reply")
		}
		msgs[i] = msg
	}
	return msgs, nil
}

func (sc *serverConn) GetMessage(id int) Message {
	ls, err := sc.transaction("LIST %d", id)
	if err != nil {
		return nil
	}
	lines, err := sc.tp.ReadDotLines()
	if err != nil {
		return nil
	}
	if len(lines) != 1 {
		sc.log.Error("Server returned incorrect number of lines", zap.Strings("lines", lines))
		return nil
	}
	msg := sc.parseMessageListLine(lines[0])
	if msg == nil {
		sc.log.Error("Bad server message line", zap.String("reply", ls))
	}
	return msg
}

func (sc *serverConn) parseMessageListLine(line string) *serverMessage {
	var sid, size int
	n, err := fmt.Sscanf(line, "%d %d", &sid, &size)
	if n != 2 || err != nil {
		sc.log.Error("Failed to parse message line", zap.Int("numItems", n), zap.Error(err))
		return nil
	}
	return &serverMessage{
		sc:   sc,
		id:   sid,
		size: size,
	}
}

func (sc *serverConn) Retrieve(msg Message) (io.ReadCloser, error) {
	_, err := sc.transaction("RETR %d", msg.ID())
	if err != nil {
		return nil, err
	}
	return io.NopCloser(sc.tp.DotReader()), nil
}

func (sc *serverConn) Delete(msg Message) error {
	_, err := sc.transaction("DELE %d", msg.ID())
	if err == nil {
		sc.deleted[msg.ID()] = struct{}{}
	}
	return err
}

func (sc *serverConn) Close() error {
	if _, err := sc.transaction("QUIT"); err != nil {
		return err
	}
	sc.tp.Close()
	return nil
}

func (sc *serverConn) Reset() {
	if _, err := sc.transaction("RSET"); err == nil {
		sc.deleted = make(map[int]struct{})
	}
}

type serverMessage struct {
	sc   *serverConn
	id   int
	size int
}

func (m *serverMessage) UniqueID() string { return "" }
func (m *serverMessage) ID() int          { return m.id }
func (m *serverMessage) Size() int        { return m.size }
func (m *serverMessage) Deleted() bool {
	_, deleted := m.sc.deleted[m.id]
	return deleted
}
