// mailpopbox
// Copyright 2025 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"go.uber.org/zap"
)

type testSource struct {
	connectErr error
	getMsgs    func() ([]Message, error)
}

func (s *testSource) Connect() error { return s.connectErr }
func (s *testSource) Reset() error   { return nil }
func (s *testSource) Close() error   { return nil }
func (s *testSource) GetMessages() ([]Message, error) {
	return s.getMsgs()
}

type testMessage struct {
	id         string
	buf        bytes.Buffer
	contentErr error
	deleted    bool
	deleteErr  error
}

func (m *testMessage) ID() string { return m.id }
func (m *testMessage) Content() (io.ReadCloser, error) {
	if m.contentErr != nil {
		return nil, m.contentErr
	}
	return io.NopCloser(&m.buf), nil
}
func (m *testMessage) Delete() error {
	m.deleted = true
	return m.deleteErr
}

type testDestination struct {
	connectErr error
	msgs       [][]byte
	addMsgErr  error
	closeErr   error
}

func (d *testDestination) Connect(context.Context) (DestinationConnection, error) {
	if d.connectErr != nil {
		return nil, d.connectErr
	}
	return d, nil
}
func (d *testDestination) AddMessage(msg []byte) error {
	if d.addMsgErr == nil {
		d.msgs = append(d.msgs, msg)
	}
	return d.addMsgErr
}
func (d *testDestination) Close() error {
	return d.closeErr
}

func makeMonitor(src Source, dst Destination) *Monitor {
	return &Monitor{
		c:   MonitorConfig{PollIntervalSeconds: 1 * time.Hour},
		log: zap.L(),
		src: src,
		dst: dst,
	}
}

var (
	srcConnErr       = fmt.Errorf("source-connect-err")
	dstConnErr       = fmt.Errorf("dest-connect-err")
	getMsgsErr       = fmt.Errorf("get-msgs")
	getMsgContentErr = fmt.Errorf("get-msg-content")
	addMsgErr        = fmt.Errorf("add-msg")
	msgDeleteErr     = fmt.Errorf("delete-msg")
)

func TestSourceConnectError(t *testing.T) {
	s := &testSource{connectErr: srcConnErr}
	d := &testDestination{}
	m := makeMonitor(s, d)
	err := m.Start(t.Context())
	if err == nil {
		t.Errorf("Expected error in Start, got nil")
	} else if !errors.Is(err, srcConnErr) {
		t.Errorf("Error is not %v", srcConnErr)
	}
}

func TestDestConnectError(t *testing.T) {
	s := &testSource{}
	d := &testDestination{connectErr: dstConnErr}
	m := makeMonitor(s, d)
	err := m.Start(t.Context())
	if err == nil {
		t.Errorf("Expected error in Start, got nil")
	} else if !errors.Is(err, dstConnErr) {
		t.Errorf("Error is not %v", dstConnErr)
	}
}

func TestGetMessagesError(t *testing.T) {
	s := &testSource{
		getMsgs: func() ([]Message, error) {
			return nil, getMsgsErr
		},
	}
	d := &testDestination{}
	m := makeMonitor(s, d)
	err := m.Start(t.Context())
	if err == nil {
		t.Errorf("Expected error in Start, got nil")
	} else if !errors.Is(err, getMsgsErr) {
		t.Errorf("Error is not %v", getMsgsErr)
	}
}

func TestMoveOneMessageSuccess(t *testing.T) {
	msg := &testMessage{id: "msg1"}
	fmt.Fprintln(&msg.buf, "Message1")
	s := &testSource{
		getMsgs: func() ([]Message, error) {
			return []Message{msg}, nil
		},
	}
	d := &testDestination{}
	m := makeMonitor(s, d)
	err := m.Start(t.Context())
	if err != nil {
		t.Errorf("Expected monitor to Start successfully")
	}
	if !msg.deleted {
		t.Errorf("Expected source message to be deleted")
	}
	if want, got := 1, len(d.msgs); want != got {
		t.Errorf("Expected %d dest messages, got %d", want, got)
	}
	if !bytes.HasSuffix(d.msgs[0], msg.buf.Bytes()) {
		t.Errorf("Expected dest message to contain %s, got %s", string(msg.buf.Bytes()), string(d.msgs[0]))
	}
}

func TestMoveMessageFailRead(t *testing.T) {
	msg := &testMessage{id: "msg1", contentErr: getMsgContentErr}
	s := &testSource{
		getMsgs: func() ([]Message, error) {
			return []Message{msg}, nil
		},
	}
	d := &testDestination{}
	m := makeMonitor(s, d)
	err := m.Start(t.Context())
	if err != nil {
		t.Errorf("Expected monitor to Start successfully")
	}
	if msg.deleted {
		t.Errorf("Expected source message to remain")
	}
	if want, got := 0, len(d.msgs); want != got {
		t.Errorf("Expected %d dest messages, got %d", want, got)
	}
}

func TestMoveMessageFailWrite(t *testing.T) {
	msg := &testMessage{id: "msg1"}
	fmt.Fprintln(&msg.buf, "Message1")
	s := &testSource{
		getMsgs: func() ([]Message, error) {
			return []Message{msg}, nil
		},
	}
	d := &testDestination{addMsgErr: addMsgErr}
	m := makeMonitor(s, d)
	err := m.Start(t.Context())
	if err != nil {
		t.Errorf("Expected monitor to Start successfully")
	}
	if msg.deleted {
		t.Errorf("Expected source message to remain")
	}
	if want, got := 0, len(d.msgs); want != got {
		t.Errorf("Expected %d dest messages, got %d", want, got)
	}
}

func TestMoveOneMessageDeleteError(t *testing.T) {
	msg := &testMessage{id: "msg1", deleteErr: msgDeleteErr}
	fmt.Fprintln(&msg.buf, "Message1")
	s := &testSource{
		getMsgs: func() ([]Message, error) {
			return []Message{msg}, nil
		},
	}
	d := &testDestination{}
	m := makeMonitor(s, d)
	err := m.Start(t.Context())
	if err != nil {
		t.Errorf("Expected monitor to Start successfully")
	}
	if !msg.deleted {
		t.Errorf("Expected source message to be deleted")
	}
	if want, got := 1, len(d.msgs); want != got {
		t.Errorf("Expected %d dest messages, got %d", want, got)
	}
	if !bytes.HasSuffix(d.msgs[0], msg.buf.Bytes()) {
		t.Errorf("Expected dest message to contain %s, got %s", string(msg.buf.Bytes()), string(d.msgs[0]))
	}
}
