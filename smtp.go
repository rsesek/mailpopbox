package main

import (
	"crypto/tls"
	"fmt"
	"net"

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
	config Config
	rc     chan error
}

func (server *smtpServer) run() {
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
	return nil
}

func (server *smtpServer) OnEHLO() *smtp.ReplyLine {
	return nil
}

func (server *smtpServer) OnMessageDelivered(en smtp.Envelope) *smtp.ReplyLine {
	fmt.Printf("MSG: %#v\n%s\n", en, string(en.Data))
	return nil
}