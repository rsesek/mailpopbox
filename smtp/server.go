package smtp

import (
	"crypto/tls"
	"regexp"
	"fmt"
	"io"
	"net"
	"net/mail"
	"strings"
	"time"
)

type ReplyLine struct {
	Code    int
	Message string
}

func (l ReplyLine) String() string {
	return fmt.Sprintf("%d %s", l.Code, l.Message)
}

var SendAsSubject = regexp.MustCompile(`(?i)\[sendas:\s*([a-zA-Z0-9\.\-_]+)\]`)

var (
	ReplyOK               = ReplyLine{250, "OK"}
	ReplyBadSyntax        = ReplyLine{501, "syntax error"}
	ReplyBadSequence      = ReplyLine{503, "bad sequence of commands"}
	ReplyBadMailbox       = ReplyLine{550, "mailbox unavailable"}
	ReplyMailboxUnallowed = ReplyLine{553, "mailbox name not allowed"}
)

func DomainForAddress(addr mail.Address) string {
	return DomainForAddressString(addr.Address)
}

func DomainForAddressString(address string) string {
	domainIdx := strings.LastIndex(address, "@")
	if domainIdx == -1 {
		return ""
	}
	return address[domainIdx+1:]
}

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
	VerifyAddress(mail.Address) ReplyLine
	// Verify that the authc+passwd identity can send mail as authz.
	Authenticate(authz, authc, passwd string) bool
	OnMessageDelivered(Envelope) *ReplyLine

	// RelayMessage instructs the server to send the Envelope to another
	// MTA for outbound delivery.
	RelayMessage(Envelope)
}

type EmptyServerCallbacks struct{}

func (*EmptyServerCallbacks) TLSConfig() *tls.Config {
	return nil
}

func (*EmptyServerCallbacks) VerifyAddress(mail.Address) ReplyLine {
	return ReplyOK
}

func (*EmptyServerCallbacks) Authenticate(authz, authc, passwd string) bool {
	return false
}

func (*EmptyServerCallbacks) OnMessageDelivered(Envelope) *ReplyLine {
	return nil
}

func (*EmptyServerCallbacks) RelayMessage(Envelope) {
}
