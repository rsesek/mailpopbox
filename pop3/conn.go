package pop3

import (
	"fmt"
	"io"
	"net"
	"net/textproto"
	"strings"
)

type state int

const (
	stateAuth state = iota
	stateTxn
	stateUpdate
)

const (
	errStateAuth = "not in AUTHORIZATION"
	errStateTxn  = "not in TRANSACTION"
	errSyntax    = "syntax error"
)

type connection struct {
	po PostOffice
	mb Mailbox

	tp         *textproto.Conn
	remoteAddr net.Addr

	state
	line string

	user string
}

func AcceptConnection(netConn net.Conn, po PostOffice) {
	conn := connection{
		po:    po,
		tp:    textproto.NewConn(netConn),
		state: stateAuth,
	}

	var err error
	conn.ok(fmt.Sprintf("POP3 (mailpopbox) server %s", po.Name()))

	for {
		conn.line, err = conn.tp.ReadLine()
		if err != nil {
			conn.err("did't catch that")
			continue
		}

		var cmd string
		if _, err := fmt.Sscanf(conn.line, "%s", &cmd); err != nil {
			conn.err("invalid command")
			continue
		}

		switch cmd {
		case "QUIT":
			conn.doQUIT()
			break
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
		default:
			conn.err("unknown command")
		}
	}
}

func (conn *connection) ok(msg string) {
	if len(msg) > 0 {
		msg = " " + msg
	}
	conn.tp.PrintfLine("+OK%s", msg)
}

func (conn *connection) err(msg string) {
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

	if _, err := fmt.Sscanf(conn.line, "USER %s", &conn.user); err != nil {
		conn.err(errSyntax)
		return
	}

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

	pass := strings.TrimPrefix(conn.line, "PASS ")
	if mbox, err := conn.po.OpenMailbox(conn.user, pass); err == nil {
		conn.state = stateTxn
		conn.mb = mbox
		conn.ok("")
	} else {
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

	msgs, err := conn.mb.ListMessages()
	if err != nil {
		conn.err(err.Error())
		return
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

	rc, err := conn.mb.Retrieve(msg)
	if err != nil {
		conn.err(err.Error())
		return
	}

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

	if err := conn.mb.Delete(msg); err != nil {
		conn.err(err.Error())
	} else {
		conn.ok("")
	}
}

func (conn *connection) doRSET() {
	if conn.state != stateTxn {
		conn.err(errStateTxn)
		return
	}
	conn.mb.Reset()
	conn.ok("")
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
