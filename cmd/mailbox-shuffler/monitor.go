// mailpopbox
// Copyright 2025 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"

	"go.uber.org/zap"
)

type Monitor struct {
	c   MonitorConfig
	log *zap.Logger
}

func NewMontior(config MonitorConfig, log *zap.Logger) *Monitor {
	return &Monitor{
		c:   config,
		log: log,
	}
}

func (m *Monitor) Start(ctx context.Context) {
}
