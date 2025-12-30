// mailpopbox
// Copyright 2025 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"encoding/base64"

	"go.uber.org/zap"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

type Destination interface {
	// Connect attempts to dial the Destination and return a stateful connection.
	Connect(context.Context) (DestinationConnection, error)
}

type DestinationConnection interface {
	// AddMessage stores the raw RFC 2822 message body in the destination mail
	// server.
	AddMessage([]byte) error
	// Close releases any connection resources on the Destination.
	Close() error
}

func NewDestination(config ServerConfig, auth OAuthServer, log *zap.Logger) Destination {
	switch config.Type {
	case ServerTypeGmail:
		return &gmailDestination{
			c:    config,
			auth: auth,
			log:  log,
		}
	default:
		panic("Unsupported destination server type")
	}
}

type gmailDestination struct {
	c    ServerConfig
	auth OAuthServer
	log  *zap.Logger

	ctx context.Context
	svc *gmail.Service
}

func (d *gmailDestination) Connect(ctx context.Context) (DestinationConnection, error) {
	tokenQ := <-d.auth.GetTokenForUser(ctx, d.c.Email)
	if tokenQ.Error != nil {
		return nil, tokenQ.Error
	}

	auth := option.WithHTTPClient(d.auth.MakeClient(ctx, tokenQ.Token))
	svc, err := gmail.NewService(ctx, auth, option.WithUserAgent("mailpopbox-router"))
	if err != nil {
		return nil, err
	}
	d2 := *d
	d2.ctx = ctx
	d2.svc = svc
	return &d2, nil
}

func (d *gmailDestination) AddMessage(msg []byte) error {
	enc := base64.RawURLEncoding.EncodeToString(msg)
	call := d.svc.Users.Messages.Insert("me", &gmail.Message{
		LabelIds: []string{"INBOX", "UNREAD"},
		Raw:      enc,
	})
	result, err := call.Do()
	d.log.Info("Result", zap.Any("result", result), zap.Error(err))
	return err
}

func (d *gmailDestination) Close() error {
	return nil
}
