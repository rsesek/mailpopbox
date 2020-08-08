// mailpopbox
// Copyright 2020 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"go.uber.org/zap"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s config.json\n", os.Args[0])
		os.Exit(1)
	}

	if os.Args[1] == "version" {
		fmt.Print(versionString)
		os.Exit(0)
	}

	configFile, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "config file: %s\n", err)
		os.Exit(2)
	}

	var config Config
	if err := json.NewDecoder(configFile).Decode(&config); err != nil {
		fmt.Fprintf(os.Stderr, "config file: %s\n", err)
		os.Exit(3)
	}
	configFile.Close()

	logConfig := zap.NewDevelopmentConfig()
	logConfig.Development = false
	logConfig.DisableStacktrace = true
	logConfig.Level.SetLevel(zap.DebugLevel)
	log, err := logConfig.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "create logger: %v\n", err)
		os.Exit(4)
	}

	log.Info("starting mailpopbox", zap.String("hostname", config.Hostname))

	pop3 := runPOP3Server(config, log)
	smtp := runSMTPServer(config, log)

	for {
		select {
		case cm := <-pop3:
			if cm == ServerControlRestart {
				pop3 = runPOP3Server(config, log)
			} else {
				break
			}
		case <-smtp:
			// smtp never reloads.
			break
		}
	}
}
