package smtp

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/mail"
	"net/textproto"
	"strings"
	"time"

	"github.com/uber-go/zap"
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

	tp *textproto.Conn

	nc         net.Conn
	remoteAddr net.Addr

	esmtp bool
	tls   *tls.ConnectionState

	// The authcid from a PLAIN SASL login. Non-empty iff tls is non-nil and
	// doAUTH() succeeded.
	authc string

	log zap.Logger

	state
	line string

	ehlo     string
	mailFrom *mail.Address
	rcptTo   []mail.Address
}

func AcceptConnection(netConn net.Conn, server Server, log zap.Logger) {
	conn := connection{
		server:     server,
		tp:         textproto.NewConn(netConn),
		nc:         netConn,
		remoteAddr: netConn.RemoteAddr(),
		log:        log.With(zap.Stringer("client", netConn.RemoteAddr())),
		state:      stateNew,
	}

	conn.log.Info("accepted connection")
	conn.writeReply(220, fmt.Sprintf("%s ESMTP [%s] (mailpopbox)",
		server.Name(), netConn.LocalAddr()))

	for {
		var err error
		conn.line, err = conn.tp.ReadLine()
		if err != nil {
			conn.log.Error("ReadLine()", zap.Error(err))
			conn.tp.Close()
			return
		}

		conn.log.Info("ReadLine()", zap.String("line", conn.line))

		var cmd string
		if _, err = fmt.Sscanf(conn.line, "%s", &cmd); err != nil {
			conn.reply(ReplyBadSyntax)
			continue
		}

		switch strings.ToUpper(cmd) {
		case "QUIT":
			conn.writeReply(221, "Goodbye")
			conn.tp.Close()
			return
		case "HELO":
			conn.esmtp = false
			fallthrough
		case "EHLO":
			conn.esmtp = true
			conn.doEHLO()
		case "STARTTLS":
			conn.doSTARTTLS()
		case "AUTH":
			conn.doAUTH()
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
}

func (conn *connection) reply(reply ReplyLine) error {
	return conn.writeReply(reply.Code, reply.Message)
}

func (conn *connection) writeReply(code int, msg string) error {
	conn.log.Info("writeReply", zap.Int("code", code))
	var err error
	if len(msg) > 0 {
		err = conn.tp.PrintfLine("%d %s", code, msg)
	} else {
		err = conn.tp.PrintfLine("%d", code)
	}
	if err != nil {
		conn.log.Error("writeReply",
			zap.Int("code", code),
			zap.Error(err))
	}
	return err
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
	params := conn.line[len(command):]
	idx := strings.Index(params, ">")
	if idx == -1 {
		return "", ReplyBadSyntax
	}
	return strings.ToLower(params[:idx+1]), ReplyOK
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
		if conn.server.TLSConfig() != nil && conn.tls == nil {
			conn.tp.PrintfLine("250-STARTTLS")
		}
		if conn.tls != nil {
			conn.tp.PrintfLine("250-AUTH PLAIN")
		}
		conn.tp.PrintfLine("250 SIZE %d", 40960000)
	}

	conn.log.Info("doEHLO()", zap.String("ehlo", conn.ehlo))

	conn.state = stateInitial
}

func (conn *connection) doSTARTTLS() {
	if conn.state != stateInitial {
		conn.reply(ReplyBadSequence)
		return
	}

	tlsConfig := conn.server.TLSConfig()
	if !conn.esmtp || tlsConfig == nil {
		conn.writeReply(500, "unrecognized command")
		return
	}

	conn.log.Info("doSTARTTLS()")
	conn.writeReply(220, "initiate TLS connection")

	tlsConn := tls.Server(conn.nc, tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		conn.log.Error("failed to do TLS handshake", zap.Error(err))
		return
	}

	conn.nc = tlsConn
	conn.tp = textproto.NewConn(tlsConn)
	conn.state = stateNew

	connState := tlsConn.ConnectionState()
	conn.tls = &connState

	conn.log.Info("TLS connection done", zap.String("state", conn.getTransportString()))
}

func (conn *connection) doAUTH() {
	if conn.state != stateInitial || conn.tls == nil {
		conn.reply(ReplyBadSequence)
		return
	}

	if conn.authc != "" {
		conn.writeReply(503, "already authenticated")
		return
	}

	var cmd, authType string
	_, err := fmt.Sscanf(conn.line, "%s %s", &cmd, &authType)
	if err != nil {
		conn.reply(ReplyBadSyntax)
		return
	}

	if authType != "PLAIN" {
		conn.writeReply(504, "unrecognized auth type")
		return
	}

	conn.log.Info("doAUTH()")

	conn.writeReply(334, " ")

	authLine, err := conn.tp.ReadLine()
	if err != nil {
		conn.log.Error("failed to read auth line", zap.Error(err))
		conn.reply(ReplyBadSyntax)
		return
	}

	authBytes, err := base64.StdEncoding.DecodeString(authLine)
	if err != nil {
		conn.reply(ReplyBadSyntax)
		return
	}

	authParts := strings.Split(string(authBytes), "\x00")
	if len(authParts) != 3 {
		conn.log.Error("bad auth line syntax")
		conn.reply(ReplyBadSyntax)
		return
	}

	if !conn.server.Authenticate(authParts[0], authParts[1], authParts[2]) {
		conn.log.Error("failed to authenticate", zap.String("authc", authParts[1]))
		conn.writeReply(535, "invalid credentials")
		return
	}

	conn.log.Info("authenticated", zap.String("authz", authParts[0]), zap.String("authc", authParts[1]))
	conn.authc = authParts[1]
	conn.reply(ReplyOK)
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
	if err != nil || conn.mailFrom == nil {
		conn.reply(ReplyBadSyntax)
		return
	}

	conn.log.Info("doMAIL()", zap.String("address", conn.mailFrom.Address))

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
		return
	}

	if reply := conn.server.VerifyAddress(*address); reply != ReplyOK {
		conn.log.Warn("invalid address",
			zap.String("address", address.Address),
			zap.Stringer("reply", reply))
		conn.reply(reply)
		return
	}

	conn.log.Info("doRCPT()", zap.String("address", address.Address))

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
	conn.log.Info("doDATA()")

	data, err := conn.tp.ReadDotBytes()
	if err != nil {
		conn.log.Error("failed to ReadDotBytes()",
			zap.Error(err),
			zap.String("bytes", fmt.Sprintf("%x", data)))
		conn.writeReply(552, "transaction failed")
		return
	}

	received := time.Now()
	env := Envelope{
		RemoteAddr: conn.remoteAddr,
		EHLO:       conn.ehlo,
		MailFrom:   *conn.mailFrom,
		RcptTo:     conn.rcptTo,
		Received:   received,
		ID:         conn.envelopeID(received),
	}

	conn.log.Info("received message",
		zap.Int("bytes", len(data)),
		zap.Time("date", received),
		zap.String("id", env.ID))

	trace := conn.getReceivedInfo(env)

	env.Data = append(trace, data...)

	if reply := conn.server.OnMessageDelivered(env); reply != nil {
		conn.log.Warn("message was rejected", zap.String("id", env.ID))
		conn.reply(*reply)
		return
	}

	conn.state = stateInitial
	conn.reply(ReplyOK)
}

func (conn *connection) envelopeID(t time.Time) string {
	var idBytes [4]byte
	rand.Read(idBytes[:])
	return fmt.Sprintf("m.%d.%x", t.UnixNano(), idBytes)
}

func (conn *connection) getReceivedInfo(envelope Envelope) []byte {
	rhost, _, err := net.SplitHostPort(conn.remoteAddr.String())
	if err != nil {
		rhost = conn.remoteAddr.String()
	}

	rhosts, err := net.LookupAddr(rhost)
	if err == nil {
		rhost = fmt.Sprintf("%s [%s]", rhosts[0], rhost)
	}

	base := fmt.Sprintf("Received: from %s (%s)\r\n        ", conn.ehlo, rhost)

	with := "SMTP"
	if conn.esmtp {
		with = "E" + with
	}
	if conn.tls != nil {
		with += "S"
	}
	base += fmt.Sprintf("by %s (mailpopbox) with %s id %s\r\n        ", conn.server.Name(), with, envelope.ID)

	base += fmt.Sprintf("for <%s>\r\n        ", envelope.RcptTo[0].Address)

	transport := conn.getTransportString()
	date := envelope.Received.Format(time.RFC1123Z) // Same as RFC 5322 § 3.3
	base += fmt.Sprintf("(using %s);\r\n        %s\r\n", transport, date)

	return []byte(base)
}

func (conn *connection) getTransportString() string {
	if conn.tls == nil {
		return "PLAINTEXT"
	}

	ciphers := map[uint16]string{
		tls.TLS_RSA_WITH_RC4_128_SHA:                "TLS_RSA_WITH_RC4_128_SHA",
		tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA:           "TLS_RSA_WITH_3DES_EDE_CBC_SHA",
		tls.TLS_RSA_WITH_AES_128_CBC_SHA:            "TLS_RSA_WITH_AES_128_CBC_SHA",
		tls.TLS_RSA_WITH_AES_256_CBC_SHA:            "TLS_RSA_WITH_AES_256_CBC_SHA",
		tls.TLS_RSA_WITH_AES_128_GCM_SHA256:         "TLS_RSA_WITH_AES_128_GCM_SHA256",
		tls.TLS_RSA_WITH_AES_256_GCM_SHA384:         "TLS_RSA_WITH_AES_256_GCM_SHA384",
		tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA:        "TLS_ECDHE_ECDSA_WITH_RC4_128_SHA",
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA:    "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA",
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA:    "TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA",
		tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA:          "TLS_ECDHE_RSA_WITH_RC4_128_SHA",
		tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA:     "TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA",
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA:      "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA",
		tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA:      "TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA",
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256:   "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256: "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384:   "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384: "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384",
	}
	versions := map[uint16]string{
		tls.VersionSSL30: "SSLv3.0",
		tls.VersionTLS10: "TLSv1.0",
		tls.VersionTLS11: "TLSv1.1",
		tls.VersionTLS12: "TLSv1.2",
	}

	state := conn.tls

	version := versions[state.Version]
	cipher := ciphers[state.CipherSuite]

	if version == "" {
		version = fmt.Sprintf("%x", state.Version)
	}
	if cipher == "" {
		cipher = fmt.Sprintf("%x", state.CipherSuite)
	}

	name := ""
	if state.ServerName != "" {
		name = fmt.Sprintf(" name=%s", state.ServerName)
	}

	return fmt.Sprintf("%s cipher=%s%s", version, cipher, name)
}

func (conn *connection) doRSET() {
	conn.log.Info("doRSET()")
	conn.state = stateInitial
	conn.resetBuffers()
	conn.reply(ReplyOK)
}

func (conn *connection) resetBuffers() {
	conn.mailFrom = nil
	conn.rcptTo = make([]mail.Address, 0)
}
