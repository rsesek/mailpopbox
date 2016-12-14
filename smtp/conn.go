package smtp

import (
	"fmt"
	"net"
	"net/mail"
	"net/textproto"
	"strings"
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

		switch strings.ToUpper(cmd) {
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
			conn.writeReply(252, "I'll do my best")
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

// parsePath parses out either a forward-, reverse-, or return-path from the
// current connection line. Returns a (valid-path, ReplyOK) if it was
// successfully parsed.
func (conn *connection) parsePath(command string) (string, ReplyLine) {
	if len(conn.line) < len(command) {
		return "", ReplyBadSyntax
	}
	if strings.ToUpper(command) != strings.ToUpper(conn.line[:len(command)]) {
		return "", ReplyLine{500, "unrecognized command"}
	}
	return conn.line[len(command):], ReplyOK
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

	mailFrom, reply := conn.parsePath("MAIL FROM:")
	if reply != ReplyOK {
		conn.reply(reply)
		return
	}

	var err error
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

	rcptTo, reply := conn.parsePath("RCPT TO:")
	if reply != ReplyOK {
		conn.reply(reply)
		return
	}

	address, err := mail.ParseAddress(rcptTo)
	if err != nil {
		conn.reply(ReplyBadSyntax)
	}

	if reply := conn.server.VerifyAddress(*address); reply != ReplyOK {
		conn.reply(reply)
		return
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

func (conn *connection) doRSET() {
	conn.state = stateInitial
	conn.resetBuffers()
	conn.reply(ReplyOK)
}

func (conn *connection) resetBuffers() {
	conn.mailFrom = nil
	conn.rcptTo = make([]mail.Address, 0)
}
