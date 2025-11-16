// mailpopbox
// Copyright 2025 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"

	"src.bluestatic.org/mailpopbox/pkg/pop3"

	"go.uber.org/zap"
)

type Source interface {
	// GetMessages returns the list of available messages on the server. The
	// returned Message objects are only valid until `Close` is called.
	GetMessages() ([]Message, error)
	// Reset attempts to rollback the transaction on the server.
	Reset() error
	// Close releases any connection resources on the Source.
	Close() error
}

type Message interface {
	ID() string
	Content() (io.ReadCloser, error)
	Delete() error
}

// NewSource creates an interface for accessing a message source. The returned
// object is *not* goroutine safe.
func NewSource(config ServerConfig, auth *OAuthServer, log *zap.Logger) Source {
	switch config.Type {
	case ServerTypePOP3:
		return &pop3Source{
			c:   config,
			log: log,
		}
	default:
		panic("Unsupported source server type")
	}
}

type pop3Source struct {
	c   ServerConfig
	log *zap.Logger

	po   pop3.PostOffice
	mbox pop3.Mailbox
}

type pop3Message struct {
	s   *pop3Source
	msg pop3.Message
}

func (m *pop3Message) ID() string                      { return fmt.Sprintf("%d", m.msg.ID()) }
func (m *pop3Message) Content() (io.ReadCloser, error) { return m.s.mbox.Retrieve(m.msg) }
func (m *pop3Message) Delete() error                   { return m.s.mbox.Delete(m.msg) }

var errNotConnected = fmt.Errorf("Source is not connected")

func (s *pop3Source) GetMessages() ([]Message, error) {
	if err := s.connect(); err != nil {
		return nil, err
	}
	pmsgs, err := s.mbox.ListMessages()
	if err != nil {
		return nil, err
	}
	msgs := make([]Message, len(pmsgs))
	for i, pmsg := range pmsgs {
		msgs[i] = &pop3Message{s: s, msg: pmsg}
	}
	return msgs, nil
}

func (s *pop3Source) connect() error {
	if s.po != nil && s.mbox != nil {
		return nil
	}

	var nc net.Conn
	var err error
	if s.c.UseTLS {
		nc, err = tls.Dial("tcp", s.c.ServerAddr, nil)
	} else {
		nc, err = net.Dial("tcp", s.c.ServerAddr)
	}
	if err != nil {
		return err
	}

	po, err := pop3.Connect(nc, s.log)
	if err != nil {
		return err
	}
	s.po = po
	mbox, err := s.po.OpenMailbox(s.c.Email, s.c.Password)
	if err != nil {
		s.po = nil
		return err
	}
	s.mbox = mbox
	return nil
}

func (s *pop3Source) Reset() error {
	if s.po == nil || s.mbox == nil {
		return errNotConnected
	}
	s.mbox.Reset()
	return nil
}

func (s *pop3Source) Close() error {
	if s.po == nil || s.mbox == nil {
		return errNotConnected
	}
	err := s.mbox.Close()
	s.po = nil
	s.mbox = nil
	return err
}
