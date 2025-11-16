// mailpopbox
// Copyright 2025 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package main

import "go.uber.org/zap"

type Destination interface {
	// AddMessage stores the raw RFC 2822 message body in the destination mail
	// server.
	AddMessage([]byte) error
	// Close releases any connection resources on the Destination.
	Close() error
}

func NewDestination(config ServerConfig, auth *OAuthServer, log *zap.Logger) Destination {
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
	auth *OAuthServer
	log  *zap.Logger
}

func (d *gmailDestination) AddMessage(msg []byte) error {
	return nil
}

func (d *gmailDestination) Close() error {
	return nil
}
