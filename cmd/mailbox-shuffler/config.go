// mailpopbox
// Copyright 2025 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"fmt"
	"time"
)

type ServerType string

const (
	ServerTypePOP3  ServerType = "pop3"
	ServerTypeGmail ServerType = "gmail"
)

// ServerConfig stores the connection information for an email
// server.
type ServerConfig struct {
	Type       ServerType
	ServerAddr string
	UseTLS     bool

	Email string

	Password string
}

// MonitorConfig controls how to move messages between a source and
// destination email server.
type MonitorConfig struct {
	Source       ServerConfig
	Destination  ServerConfig
	PollInterval time.Duration
}

// OAuthServerConfig stores the configuration for an OAuth 2.0
// application for authenticating to GMail.
type OAuthServerConfig struct {
	RedirectURL             string
	ListenAddr              string
	CredentialsPath         string
	TokenStore              string
	TLSCertPath, TLSKeyPath string
}

// Config is the top-level config of mailbox-shuffler.
type Config struct {
	Monitor     []MonitorConfig
	OAuthServer OAuthServerConfig
}

func (c *Config) Validate() error {
	for _, mon := range c.Monitor {
		if mon.Source.Email == "" || mon.Destination.Email == "" {
			return fmt.Errorf("Monitor source/destination email missing")
		}
		if err := validateSource(mon.Source); err != nil {
			return fmt.Errorf("Invalid Source: %w", err)
		}
		if err := validateDest(mon.Destination); err != nil {
			return fmt.Errorf("Invalid Destination: %w", err)
		}
	}
	return nil
}

func validateSource(c ServerConfig) error {
	if c.Type != ServerTypePOP3 {
		return fmt.Errorf("Invalid Type: %q", c.Type)
	}
	if c.ServerAddr == "" {
		return fmt.Errorf("Missing ServerAddr")
	}
	return nil
}

func validateDest(c ServerConfig) error {
	if c.Type != ServerTypeGmail {
		return fmt.Errorf("Invalid Type: %q", c.Type)
	}
	return nil
}
