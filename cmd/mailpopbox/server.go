// mailpopbox
// Copyright 2020 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"net"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
)

type ServerControlMessage int

const (
	ServerControlFatalError ServerControlMessage = iota
	ServerControlRestart
)

func RunAcceptLoop(l net.Listener, c chan<- net.Conn, log *zap.Logger) {
	for {
		conn, err := l.Accept()
		if err != nil {
			log.Error("accept", zap.Error(err))
			close(c)
			return
		}

		c <- conn
	}
}

func CreateReloadSignal() <-chan os.Signal {
	reloadChan := make(chan os.Signal, 1)
	signal.Notify(reloadChan, syscall.SIGHUP)
	return reloadChan
}
