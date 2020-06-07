// mailpopbox
// Copyright 2020 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package smtp

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"strings"
	"testing"

	"go.uber.org/zap"
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

	host, port, _ := net.SplitHostPort(l.Addr().String())
	relayMessageToHost(s, env, zap.NewNop(), env.RcptTo[0].Address, host, port)

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

func TestDeliveryFailureMessage(t *testing.T) {
	s := &deliveryServer{}

	env := Envelope{
		MailFrom:   mail.Address{Address: "from@sender.org"},
		RcptTo:     []mail.Address{{Address: "to@receive.net"}},
		Data:       []byte("Message\n"),
		ID:         "m.willfail",
		EHLO:       "mx.receive.net",
		RemoteAddr: &net.IPAddr{net.IPv4(127, 0, 0, 1), ""},
	}

	errorStr1 := "internal message"
	errorStr2 := "general error 122"
	deliverRelayFailure(s, env, zap.NewNop(), env.RcptTo[0].Address, errorStr1, fmt.Errorf(errorStr2))

	if len(s.messages) != 1 {
		t.Errorf("Expected 1 failure notification, got %d", len(s.messages))
		return
	}

	failure := s.messages[0]

	if failure.RcptTo[0].Address != env.MailFrom.Address {
		t.Errorf("Failure message should be delivered to sender %s, actually %s", env.MailFrom.Address, failure.RcptTo[0].Address)
	}

	// Read the failure message.
	buf := bytes.NewBuffer(failure.Data)
	msg, err := mail.ReadMessage(buf)
	if err != nil {
		t.Errorf("Failed to read message: %v", err)
		return
	}

	// Parse out the Content-Type to get the multipart boundary string.
	mediatype, mtheaders, err := mime.ParseMediaType(msg.Header["Content-Type"][0])
	if err != nil {
		t.Errorf("Failed to parse MIME headers: %v", err)
		return
	}

	expected := "multipart/report"
	if mediatype != expected {
		t.Errorf("Expected MIME type of %q, got %q", expected, mediatype)
	}

	expected = "delivery-status"
	if mtheaders["report-type"] != expected {
		t.Errorf("Expected report-type of %q, got %q", expected, mtheaders["report-type"])
	}

	boundary := mtheaders["boundary"]

	expected = "Delivery Status Notification (Failure)"
	if msg.Header["Subject"][0] != expected {
		t.Errorf("Subject did not match %q, got %q", expected, mtheaders["Subject"])
	}

	if msg.Header["To"][0] != "<"+env.MailFrom.Address+">" {
		t.Errorf("To field does not match %q, got %q", env.MailFrom.Address, msg.Header["To"][0])
	}

	// Parse the multi-part messsage.
	mpr := multipart.NewReader(msg.Body, boundary)
	part, err := mpr.NextPart()
	if err != nil {
		t.Errorf("Error reading part 0: %v", err)
		return
	}

	// First part is the human-readable error.
	expected = "text/plain; charset=UTF-8"
	if part.Header["Content-Type"][0] != expected {
		t.Errorf("Part 0 type expected %q, got %q", expected, part.Header["Content-Type"][0])
	}

	content, err := ioutil.ReadAll(part)
	if err != nil {
		t.Errorf("Failed to read part 0 content: %v", err)
		return
	}
	contentStr := string(content)

	if !strings.Contains(contentStr, "Delivery Failure") {
		t.Errorf("Missing Delivery Failure")
	}

	expected = fmt.Sprintf("%s:\n%s", errorStr1, errorStr2)
	if !strings.Contains(contentStr, expected) {
		t.Errorf("Missing error string %q", expected)
	}

	// Second part is the status information.
	part, err = mpr.NextPart()
	if err != nil {
		t.Errorf("Error reading part 1: %v", err)
		return
	}

	expected = "message/delivery-status"
	if part.Header["Content-Type"][0] != expected {
		t.Errorf("Part 1 type expected %q, got %q", expected, part.Header["Content-Type"][0])
	}

	content, err = ioutil.ReadAll(part)
	if err != nil {
		t.Errorf("Failed to read part 1 content: %v", err)
		return
	}
	contentStr = string(content)

	expected = "Original-Envelope-ID: " + env.ID + "\n"
	if !strings.Contains(contentStr, expected) {
		t.Errorf("Missing %q in %q", expected, contentStr)
	}

	expected = "Reporting-UA: " + env.EHLO + "\n"
	if !strings.Contains(contentStr, expected) {
		t.Errorf("Missing %q in %q", expected, contentStr)
	}

	expected = "Reporting-MTA: dns; localhost [127.0.0.1]\n"
	if !strings.Contains(contentStr, expected) {
		t.Errorf("Missing %q in %q", expected, contentStr)
	}

	// Third part is the original message.
	part, err = mpr.NextPart()
	if err != nil {
		t.Errorf("Error reading part 2: %v", err)
		return
	}

	expected = "message/rfc822"
	if part.Header["Content-Type"][0] != expected {
		t.Errorf("Part 2 type expected %q, got %q", expected, part.Header["Content-Type"][0])
	}

	content, err = ioutil.ReadAll(part)
	if err != nil {
		t.Errorf("Failed to read part 2 content: %v", err)
		return
	}

	if !bytes.Equal(content, env.Data) {
		t.Errorf("Byte content of original message does not match")
	}
}
