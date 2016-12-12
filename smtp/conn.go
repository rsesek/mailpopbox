package smtp

import (
	"fmt"
	"net"
	"net/mail"
	"net/textproto"
)

type state int

const (
	stateNew state = iota // Before EHOL.
	stateInitial
	stateMail
	stateRecipient
	stateData
)

type connection struct {
	tp         *textproto.Conn
	remoteAddr net.Addr

	state
	line string

	ehlo     string
	mailFrom *mail.Address
	rcptTo   []mail.Address
}

func AcceptConnection(netConn net.Conn, server Server) error {
	conn := connection{
		tp:         textproto.NewConn(netConn),
		remoteAddr: netConn.RemoteAddr(),
		state:      stateNew,
	}

	var err error

	conn.writeReply(250, fmt.Sprintf("%s ESMTP [%s] mailpopbox", server.Name(), netConn.LocalAddr().String()))

	for {
		conn.line, err = conn.tp.ReadLine()
		if err != nil {
			conn.writeReply(500, "line too long")
			continue
		}

		var cmd string
		if _, err = fmt.Sscanf(conn.line, "%s", &cmd); err != nil {
			conn.writeBadSyntax()
			continue
		}

		switch cmd {
		case "QUIT":
			conn.tp.Close()
			break
		case "HELO":
			fallthrough
		case "EHLO":
			conn.doEHLO()
		case "MAIL":
			conn.doMAIL()
		case "RCPT":
			conn.doRCPT()
		case "DATA":
			conn.doDATA()
		case "RSET":
			conn.doRSET()
		case "VRFY":
			conn.doVRFY()
		case "EXPN":
			conn.writeReply(550, "access denied")
		case "NOOP":
			conn.writeOK()
		case "HELP":
			conn.writeReply(250, "https://tools.ietf.org/html/rfc5321")
		default:
			conn.writeReply(500, "unrecognized command")
		}
	}

	return err
}

func (conn *connection) writeReply(code int, msg string) {
	if len(msg) > 0 {
		conn.tp.PrintfLine("%d %s", code, msg)
	} else {
		conn.tp.PrintfLine("%d", code)
	}
}

func (conn *connection) writeOK() {
	conn.writeReply(250, "OK")
}

func (conn *connection) writeBadSyntax() {
	conn.writeReply(501, "syntax error")
}

func (conn *connection) writeBadSequence() {
	conn.writeReply(503, "bad sequence of commands")
}

func (conn *connection) doEHLO() {
	conn.resetBuffers()

	var cmd string
	_, err := fmt.Sscanf(conn.line, "%s %s", &cmd, &conn.ehlo)
	if err != nil {
		conn.writeBadSyntax()
		return
	}

	conn.writeReply(250, fmt.Sprintf("Hello %s, I am glad to meet you", conn.ehlo))

	conn.state = stateInitial
}

func (conn *connection) doMAIL() {
	if conn.state != stateInitial {
		conn.writeBadSequence()
		return
	}

	var mailFrom string
	_, err := fmt.Sscanf(conn.line, "MAIL FROM:%s", &mailFrom)
	if err != nil {
		conn.writeBadSyntax()
		return
	}

	conn.mailFrom, err = mail.ParseAddress(mailFrom)
	if err != nil {
		conn.writeBadSyntax()
		return
	}

	conn.state = stateMail
	conn.writeOK()
}

func (conn *connection) doRCPT() {
	if conn.state != stateMail && conn.state != stateRecipient {
		conn.writeBadSequence()
		return
	}

	var rcptTo string
	_, err := fmt.Sscanf(conn.line, "RCPT TO:%s", &rcptTo)
	if err != nil {
		conn.writeBadSyntax()
		return
	}

	address, err := mail.ParseAddress(rcptTo)
	if err != nil {
		conn.writeBadSyntax()
	}

	conn.rcptTo = append(conn.rcptTo, *address)

	conn.state = stateRecipient
	conn.writeOK()
}

func (conn *connection) doDATA() {
	if conn.state != stateRecipient {
		conn.writeBadSequence()
		return
	}

	conn.writeReply(354, "Start mail input; end with <CRLF>.<CRLF>")

	data, err := conn.tp.ReadDotBytes()
	if err != nil {
		// TODO: log error
		conn.writeReply(552, "transaction failed")
		return
	}

	fmt.Println(string(data))

	conn.state = stateInitial
	conn.writeOK()
}

func (conn *connection) doVRFY() {
}

func (conn *connection) doRSET() {
	conn.state = stateInitial
	conn.resetBuffers()
	conn.writeOK()
}

func (conn *connection) resetBuffers() {
	conn.mailFrom = nil
	conn.rcptTo = make([]mail.Address, 0)
}
