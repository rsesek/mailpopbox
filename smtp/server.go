package smtp

import (
	"crypto/tls"
	"net"
	"net/mail"
)

type ReplyLine struct {
	Code    int
	Message string
}

var (
	ReplyOK          = ReplyLine{250, "OK"}
	ReplyBadSyntax   = ReplyLine{501, "syntax error"}
	ReplyBadSequence = ReplyLine{503, "bad sequence of commands"}
)

type Envelope struct {
	RemoteAddr net.Addr
	EHLO       string
	MailFrom   mail.Address
	RcptTo     []mail.Address
	Data       []byte
}

type Server interface {
	Name() string
	TLSConfig() *tls.Config
	OnEHLO() *ReplyLine
	OnMessageDelivered(Envelope) *ReplyLine
}

type EmptyServerCallbacks struct {}

func (*EmptyServerCallbacks) TLSConfig() *tls.Config {
	return nil
}

func (*EmptyServerCallbacks) OnEHLO() *ReplyLine {
	return nil
}

func (*EmptyServerCallbacks) OnMessageDelivered(Envelope) *ReplyLine {
	return nil
}
