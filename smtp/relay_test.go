// mailpopbox
// Copyright 2020 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package smtp

import (
	"bytes"
	"net/mail"
	"testing"

	"github.com/uber-go/zap"
)

type deliveryServer struct {
	testServer
	messages []Envelope
}

func (s *deliveryServer) OnMessageDelivered(env Envelope) *ReplyLine {
	s.messages = append(s.messages, env)
	return nil
}

func TestRelayRoundTrip(t *testing.T) {
	s := &deliveryServer{
		testServer: testServer{domain: "receive.net"},
	}
	l := runServer(t, s)
	defer l.Close()

	env := Envelope{
		MailFrom: mail.Address{Address: "from@sender.org"},
		RcptTo:   []mail.Address{{Address: "to@receive.net"}},
		Data:     []byte("~~~Message~~~\n"),
		ID:       "ididid",
	}

	relayMessageToHost(s, env, zap.New(zap.NullEncoder()), env.RcptTo[0].Address, l.Addr().String())

	if len(s.messages) != 1 {
		t.Errorf("Expected 1 message to be delivered, got %d", len(s.messages))
		return
	}

	received := s.messages[0]

	if env.MailFrom.Address != received.MailFrom.Address {
		t.Errorf("Expected MailFrom %s, got %s", env.MailFrom.Address, received.MailFrom.Address)
	}
	if len(received.RcptTo) != 1 {
		t.Errorf("Expected 1 RcptTo, got %d", len(received.RcptTo))
		return
	}
	if env.RcptTo[0].Address != received.RcptTo[0].Address {
		t.Errorf("Expected RcptTo %s, got %s", env.RcptTo[0].Address, received.RcptTo[0].Address)
	}

	if !bytes.HasSuffix(received.Data, env.Data) {
		t.Errorf("Delivered message does not match relayed one. Delivered=%q Relayed=%q", string(env.Data), string(received.Data))
	}
}
