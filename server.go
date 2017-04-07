package main

import (
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/uber-go/zap"
)

type ServerControlMessage int

const (
	ServerControlFatalError ServerControlMessage = iota
	ServerControlRestart
)

func RunAcceptLoop(l net.Listener, c chan<- net.Conn, log zap.Logger) {
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
