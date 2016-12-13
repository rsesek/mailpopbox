package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"

	"src.bluestatic.org/mailpopbox/pop3"
)

func runPOP3Server(config Config) <-chan error {
	server := pop3Server{
		config: config,
		rc:     make(chan error),
	}
	go server.run()
	return server.rc
}

type pop3Server struct {
	config Config
	rc     chan error
}

func (server *pop3Server) run() {
	for _, s := range server.config.Servers {
		if err := os.Mkdir(s.MaildropPath, 0700); err != nil && !os.IsExist(err) {
			server.rc <- err
		}
	}

	l, err := net.Listen("tcp", fmt.Sprintf(":%d", server.config.POP3Port))
	if err != nil {
		server.rc <- err
		return
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			server.rc <- err
			break
		}

		go pop3.AcceptConnection(conn, server)
	}
}

func (server *pop3Server) Name() string {
	return server.config.Hostname
}

func (server *pop3Server) OpenMailbox(user, pass string) (pop3.Mailbox, error) {
	for _, s := range server.config.Servers {
		if user == "mailbox@"+s.Domain && pass == s.MailboxPassword {
			return server.openMailbox(s.MaildropPath)
		}
	}
	return nil, errors.New("permission denied")
}

func (server *pop3Server) openMailbox(maildrop string) (*mailbox, error) {
	mb := &mailbox{
		messages: make([]message, 0),
	}
	return mb, nil
}

type mailbox struct {
	messages []message
}

type message struct {
	filename string
	index    int
	size     int
	deleted  bool
}

func (m message) ID() int {
	return m.index + 1
}

func (m message) Size() int {
	return m.size
}

func (mb *mailbox) ListMessages() ([]pop3.Message, error) {
	msgs := make([]pop3.Message, len(mb.messages))
	for i := 0; i < len(mb.messages); i++ {
		msgs[i] = &mb.messages[i]
	}
	return msgs, nil
}

func (mb *mailbox) Retrieve(msg pop3.Message) (io.ReadCloser, error) {
	filename := msg.(*message).filename
	return os.Open(filename)
}

func (mb *mailbox) Delete(msg pop3.Message) error {
	message := msg.(*message)
	if message.deleted {
		return errors.New("already deleted")
	}
	message.deleted = true
	return nil
}

func (mb *mailbox) Close() error {
	for _, message := range mb.messages {
		if message.deleted {
			os.Remove(message.filename)
		}
	}
	return nil
}

func (mb *mailbox) Reset() {
	for _, message := range mb.messages {
		message.deleted = false
	}
}
