// mailpopbox
// Copyright 2020 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"bytes"
	"io/ioutil"
	"net/mail"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"src.bluestatic.org/mailpopbox/smtp"
)

func TestVerifyAddress(t *testing.T) {
	dir, err := ioutil.TempDir("", "maildrop")
	if err != nil {
		t.Errorf("Failed to create temp dir: %v", err)
		return
	}
	defer os.RemoveAll(dir)

	s := smtpServer{
		config: Config{
			Hostname: "mx.example.com",
			Servers: []Server{
				{
					Domain:       "example.com",
					MaildropPath: dir,
				},
			},
		},
		log: zap.NewNop(),
	}

	if s.VerifyAddress(mail.Address{Address: "example@example.com"}) != smtp.ReplyOK {
		t.Errorf("Valid mailbox is not reported to be valid")
	}
	if s.VerifyAddress(mail.Address{Address: "mailbox@example.com"}) != smtp.ReplyOK {
		t.Errorf("Valid mailbox is not reported to be valid")
	}
	if s.VerifyAddress(mail.Address{Address: "hello@other.net"}) == smtp.ReplyOK {
		t.Errorf("Invalid mailbox reports to be valid")
	}
	if s.VerifyAddress(mail.Address{Address: "hello@mx.example.com"}) == smtp.ReplyOK {
		t.Errorf("Invalid mailbox reports to be valid")
	}
	if s.VerifyAddress(mail.Address{Address: "unknown"}) == smtp.ReplyOK {
		t.Errorf("Invalid mailbox reports to be valid")
	}
}

func TestMessageDelivery(t *testing.T) {
	dir, err := ioutil.TempDir("", "maildrop")
	if err != nil {
		t.Errorf("Failed to create temp dir: %v", err)
		return
	}
	defer os.RemoveAll(dir)

	s := smtpServer{
		config: Config{
			Hostname: "mx.example.com",
			Servers: []Server{
				{
					Domain:       "example.com",
					MaildropPath: dir,
				},
			},
		},
		log: zap.NewNop(),
	}

	env := smtp.Envelope{
		MailFrom: mail.Address{Address: "sender@mail.net"},
		RcptTo:   []mail.Address{{Address: "receive@example.com"}},
		Data:     []byte("Hello, world"),
		ID:       "msgid",
	}

	if rl := s.DeliverMessage(env); rl != nil {
		t.Errorf("Failed to deliver message: %v", rl)
	}

	f, err := os.Open(filepath.Join(dir, "msgid.msg"))
	if err != nil {
		t.Errorf("Failed to open delivered message: %v", err)
	}
	defer f.Close()

	data, err := ioutil.ReadAll(f)
	if err != nil {
		t.Errorf("Failed to read message: %v", err)
	}

	if !bytes.Contains(data, env.Data) {
		t.Errorf("Could not find expected data in message")
	}
}

func TestAuthenticate(t *testing.T) {
	server := smtpServer{
		config: Config{
			Servers: []Server{
				Server{
					Domain:          "domain1.net",
					MailboxPassword: "d1",
				},
				Server{
					Domain:          "domain2.xyz",
					MailboxPassword: "d2",
				},
			},
		},
	}

	authTests := []struct {
		authz, authc, passwd string
		ok                   bool
	}{
		{"foo@domain1.net", "mailbox@domain1.net", "d1", true},
		{"", "mailbox@domain1.net", "d1", true},
		{"foo@domain2.xyz", "mailbox@domain1.xyz", "d1", false},
		{"foo@domain2.xyz", "mailbox@domain1.xyz", "d2", false},
		{"foo@domain2.xyz", "mailbox@domain2.xyz", "d2", true},
		{"invalid", "mailbox@domain2.xyz", "d2", false},
		{"", "mailbox@domain2.xyz", "d2", true},
		{"", "", "", false},
	}

	for i, test := range authTests {
		actual := server.Authenticate(test.authz, test.authc, test.passwd)
		if actual != test.ok {
			t.Errorf("Test %d, got %v, expected %v", i, actual, test.ok)
		}
	}
}
