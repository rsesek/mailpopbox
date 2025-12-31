// mailpopbox
// Copyright 2025 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"go.uber.org/zap"
)

type Monitor struct {
	c   MonitorConfig
	log *zap.Logger

	src Source
	dst Destination
}

func NewMontior(config MonitorConfig, auth OAuthServer, log *zap.Logger) *Monitor {
	log = log.With(zap.String("source", config.Source.LogDescription()),
		zap.String("dest", config.Destination.LogDescription()))
	return &Monitor{
		c:   config,
		log: log,
		src: NewSource(config.Source, auth, log),
		dst: NewDestination(config.Destination, auth, log),
	}
}

func (m *Monitor) Start(ctx context.Context) error {
	if err := m.runOnce(ctx); err != nil {
		m.log.Error("Failed to start monitor", zap.Error(err))
		return err
	}

	go m.run(ctx)

	return nil
}

func (m *Monitor) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			m.log.Info("Monitor stopping")
			return
		case <-time.After(m.c.PollIntervalSeconds * time.Second):
			m.runOnce(ctx)
		}
	}
}

func (m *Monitor) runOnce(ctx context.Context) error {
	m.log.Info("Polling for messages")

	if err := m.src.Connect(); err != nil {
		return fmt.Errorf("Failed to connect to source: %w", err)
	}
	dstConn, err := m.dst.Connect(ctx)
	if err != nil {
		return fmt.Errorf("Failed to connect to dest: %w", err)
	}

	msgs, err := m.src.GetMessages()
	if err != nil {
		return fmt.Errorf("Failed to list messages: %w", err)
	}
	for _, msg := range msgs {
		log := m.log.With(zap.String("id", msg.ID()))
		log.Info("Transferring message to destination")
		err := m.transferMessageTo(msg, dstConn)
		if err == nil {
			log.Info("Successfully transferred message")
		} else {
			log.Error("Failed to transfer message", zap.Error(err))
		}
	}

	if err := m.src.Close(); err != nil {
		return fmt.Errorf("Failed to close source: %w", err)
	}
	if err := dstConn.Close(); err != nil {
		return fmt.Errorf("Failed to close dest: %w", err)
	}

	return nil
}

func (m *Monitor) transferMessageTo(msg Message, dst DestinationConnection) error {
	r, err := msg.Content()
	if err != nil {
		return fmt.Errorf("Failed to get message content: %w", err)
	}
	defer r.Close()

	body, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("Failed to read message content: %w", err)
	}

	content := getReceivedInfo(m.c, time.Now())
	content = append(content, body...)

	if err = dst.AddMessage(content); err == nil {
		if err = msg.Delete(); err != nil {
			return fmt.Errorf("Failed to mark source message as deleted: %w", err)
		}
		return nil
	} else {
		return fmt.Errorf("Failed to add message to destination: %w", err)
	}
}

func getReceivedInfo(cfg MonitorConfig, t time.Time) []byte {
	line := fmt.Sprintf(
		"Received: from <%s> (via %s) by mailpopbox-router\r\n        for <%s> (via %s); %s\r\n",
		cfg.Source.Email, cfg.Source.Type,
		cfg.Destination.Email, cfg.Destination.Type,
		t.Format(time.RFC1123Z))
	return []byte(line)
}
