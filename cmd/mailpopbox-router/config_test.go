// mailpopbox
// Copyright 2025 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"testing"
)

func TestInvalidConfigs(t *testing.T) {
	configs := []Config{
		// Missing Email.
		{
			Monitor: []MonitorConfig{{
				PollIntervalSeconds: 10,
				Source: ServerConfig{
					Type:       ServerTypePOP3,
					ServerAddr: "localhost:995",
				},
				Destination: ServerConfig{
					Type:  ServerTypeGmail,
					Email: "here",
				},
			}},
		},
		{
			Monitor: []MonitorConfig{{
				PollIntervalSeconds: 10,
				Source: ServerConfig{
					Type:       ServerTypePOP3,
					Email:      "here",
					ServerAddr: "localhost:995",
				},
				Destination: ServerConfig{Type: ServerTypeGmail},
			}},
		},
		// Missing PollIntervalSeconds.
		{
			Monitor: []MonitorConfig{{
				Source: ServerConfig{
					Type:       ServerTypePOP3,
					Email:      "here",
					ServerAddr: "localhost:995",
				},
				Destination: ServerConfig{
					Type:  ServerTypeGmail,
					Email: "here",
				},
			}},
		},
		// Missing ServerAddr.
		{
			Monitor: []MonitorConfig{{
				PollIntervalSeconds: 10,
				Source: ServerConfig{
					Type:  ServerTypePOP3,
					Email: "here",
				},
				Destination: ServerConfig{
					Type:  ServerTypeGmail,
					Email: "here",
				},
			}},
		},
		// Invalid server types.
		{
			Monitor: []MonitorConfig{{
				PollIntervalSeconds: 10,
				Source: ServerConfig{
					Type:       ServerType("pop5"),
					Email:      "here",
					ServerAddr: "localhost:995",
				},
				Destination: ServerConfig{
					Type:  ServerTypeGmail,
					Email: "here",
				},
			}},
		},
		{
			Monitor: []MonitorConfig{{
				PollIntervalSeconds: 10,
				Source: ServerConfig{
					Type:       ServerTypePOP3,
					Email:      "here",
					ServerAddr: "localhost:995",
				},
				Destination: ServerConfig{
					Type:  ServerType("google-inbox-rip"),
					Email: "here",
				},
			}},
		},
	}
	for i, cfg := range configs {
		err := cfg.Validate()
		if err == nil {
			t.Errorf("Expected error for config #%d: %#v", i, cfg)
		}
	}
}
