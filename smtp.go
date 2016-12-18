package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"os"
	"path"
	"strings"

	"src.bluestatic.org/mailpopbox/smtp"
)

func runSMTPServer(config Config) <-chan error {
	server := smtpServer{
		config: config,
		rc:     make(chan error),
	}
	go server.run()
	return server.rc
}

type smtpServer struct {
	config    Config
	tlsConfig *tls.Config

	rc chan error
}

func (server *smtpServer) run() {
	var err error
	server.tlsConfig, err = server.config.GetTLSConfig()
	if err != nil {
		server.rc <- err
	}

	l, err := net.Listen("tcp", fmt.Sprintf(":%d", server.config.SMTPPort))
	if err != nil {
		server.rc <- err
		return
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			server.rc <- err
			return
		}

		go smtp.AcceptConnection(conn, server)
	}
}

func (server *smtpServer) Name() string {
	return server.config.Hostname
}

func (server *smtpServer) TLSConfig() *tls.Config {
	return server.tlsConfig
}

func (server *smtpServer) VerifyAddress(addr mail.Address) smtp.ReplyLine {
	if server.maildropForAddress(addr) == "" {
		return smtp.ReplyBadMailbox
	}
	return smtp.ReplyOK
}

func (server *smtpServer) OnEHLO() *smtp.ReplyLine {
	return nil
}

func (server *smtpServer) OnMessageDelivered(en smtp.Envelope) *smtp.ReplyLine {
	maildrop := server.maildropForAddress(en.RcptTo[0])
	if maildrop == "" {
		// TODO: log error
		return &smtp.ReplyBadMailbox
	}

	f, err := os.Create(path.Join(maildrop, en.ID+".msg"))
	if err != nil {
		// TODO: log error
		return &smtp.ReplyBadMailbox
	}

	smtp.WriteEnvelopeForDelivery(f, en)
	f.Close()
	return nil
}

func (server *smtpServer) maildropForAddress(addr mail.Address) string {
	domainIdx := strings.LastIndex(addr.Address, "@")
	if domainIdx == -1 {
		return ""
	}

	domain := addr.Address[domainIdx+1:]

	for _, s := range server.config.Servers {
		if domain == s.Domain {
			return s.MaildropPath
		}
	}

	return ""
}
