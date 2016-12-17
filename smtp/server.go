package smtp

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/mail"
	"time"
)

type ReplyLine struct {
	Code    int
	Message string
}

var (
	ReplyOK          = ReplyLine{250, "OK"}
	ReplyBadSyntax   = ReplyLine{501, "syntax error"}
	ReplyBadSequence = ReplyLine{503, "bad sequence of commands"}
	ReplyBadMailbox  = ReplyLine{550, "mailbox unavailable"}
)

type Envelope struct {
	RemoteAddr net.Addr
	EHLO       string
	MailFrom   mail.Address
	RcptTo     []mail.Address
	Data       []byte
	Received   time.Time
	ID         string
}

func WriteEnvelopeForDelivery(w io.Writer, e Envelope) {
	fmt.Fprintf(w, "Delivered-To: <%s>\r\n", e.RcptTo[0].Address)
	fmt.Fprintf(w, "Return-Path: <%s>\r\n", e.MailFrom.Address)
	w.Write(e.Data)
}

type Server interface {
	Name() string
	TLSConfig() *tls.Config
	OnEHLO() *ReplyLine
	VerifyAddress(mail.Address) ReplyLine
	OnMessageDelivered(Envelope) *ReplyLine
}

type EmptyServerCallbacks struct{}

func (*EmptyServerCallbacks) TLSConfig() *tls.Config {
	return nil
}

func (*EmptyServerCallbacks) OnEHLO() *ReplyLine {
	return nil
}

func (*EmptyServerCallbacks) VerifyAddress(mail.Address) ReplyLine {
	return ReplyOK
}

func (*EmptyServerCallbacks) OnMessageDelivered(Envelope) *ReplyLine {
	return nil
}
