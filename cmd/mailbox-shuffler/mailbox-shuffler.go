// mailpopbox
// Copyright 2025 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"src.bluestatic.org/mailpopbox/pkg/version"

	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
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

	token, err := getToken(ctx, log, &config, oauthConfig)
	if err != nil {
		log.Fatal("Failed to get OAuth token", zap.Error(err))
	}

	auth := option.WithHTTPClient(oauthConfig.Client(ctx, token))
	client, err := gmail.NewService(ctx, auth, option.WithUserAgent("mailbox-shuffler"))
	if err != nil {
		log.Fatal("Failed to create GMail client", zap.Error(err))
	}

	rawEnc := base64.RawURLEncoding.EncodeToString(rawMsg)

	call := client.Users.Messages.Insert("me", &gmail.Message{
		LabelIds: []string{"INBOX", "UNREAD"},
		Raw:      rawEnc,
	})
	result, err := call.Do()
	log.Info("Result", zap.Any("result", result), zap.Error(err))
}

func getToken(ctx context.Context, log *zap.Logger, config *Config, oauthConfig *oauth2.Config) (*oauth2.Token, error) {
	var token *oauth2.Token
	f, err := os.Open(config.OAuthServer.TokenStore)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	} else if f != nil {
		defer f.Close()
		if err = json.NewDecoder(f).Decode(&token); err != nil {
			return nil, err
		} else {
			return token, nil
		}
	}

	srv := &http.Server{Addr: "localhost:8025"}
	oauthConfig.RedirectURL = fmt.Sprintf("http://%s", srv.Addr)

	srvCtx, cancel := context.WithCancel(ctx)
	s := RunOAuthServer(srvCtx, srv, oauthConfig, zap.L())

	authURL, ch := s.AuthorizeToken()
	fmt.Printf("Authorize the application at this URL:\n\t%s\n", authURL)

	code := <-ch
	cancel()

	log.Info("Got code", zap.String("code", code))

	token, err = oauthConfig.Exchange(ctx, code)
	if err != nil {
		return nil, err
	}

	f, err = os.Create(config.OAuthServer.TokenStore)
	if err != nil {
		return token, err
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(token); err != nil {
		return token, err
	}

	return token, nil
}
