// mailpopbox
// Copyright 2020 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"os"
	"path"
	"regexp"

	"go.uber.org/zap"

	"src.bluestatic.org/mailpopbox/smtp"
)

var sendAsSubject = regexp.MustCompile(`(?i)\[sendas:\s*([a-zA-Z0-9\.\-_]+)\]`)

func runSMTPServer(config Config, log *zap.Logger) <-chan ServerControlMessage {
	server := smtpServer{
		config:      config,
		controlChan: make(chan ServerControlMessage),
		log:         log.With(zap.String("server", "smtp")),
	}
	server.mta = smtp.NewDefaultMTA(&server, server.log)
	go server.run()
	return server.controlChan
}

type smtpServer struct {
	config    Config
	tlsConfig *tls.Config

	mta smtp.MTA

	log *zap.Logger

	controlChan chan ServerControlMessage
}

func (server *smtpServer) run() {
	if !server.loadTLSConfig() {
		return
	}

	addr := fmt.Sprintf(":%d", server.config.SMTPPort)
	server.log.Info("starting server", zap.String("address", addr))

	l, err := net.Listen("tcp", addr)
	if err != nil {
		server.log.Error("listen", zap.Error(err))
		server.controlChan <- ServerControlFatalError
		return
	}

	connChan := make(chan net.Conn)
	go RunAcceptLoop(l, connChan, server.log)

	reloadChan := CreateReloadSignal()

	for {
		select {
		case <-reloadChan:
			if !server.loadTLSConfig() {
				return
			}
		case conn, ok := <-connChan:
			if ok {
				go smtp.AcceptConnection(conn, server, server.log)
			} else {
				break
			}
		}
	}
}

func (server *smtpServer) loadTLSConfig() bool {
	var err error
	server.tlsConfig, err = server.config.GetTLSConfig()
	if err != nil {
		server.log.Error("failed to configure TLS", zap.Error(err))
		server.controlChan <- ServerControlFatalError
		return false
	}
	server.log.Info("loaded TLS config")
	return true
}

func (server *smtpServer) Name() string {
	return server.config.Hostname
}

func (server *smtpServer) TLSConfig() *tls.Config {
	return server.tlsConfig
}

func (server *smtpServer) VerifyAddress(addr mail.Address) smtp.ReplyLine {
	s := server.configForAddress(addr)
	if s == nil {
		return smtp.ReplyBadMailbox
	}
	for _, blocked := range s.BlockedAddresses {
		if blocked == addr.Address {
			return smtp.ReplyMailboxUnallowed
		}
	}
	return smtp.ReplyOK
}

func (server *smtpServer) Authenticate(authz, authc, passwd string) bool {
	authcAddr, err := mail.ParseAddress(authc)
	if err != nil {
		return false
	}

	authzAddr, err := mail.ParseAddress(authz)
	if authz != "" && err != nil {
		return false
	}

	domain := smtp.DomainForAddress(*authcAddr)
	for _, s := range server.config.Servers {
		if domain == s.Domain {
			authOk := authc == MailboxAccount+s.Domain && passwd == s.MailboxPassword
			if authzAddr != nil {
				authOk = authOk && smtp.DomainForAddress(*authzAddr) == domain
			}
			return authOk
		}
	}
	return false
}

func (server *smtpServer) DeliverMessage(en smtp.Envelope) *smtp.ReplyLine {
	maildrop := server.maildropForAddress(en.RcptTo[0])
	if maildrop == "" {
		server.log.Error("faild to open maildrop to deliver message", zap.String("id", en.ID))
		return &smtp.ReplyBadMailbox
	}

	f, err := os.Create(path.Join(maildrop, en.ID+".msg"))
	if err != nil {
		server.log.Error("failed to create message file", zap.String("id", en.ID), zap.Error(err))
		return &smtp.ReplyBadMailbox
	}

	smtp.WriteEnvelopeForDelivery(f, en)
	f.Close()
	return nil
}

func (server *smtpServer) configForAddress(addr mail.Address) *Server {
	domain := smtp.DomainForAddress(addr)
	for _, s := range server.config.Servers {
		if domain == s.Domain {
			return &s
		}
	}
	return nil
}

func (server *smtpServer) maildropForAddress(addr mail.Address) string {
	s := server.configForAddress(addr)
	if s != nil {
		return s.MaildropPath
	}
	return ""
}

func (server *smtpServer) RelayMessage(en smtp.Envelope, authc string) {
	go func() {
		log := server.log.With(zap.String("id", en.ID))
		server.handleSendAs(log, &en, authc)
		server.mta.RelayMessage(en)
	}()
}

func (server *smtpServer) handleSendAs(log *zap.Logger, en *smtp.Envelope, authc string) {
	// Find the separator between the message header and body.
	headerIdx := bytes.Index(en.Data, []byte("\n\n"))
	if headerIdx == -1 {
		log.Error("send-as: could not find headers index")
		return
	}

	var buf bytes.Buffer

	headers := bytes.SplitAfter(en.Data[:headerIdx], []byte("\n"))

	var fromIdx, subjectIdx int
	for i, header := range headers {
		if bytes.HasPrefix(header, []byte("From:")) {
			fromIdx = i
			continue
		}
		if bytes.HasPrefix(header, []byte("Subject:")) {
			subjectIdx = i
			continue
		}
	}

	if subjectIdx == -1 {
		log.Error("send-as: could not find Subject header")
		return
	}
	if fromIdx == -1 {
		log.Error("send-as: could not find From header")
		return
	}

	sendAs := sendAsSubject.FindSubmatchIndex(headers[subjectIdx])
	if sendAs == nil {
		// No send-as modification.
		return
	}

	// Submatch 0 is the whole sendas magic. Submatch 1 is the address prefix.
	sendAsUser := headers[subjectIdx][sendAs[2]:sendAs[3]]
	sendAsAddress := string(sendAsUser) + "@" + smtp.DomainForAddressString(authc)

	log.Info("handling send-as", zap.String("address", sendAsAddress))

	for i, header := range headers {
		if i == subjectIdx {
			buf.Write(header[:sendAs[0]])
			buf.Write(header[sendAs[1]:])
		} else if i == fromIdx {
			addressStart := bytes.LastIndexByte(header, byte('<'))
			buf.Write(header[:addressStart+1])
			buf.WriteString(sendAsAddress)
			buf.WriteString(">\n")
		} else {
			buf.Write(header)
		}
	}

	buf.Write(en.Data[headerIdx:])

	en.Data = buf.Bytes()
	en.MailFrom.Address = sendAsAddress
}
