// mailpopbox
// Copyright 2020 Blue Static <https://www.bluestatic.org>
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

type state int

const (
	stateAuth state = iota
	stateTxn
	stateUpdate
)

const (
	errStateAuth  = "not in AUTHORIZATION"
	errStateTxn   = "not in TRANSACTION"
	errSyntax     = "syntax error"
	errDeletedMsg = "no such message - deleted"
)

type connection struct {
	po PostOffice
	mb Mailbox

	tp         *textproto.Conn
	remoteAddr net.Addr

	log *zap.Logger

	state
	line string

	user string
}

// AcceptConnection implements a POP3 server connection, parsing the client
// requests sent over `netConn` and providing access to the mailboxes in the
// specified `PostOffice`.
func AcceptConnection(netConn net.Conn, po PostOffice, log *zap.Logger) {
	log = log.With(zap.Stringer("client", netConn.RemoteAddr()))
	conn := connection{
		po:    po,
		tp:    textproto.NewConn(netConn),
		state: stateAuth,
		log:   log,
	}

	conn.log.Info("accepted connection")
	conn.ok(fmt.Sprintf("POP3 (mailpopbox) server %s", po.Name()))

	var err error

	for {
		conn.line, err = conn.tp.ReadLine()
		if err != nil {
			conn.log.Error("ReadLine()", zap.Error(err))
			conn.tp.Close()
			return
		}

		var cmd string
		if _, err := fmt.Sscanf(conn.line, "%s", &cmd); err != nil {
			conn.err("invalid command")
			continue
		}

		conn.log = log.With(zap.String("command", cmd))

		switch strings.ToUpper(cmd) {
		case "QUIT":
			conn.doQUIT()
			return
		case "USER":
			conn.doUSER()
		case "PASS":
			conn.doPASS()
		case "STAT":
			conn.doSTAT()
		case "LIST":
			conn.doLIST()
		case "RETR":
			conn.doRETR()
		case "DELE":
			conn.doDELE()
		case "NOOP":
			conn.ok("")
		case "RSET":
			conn.doRSET()
		case "UIDL":
			conn.doUIDL()
		case "CAPA":
			conn.doCAPA()
		default:
			conn.err("unknown command")
		}
	}
}

func (conn *connection) ok(msg string) {
	conn.log.Info("ok", zap.String("reply", msg))
	if len(msg) > 0 {
		msg = " " + msg
	}
	conn.tp.PrintfLine("+OK%s", msg)
}

func (conn *connection) err(msg string) {
	conn.log.Error("error", zap.String("message", msg))
	if len(msg) > 0 {
		msg = " " + msg
		conn.tp.PrintfLine("-ERR%s", msg)
	}
}

func (conn *connection) doQUIT() {
	defer conn.tp.Close()

	if conn.mb != nil {
		err := conn.mb.Close()
		if err != nil {
			conn.err("failed to remove some messages")
			return
		}
	}
	conn.ok("goodbye")
}

func (conn *connection) doUSER() {
	if conn.state != stateAuth {
		conn.err(errStateAuth)
		return
	}

	cmd := len("USER ")
	if len(conn.line) < cmd {
		conn.err("invalid user")
		return
	}

	conn.user = conn.line[cmd:]
	conn.ok("")
}

func (conn *connection) doPASS() {
	if conn.state != stateAuth {
		conn.err(errStateAuth)
		return
	}

	if len(conn.user) == 0 {
		conn.err("no USER")
		return
	}

	cmd := len("PASS ")
	if len(conn.line) < cmd {
		conn.err("invalid pass")
		return
	}

	pass := conn.line[cmd:]
	if mbox, err := conn.po.OpenMailbox(conn.user, pass); err == nil {
		conn.log.Info("authenticated", zap.String("user", conn.user))
		conn.state = stateTxn
		conn.mb = mbox
		conn.ok("")
	} else {
		conn.log.Error("failed to open mailbox", zap.Error(err))
		conn.err(err.Error())
	}
}

func (conn *connection) doSTAT() {
	if conn.state != stateTxn {
		conn.err(errStateTxn)
		return
	}

	msgs, err := conn.mb.ListMessages()
	if err != nil {
		conn.log.Error("failed to list messages", zap.Error(err))
		conn.err(err.Error())
		return
	}

	size := 0
	num := 0
	for _, msg := range msgs {
		if msg.Deleted() {
			continue
		}
		size += msg.Size()
		num++
	}

	conn.ok(fmt.Sprintf("%d %d", num, size))
}

func (conn *connection) doLIST() {
	if conn.state != stateTxn {
		conn.err(errStateTxn)
		return
	}

	var msgs []Message

	var cmd string
	var id int
	n, _ := fmt.Sscanf(conn.line, "%s %d", &cmd, &id)
	if n == 2 {
		msg := conn.mb.GetMessage(id)
		if msg == nil {
			conn.err("No message with that ID")
			return
		}
		if msg.Deleted() {
			conn.err(errDeletedMsg)
			return
		}
		msgs = []Message{msg}
	} else {
		var err error
		msgs, err = conn.mb.ListMessages()
		if err != nil {
			conn.log.Error("failed to list messages", zap.Error(err))
			conn.err(err.Error())
			return
		}
	}

	conn.ok("scan listing")
	for _, msg := range msgs {
		conn.tp.PrintfLine("%d %d", msg.ID(), msg.Size())
	}
	conn.tp.PrintfLine(".")
}

func (conn *connection) doRETR() {
	if conn.state != stateTxn {
		conn.err(errStateTxn)
		return
	}

	msg := conn.getRequestedMessage()
	if msg == nil {
		return
	}

	if msg.Deleted() {
		conn.err(errDeletedMsg)
		return
	}

	rc, err := conn.mb.Retrieve(msg)
	if err != nil {
		conn.log.Error("failed to retrieve messages", zap.Error(err))
		conn.err(err.Error())
		return
	}

	conn.log.Info("retrieve message", zap.String("unique-id", msg.UniqueID()))
	conn.ok(fmt.Sprintf("%d", msg.Size()))

	w := conn.tp.DotWriter()
	io.Copy(w, rc)
	w.Close()
}

func (conn *connection) doDELE() {
	if conn.state != stateTxn {
		conn.err(errStateTxn)
		return
	}

	msg := conn.getRequestedMessage()
	if msg == nil {
		return
	}

	if msg.Deleted() {
		conn.err(errDeletedMsg)
		return
	}

	if err := conn.mb.Delete(msg); err != nil {
		conn.log.Error("failed to delete message", zap.Error(err))
		conn.err(err.Error())
	} else {
		conn.log.Info("delete message", zap.String("unique-id", msg.UniqueID()))
		conn.ok("")
	}
}

func (conn *connection) doRSET() {
	if conn.state != stateTxn {
		conn.err(errStateTxn)
		return
	}
	conn.mb.Reset()
	conn.log.Info("reset")
	conn.ok("")
}

func (conn *connection) doUIDL() {
	if conn.state != stateTxn {
		conn.err(errStateTxn)
		return
	}

	msgs, err := conn.mb.ListMessages()
	if err != nil {
		conn.log.Error("failed to list messages", zap.Error(err))
		conn.err(err.Error())
		return
	}

	conn.ok("unique-id listing")
	for _, msg := range msgs {
		if msg.Deleted() {
			continue
		}
		conn.tp.PrintfLine("%d %s", msg.ID(), msg.UniqueID())
	}
	conn.tp.PrintfLine(".")
}

func (conn *connection) doCAPA() {
	conn.ok("capability list")

	caps := []string{
		"USER",
		"UIDL",
		".",
	}
	for _, c := range caps {
		conn.tp.PrintfLine("%s", c)
	}
}

func (conn *connection) getRequestedMessage() Message {
	var cmd string
	var idx int
	if _, err := fmt.Sscanf(conn.line, "%s %d", &cmd, &idx); err != nil {
		conn.err(errSyntax)
		return nil
	}

	if idx < 1 {
		conn.err("invalid message-number")
		return nil
	}

	msg := conn.mb.GetMessage(idx)
	if msg == nil {
		conn.err("no such message")
		return nil
	}
	return msg
}
