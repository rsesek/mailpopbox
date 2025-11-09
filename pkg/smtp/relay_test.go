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

func (s *deliveryServer) DeliverMessage(env Envelope) *ReplyLine {
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
	mta := mta{
		server: s,
		log:    zap.NewNop(),
	}
	mta.relayMessageToHost(env, zap.NewNop(), env.RcptTo[0].Address, host, port)

	if want, got := 1, len(s.messages); want != got {
		t.Errorf("Want %d message to be delivered, got %d", want, got)
		return
	}

	received := s.messages[0]

	if want, got := env.MailFrom.Address, received.MailFrom.Address; want != got {
		t.Errorf("Want MailFrom %s, got %s", want, got)
	}
	if want, got := 1, len(received.RcptTo); want != got {
		t.Errorf("Want %d RcptTo, got %d", want, got)
		return
	}
	if want, got := env.RcptTo[0].Address, received.RcptTo[0].Address; want != got {
		t.Errorf("Want RcptTo %s, got %s", want, got)
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
	mta := mta{
		server: s,
		log:    zap.NewNop(),
	}
	mta.deliverRelayFailure(env, zap.NewNop(), env.RcptTo[0].Address, errorStr1, fmt.Errorf(errorStr2))

	if want, got := 1, len(s.messages); want != got {
		t.Errorf("Want %d failure notification, got %d", want, got)
		return
	}

	failure := s.messages[0]

	if want, got := env.MailFrom.Address, failure.RcptTo[0].Address; want != got {
		t.Errorf("Failure message should be delivered to sender %s, actually %s", want, got)
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

	if want, got := "multipart/report", mediatype; want != got {
		t.Errorf("Want MIME type of %q, got %q", want, got)
	}

	if want, got := "delivery-status", mtheaders["report-type"]; want != got {
		t.Errorf("Want report-type of %q, got %q", want, got)
	}

	boundary := mtheaders["boundary"]

	if want, got := "Delivery Status Notification (Failure)", msg.Header["Subject"][0]; want != got {
		t.Errorf("Want Subject field %q, got %q", want, got)
	}

	if want, got := "<"+env.MailFrom.Address+">", msg.Header["To"][0]; want != got {
		t.Errorf("Want To field %q, got %q", want, got)
	}

	// Parse the multi-part messsage.
	mpr := multipart.NewReader(msg.Body, boundary)
	part, err := mpr.NextPart()
	if err != nil {
		t.Errorf("Error reading part 0: %v", err)
		return
	}

	// First part is the human-readable error.
	if want, got := "text/plain; charset=UTF-8", part.Header["Content-Type"][0]; want != got {
		t.Errorf("Part 0 type want %q, got %q", want, got)
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

	if want := fmt.Sprintf("%s:\n%s", errorStr1, errorStr2); !strings.Contains(contentStr, want) {
		t.Errorf("Missing error string %q", want)
	}

	// Second part is the status information.
	part, err = mpr.NextPart()
	if err != nil {
		t.Errorf("Error reading part 1: %v", err)
		return
	}

	if want, got := "message/delivery-status", part.Header["Content-Type"][0]; want != got {
		t.Errorf("Part 1 type want %q, got %q", want, got)
	}

	content, err = ioutil.ReadAll(part)
	if err != nil {
		t.Errorf("Failed to read part 1 content: %v", err)
		return
	}
	contentStr = string(content)

	if want := "Original-Envelope-ID: " + env.ID + "\n"; !strings.Contains(contentStr, want) {
		t.Errorf("Missing %q in %q", want, contentStr)
	}

	if want := "Reporting-UA: " + env.EHLO + "\n"; !strings.Contains(contentStr, want) {
		t.Errorf("Missing %q in %q", want, contentStr)
	}

	if want := "Reporting-MTA: dns; localhost [127.0.0.1]\n"; !strings.Contains(contentStr, want) {
		t.Errorf("Missing %q in %q", want, contentStr)
	}

	// Third part is the original message.
	part, err = mpr.NextPart()
	if err != nil {
		t.Errorf("Error reading part 2: %v", err)
		return
	}

	if want, got := "message/rfc822", part.Header["Content-Type"][0]; want != got {
		t.Errorf("Part 2 type want %q, got %q", want, got)
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
