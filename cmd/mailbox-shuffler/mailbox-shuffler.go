// mailpopbox
// Copyright 2025 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"src.bluestatic.org/mailpopbox/pkg/version"

	"go.uber.org/zap"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s config.json\n", os.Args[0])
		os.Exit(1)
	}

	if os.Args[1] == "version" {
		fmt.Print(version.VersionString)
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

	log.Info("starting mailbox-shuffler")

	rawMsg, err := os.ReadFile("dev/test.msg")
	if err != nil {
		log.Fatal("Failed to read test mesage", zap.Error(err))
	}

	clientSecret, err := os.ReadFile(config.OAuthServer.CredentialsPath)
	if err != nil {
		log.Fatal("Failed to read client secret", zap.Error(err))
	}
	oauthConfig, err := google.ConfigFromJSON(clientSecret, gmail.GmailInsertScope)
	if err != nil {
		log.Fatal("Failed to load API config", zap.Error(err))
	}
	ctx := context.Background()

	oauthServer := RunOAuthServer(ctx, config.OAuthServer, oauthConfig, log)

	dest := NewDestination(config.Monitor[0].Destination, oauthServer, log)
	destConn, err := dest.Connect(ctx)
	if err != nil {
		log.Fatal("Failed to connect to destination", zap.Error(err))
	}

	err = destConn.AddMessage(rawMsg)
	if err != nil {
		log.Fatal("Failed to insert message", zap.Error(err))
	}
}
