// mailpopbox
// Copyright 2020 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package smtp

import (
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/mail"
	"strings"
	"time"

	"go.uber.org/zap"
)

type ReplyLine struct {
	Code    int
	Message string
}

func (l ReplyLine) String() string {
	return fmt.Sprintf("%d %s", l.Code, l.Message)
}

var (
	ReplyOK               = ReplyLine{250, "OK"}
	ReplyAuthOK           = ReplyLine{235, "auth success"}
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

func generateEnvelopeId(prefix string, t time.Time) string {
	var idBytes [4]byte
	rand.Read(idBytes[:])
	return fmt.Sprintf("%s.%d.%x", prefix, t.UnixNano(), idBytes)
}

// lookupRemoteHost attempts to reverse look-up the provided IP address. On
// success, it returns the hostname and the IP as formatted for a receive
// trace. If the lookup fails, it just returns the original IP.
func lookupRemoteHost(addr net.Addr) string {
	rhost, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		rhost = addr.String()
	}

	rhosts, err := net.LookupAddr(rhost)
	if err == nil {
		rhost = fmt.Sprintf("%s [%s]", rhosts[0], rhost)
	}

	return rhost
}

// Server provides an interface for handling incoming SMTP requests via
// AcceptConnection.
type Server interface {
	// Returns the name of the server, to use in HELO advertisements.
	Name() string

	// If non-nil, enables STARTTLS support on the SMTP server with the given
	// configuration.
	TLSConfig() *tls.Config

	// Returns an status line indicating whether this server can receive
	// mail for the specified email address.
	VerifyAddress(mail.Address) ReplyLine

	// Verify that the authc+passwd identity can send mail as authz on this
	// server.
	Authenticate(authz, authc, passwd string) bool

	// Delivers a valid incoming message to a recipient on this server. The
	// addressee has been validated via VerifyAddress.
	DeliverMessage(Envelope) *ReplyLine

	// RelayMessage instructs the server to send the Envelope to another
	// MTA for outbound delivery. `authc` reports the authenticated username.
	RelayMessage(en Envelope, authc string)
}

// MTA (Mail Transport Agent) allows a Server to interface with other SMTP
// MTAs.
type MTA interface {
	// RelayMessage will attempt to send the specified Envelope. It will ask the
	// Server to dial the MX servers for the addresses in Envelope.RcptTo for
	// delivery. If relaying fails, a failure notice will be sent to the sender
	// via Server.DeliverMessage.
	RelayMessage(Envelope)
}

func NewDefaultMTA(server Server, log *zap.Logger) MTA {
	return &mta{
		server: server,
		log:    log,
	}
}

type mta struct {
	server Server
	log    *zap.Logger
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

func (*EmptyServerCallbacks) DeliverMessage(Envelope) *ReplyLine {
	return nil
}

func (*EmptyServerCallbacks) RelayMessage(Envelope) {
}
