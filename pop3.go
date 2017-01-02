package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path"

	"github.com/uber-go/zap"

	"src.bluestatic.org/mailpopbox/pop3"
)

func runPOP3Server(config Config, log zap.Logger) <-chan error {
	server := pop3Server{
		config: config,
		rc:     make(chan error),
		log:    log.With(zap.String("server", "pop3")),
	}
	go server.run()
	return server.rc
}

type pop3Server struct {
	config Config
	rc     chan error
	log    zap.Logger
}

func (server *pop3Server) run() {
	for _, s := range server.config.Servers {
		if err := os.Mkdir(s.MaildropPath, 0700); err != nil && !os.IsExist(err) {
			server.log.Error("failed to open maildrop", zap.Error(err))
			server.rc <- err
		}
	}

	tlsConfig, err := server.config.GetTLSConfig()
	if err != nil {
		server.log.Error("failed to configure TLS", zap.Error(err))
		server.rc <- err
		return
	}

	addr := fmt.Sprintf(":%d", server.config.POP3Port)
	server.log.Info("starting server", zap.String("address", addr))

	var l net.Listener
	if tlsConfig == nil {
		l, err = net.Listen("tcp", addr)
	} else {
		l, err = tls.Listen("tcp", addr, tlsConfig)
	}
	if err != nil {
		server.log.Error("listen", zap.Error(err))
		server.rc <- err
		return
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			server.log.Error("accept", zap.Error(err))
			server.rc <- err
			break
		}

		go pop3.AcceptConnection(conn, server, server.log)
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
	files, err := ioutil.ReadDir(maildrop)
	if err != nil {
		// TODO: hide error, log instead
		return nil, err
	}

	mb := &mailbox{
		messages: make([]message, 0, len(files)),
	}

	i := 0
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		msg := message{
			filename: path.Join(maildrop, file.Name()),
			index:    i,
			size:     file.Size(),
		}
		mb.messages = append(mb.messages, msg)
		i++
	}

	return mb, nil
}

type mailbox struct {
	messages []message
}

type message struct {
	filename string
	index    int
	size     int64
	deleted  bool
}

func (m message) UniqueID() string {
	l := len(m.filename)
	return path.Base(m.filename[:l-len(".msg")])
}

func (m message) ID() int {
	return m.index + 1
}

func (m message) Size() int {
	return int(m.size)
}

func (m message) Deleted() bool {
	return m.deleted
}

func (mb *mailbox) ListMessages() ([]pop3.Message, error) {
	msgs := make([]pop3.Message, len(mb.messages))
	for i := 0; i < len(mb.messages); i++ {
		msgs[i] = &mb.messages[i]
	}
	return msgs, nil
}

func (mb *mailbox) GetMessage(id int) pop3.Message {
	if id > len(mb.messages) {
		return nil
	}
	return &mb.messages[id-1]
}

func (mb *mailbox) Retrieve(msg pop3.Message) (io.ReadCloser, error) {
	filename := msg.(*message).filename
	return os.Open(filename)
}

func (mb *mailbox) Delete(msg pop3.Message) error {
	msg.(*message).deleted = true
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
	for i, _ := range mb.messages {
		mb.messages[i].deleted = false
	}
}
