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

	"github.com/uber-go/zap"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s config.json\n", os.Args[0])
		os.Exit(1)
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

	log := zap.New(zap.NewTextEncoder())

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
