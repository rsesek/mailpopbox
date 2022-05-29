// mailpopbox
// Copyright 2020 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"os"
	"path"

	"go.uber.org/zap"

	"src.bluestatic.org/mailpopbox/smtp"
)

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
	if server.maildropForAddress(addr) == "" {
		return smtp.ReplyBadMailbox
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

func (server *smtpServer) RelayMessage(en smtp.Envelope) {
	go server.mta.RelayMessage(en)
}

func (server *smtpServer) maildropForAddress(addr mail.Address) string {
	domain := smtp.DomainForAddress(addr)
	for _, s := range server.config.Servers {
		if domain == s.Domain {
			return s.MaildropPath
		}
	}

	return ""
}
