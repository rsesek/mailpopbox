// mailpopbox
// Copyright 2020 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"io/ioutil"
	"net/textproto"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func TestReset(t *testing.T) {
	mbox := mailbox{
		messages: []message{
			{"msg1", 1, 4, false},
			{"msg2", 2, 4, false},
		},
	}

	msg := mbox.GetMessage(1)
	if msg == nil {
		t.Errorf("failed to GetMessage #1")
	}

	if msg.Deleted() {
		t.Errorf("message %v should not be deleted", msg)
	}

	if err := mbox.Delete(msg); err != nil {
		t.Error(err)
	}

	if !msg.Deleted() {
		t.Errorf("message %v should be deleted", msg)
	}

	mbox.Reset()

	if msg.Deleted() {
		t.Errorf("reset did not un-delete message %v", msg)
	}
}

func TestOpenMailboxAuth(t *testing.T) {
	dir, err := ioutil.TempDir("", "maildrop")
	if err != nil {
		t.Errorf("Failed to create temp dir: %v", err)
		return
	}
	defer os.RemoveAll(dir)

	s := &pop3Server{
		config: Config{
			Servers: []Server{
				{
					Domain:          "example.com",
					MailboxPassword: "letmein",
					MaildropPath:    dir,
				},
				{
					Domain:          "test.net",
					MailboxPassword: "open-sesame",
					MaildropPath:    dir,
				},
			},
		},
		log: zap.NewNop(),
	}

	cases := []struct {
		user, pass string
		ok         bool
	}{
		{"mailbox@example.com", "letmein", true},
		{"mailbox@test.net", "open-sesame", true},
		{"mailbox@example.com", "open-sesame", false},
		{"test@test.net", "open-sesame", false},
		{"mailbox@an-example.net", "letmein", false},
	}
	for i, c := range cases {
		mb, err := s.OpenMailbox(c.user, c.pass)
		got := (mb != nil && err == nil)
		if got != c.ok {
			t.Errorf("Expected error=%v for case %d (%#v), got %v (error=%v, mb=%v)", c.ok, i, c, got, err, mb)
		}
	}
}

func TestBasicListener(t *testing.T) {
	dir, err := ioutil.TempDir("", "maildrop")
	if err != nil {
		t.Errorf("Failed to create temp dir: %v", err)
		return
	}
	defer os.RemoveAll(dir)

	s := &pop3Server{
		config: Config{
			POP3Port: 9648,
			Hostname: "example.com",
			Servers: []Server{
				{
					Domain:       "example.com",
					MaildropPath: dir,
				},
			},
		},
		log: zap.NewNop(),
	}

	go s.run()

	conn, err := textproto.Dial("tcp", "localhost:9648")
	if err != nil {
		t.Errorf("Failed to dial test server: %v", err)
		return
	}

	_, err = conn.ReadLine()
	if err != nil {
		t.Errorf("Failed to read line: %v", err)
		return
	}
}

func TestMailbox(t *testing.T) {
	dir, err := ioutil.TempDir("", "maildrop")
	if err != nil {
		t.Errorf("Failed to create temp dir: %v", err)
		return
	}
	defer os.RemoveAll(dir)

	// Create the first message.
	f, err := os.Create(filepath.Join(dir, "a.msg"))
	if err != nil {
		t.Errorf("Failed to create a.msg: %v", err)
		return
	}
	for i := 0; i < 1024*10; i++ {
		buf := []byte{'a'}
		_, err = f.Write(buf)
		if err != nil {
			t.Errorf("Failed to write a.msg: %v", err)
		}
	}
	f.Close()

	// Create the second message.
	f, err = os.Create(filepath.Join(dir, "b.msg"))
	if err != nil {
		t.Errorf("Failed to create b.msg: %v", err)
		return
	}
	for i := 0; i < 1024*3; i++ {
		buf := []byte{'z'}
		_, err = f.Write(buf)
		if err != nil {
			t.Errorf("Failed to write z.msg: %v", err)
		}
	}
	f.Close()

	s := &pop3Server{
		config: Config{
			Servers: []Server{
				{
					Domain:          "example.com",
					MailboxPassword: "letmein",
					MaildropPath:    dir,
				},
			},
		},
		log: zap.NewNop(),
	}

	// Test message metadata.
	mb, err := s.OpenMailbox("mailbox@example.com", "letmein")
	if err != nil {
		t.Errorf("Failed to open mailbox: %v", err)
	}

	msgs, err := mb.ListMessages()
	if err != nil {
		t.Errorf("Failed to list messages: %v", err)
	}

	if want, got := 2, len(msgs); want != got {
		t.Errorf("Want %d messages, got %d", want, got)
	}

	if mb.GetMessage(0) != nil {
		t.Errorf("Messages should be 1-indexed")
	}
	if mb.GetMessage(3) != nil {
		t.Errorf("Retreived unexpected message")
	}

	if msgs[0] != mb.GetMessage(msgs[0].ID()) {
		t.Errorf("Failed to look up message by ID")
	}

	if want, got := "a", msgs[0].UniqueID(); want != got {
		t.Errorf("Want message #1 unique ID to be %s, got %s", want, got)
	}
	if want, got := 1024*10, msgs[0].Size(); want != got {
		t.Errorf("Want message #1 size to be %v, got %v", want, got)
	}

	if want, got := "b", msgs[1].UniqueID(); want != got {
		t.Errorf("Want message #2 unique ID to be %s, got %s", want, got)
	}
	if want, got := 1024*3, msgs[1].Size(); want != got {
		t.Errorf("Want message #2 size to be %v, got %v", want, got)
	}

	// Test message contents.
	rc, err := mb.Retrieve(msgs[0])
	if err != nil {
		t.Errorf("Failed to retrieve message: %v", err)
	}
	rc.Close()

	// Test deletion marking and reset.
	err = mb.Delete(msgs[1])
	if err != nil {
		t.Errorf("Failed to mark message #2 for deletion: %v", err)
	}

	if !msgs[1].Deleted() {
		t.Errorf("Message should be marked for deletion and isn't")
	}

	mb.Reset()

	if msgs[1].Deleted() {
		t.Errorf("Message is marked for deletion and shouldn't be")
	}

	// Test deletion for real.
	err = mb.Delete(msgs[0])
	if err != nil {
		t.Errorf("Failed to mark message for deletion: %v", err)
	}

	err = mb.Close()
	if err != nil {
		t.Errorf("Failed to close mailbox: %v", err)
	}

	mb, err = s.OpenMailbox("mailbox@example.com", "letmein")
	if err != nil {
		t.Errorf("Failed to re-open mailbox: %v", err)
	}

	msgs, err = mb.ListMessages()
	if err != nil {
		t.Errorf("Failed to list messages: %v", err)
	}

	if want, got := 1, len(msgs); want != got {
		t.Errorf("Number of messages should be %d, got %d", want, got)
	}

	if want, got := "b", msgs[0].UniqueID(); want != got {
		t.Errorf("Message Unique ID should be %s, got %s", want, got)
	}
}
