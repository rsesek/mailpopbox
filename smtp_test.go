// mailpopbox
// Copyright 2020 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
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

type testMTA struct {
	relayed chan smtp.Envelope
}

func (m *testMTA) RelayMessage(en smtp.Envelope) {
	m.relayed <- en
}

func newTestMTA() *testMTA {
	return &testMTA{
		relayed: make(chan smtp.Envelope),
	}
}

func TestBasicRelay(t *testing.T) {
	mta := newTestMTA()
	server := smtpServer{
		mta: mta,
		log: zap.NewNop(),
	}

	buf := new(bytes.Buffer)
	fmt.Fprintln(buf, "From: <mailbox@example.com>\r")
	fmt.Fprintln(buf, "To: <dest@another.net>\r")
	fmt.Fprintf(buf, "Subject: Basic relay\n\n")
	fmt.Fprintln(buf, "This is a basic relay message")

	en := smtp.Envelope{
		MailFrom: mail.Address{Address: "mailbox@example.com"},
		RcptTo:   []mail.Address{{Address: "dest@another.com"}},
		Data:     buf.Bytes(),
		ID:       "id1",
	}

	server.RelayMessage(en, en.MailFrom.Address)

	relayed := <-mta.relayed

	if !bytes.Equal(relayed.Data, en.Data) {
		t.Errorf("Relayed message data does not match")
	}
}

func TestSendAsRelay(t *testing.T) {
	mta := newTestMTA()
	server := smtpServer{
		mta: mta,
		log: zap.NewNop(),
	}

	buf := new(bytes.Buffer)
	fmt.Fprintln(buf, "Received: msg from wherever")
	fmt.Fprintln(buf, "From: <mailbox@example.com>")
	fmt.Fprintln(buf, "To: <valid@dest.xyz>")
	fmt.Fprintf(buf, "Subject: Send-as relay [sendas:source]\n\n")
	fmt.Fprintln(buf, "We've switched the senders!")

	en := smtp.Envelope{
		MailFrom: mail.Address{Address: "mailbox@example.com"},
		RcptTo:   []mail.Address{{Address: "valid@dest.xyz"}},
		Data:     buf.Bytes(),
		ID:       "id1",
	}

	server.RelayMessage(en, en.MailFrom.Address)

	relayed := <-mta.relayed

	replaced := "source@example.com"
	original := "mailbox@example.com"

	if want, got := replaced, relayed.MailFrom.Address; want != got {
		t.Errorf("Want mail to be from %q, got %q", want, got)
	}

	if want, got := 1, len(relayed.RcptTo); want != got {
		t.Errorf("Want %d recipient, got %d", want, got)
	}
	if want, got := "valid@dest.xyz", relayed.RcptTo[0].Address; want != got {
		t.Errorf("Unexpected RcptTo %q", got)
	}

	msg := string(relayed.Data)

	if strings.Index(msg, original) != -1 {
		t.Errorf("Should not find %q in message %q", original, msg)
	}

	if strings.Index(msg, "\nFrom: <source@example.com>\n") == -1 {
		t.Errorf("Could not find From: header in message %q", msg)
	}

	if strings.Index(msg, "\nSubject: Send-as relay \n") == -1 {
		t.Errorf("Could not find modified Subject: header in message %q", msg)
	}
}
