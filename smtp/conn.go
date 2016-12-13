package smtp

import (
	"fmt"
	"net"
	"net/mail"
	"net/textproto"
)

type state int

const (
	stateNew state = iota // Before EHLO.
	stateInitial
	stateMail
	stateRecipient
	stateData
)

type connection struct {
	server Server

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
		server:     server,
		tp:         textproto.NewConn(netConn),
		remoteAddr: netConn.RemoteAddr(),
		state:      stateNew,
	}

	var err error

	conn.writeReply(220, fmt.Sprintf("%s ESMTP [%s] (mailpopbox)", server.Name(), netConn.LocalAddr()))

	for {
		conn.line, err = conn.tp.ReadLine()
		if err != nil {
			conn.writeReply(500, "line too long")
			continue
		}

		var cmd string
		if _, err = fmt.Sscanf(conn.line, "%s", &cmd); err != nil {
			conn.reply(ReplyBadSyntax)
			continue
		}

		switch cmd {
		case "QUIT":
			conn.writeReply(221, "Goodbye")
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
			conn.reply(ReplyOK)
		case "HELP":
			conn.writeReply(250, "https://tools.ietf.org/html/rfc5321")
		default:
			conn.writeReply(500, "unrecognized command")
		}
	}

	return err
}

func (conn *connection) reply(reply ReplyLine) {
	conn.writeReply(reply.Code, reply.Message)
}

func (conn *connection) writeReply(code int, msg string) {
	if len(msg) > 0 {
		conn.tp.PrintfLine("%d %s", code, msg)
	} else {
		conn.tp.PrintfLine("%d", code)
	}
}

func (conn *connection) doEHLO() {
	conn.resetBuffers()

	var cmd string
	_, err := fmt.Sscanf(conn.line, "%s %s", &cmd, &conn.ehlo)
	if err != nil {
		conn.reply(ReplyBadSyntax)
		return
	}

	if cmd == "HELO" {
		conn.writeReply(250, fmt.Sprintf("Hello %s [%s]", conn.ehlo, conn.remoteAddr))
	} else {
		conn.tp.PrintfLine("250-Hello %s [%s]", conn.ehlo, conn.remoteAddr)
		if conn.server.TLSConfig() != nil {
			conn.tp.PrintfLine("250-STARTTLS")
		}
		conn.tp.PrintfLine("250 SIZE %d", 40960000)
	}

	conn.state = stateInitial
}

func (conn *connection) doMAIL() {
	if conn.state != stateInitial {
		conn.reply(ReplyBadSequence)
		return
	}

	var mailFrom string
	_, err := fmt.Sscanf(conn.line, "MAIL FROM:%s", &mailFrom)
	if err != nil {
		conn.reply(ReplyBadSyntax)
		return
	}

	conn.mailFrom, err = mail.ParseAddress(mailFrom)
	if err != nil {
		conn.reply(ReplyBadSyntax)
		return
	}

	conn.state = stateMail
	conn.reply(ReplyOK)
}

func (conn *connection) doRCPT() {
	if conn.state != stateMail && conn.state != stateRecipient {
		conn.reply(ReplyBadSequence)
		return
	}

	var rcptTo string
	_, err := fmt.Sscanf(conn.line, "RCPT TO:%s", &rcptTo)
	if err != nil {
		conn.reply(ReplyBadSyntax)
		return
	}

	address, err := mail.ParseAddress(rcptTo)
	if err != nil {
		conn.reply(ReplyBadSyntax)
	}

	conn.rcptTo = append(conn.rcptTo, *address)

	conn.state = stateRecipient
	conn.reply(ReplyOK)
}

func (conn *connection) doDATA() {
	if conn.state != stateRecipient {
		conn.reply(ReplyBadSequence)
		return
	}

	conn.writeReply(354, "Start mail input; end with <CRLF>.<CRLF>")

	data, err := conn.tp.ReadDotBytes()
	if err != nil {
		// TODO: log error
		conn.writeReply(552, "transaction failed")
		return
	}

	env := Envelope{
		RemoteAddr: conn.remoteAddr,
		EHLO:       conn.ehlo,
		MailFrom:   *conn.mailFrom,
		RcptTo:     conn.rcptTo,
		Data:       data,
	}

	if reply := conn.server.OnMessageDelivered(env); reply != nil {
		conn.reply(*reply)
		return
	}

	conn.state = stateInitial
	conn.reply(ReplyOK)
}

func (conn *connection) doVRFY() {
}

func (conn *connection) doRSET() {
	conn.state = stateInitial
	conn.resetBuffers()
	conn.reply(ReplyOK)
}

func (conn *connection) resetBuffers() {
	conn.mailFrom = nil
	conn.rcptTo = make([]mail.Address, 0)
}
