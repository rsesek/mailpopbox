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
	"log"
	"net/http"
	"os"

	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

const (
	tokenFile = "dev/token.json"
)

func main() {
	rawMsg, err := os.ReadFile("dev/test.msg")
	if err != nil {
		log.Fatalf("Failed to read test mesage: %v", err)
	}

	clientSecret, err := os.ReadFile("dev/client_secret.json")
	if err != nil {
		log.Fatalf("Failed to read client secret: %v", err)
	}
	config, err := google.ConfigFromJSON(clientSecret, gmail.GmailInsertScope)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	ctx := context.Background()

	token, err := getToken(ctx, config)
	if err != nil {
		log.Fatalf("Failed to get OAuth token: %v", err)
	}

	auth := option.WithHTTPClient(config.Client(ctx, token))
	client, err := gmail.NewService(ctx, auth, option.WithUserAgent("mailbox-shuffler"))
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	rawEnc := base64.RawURLEncoding.EncodeToString(rawMsg)

	call := client.Users.Messages.Insert("me", &gmail.Message{
		LabelIds: []string{"INBOX", "UNREAD"},
		Raw:      rawEnc,
	})
	result, err := call.Do()
	log.Printf("Result: %#v", result)
	log.Printf("Err: %#v", err)
}

func getToken(ctx context.Context, config *oauth2.Config) (*oauth2.Token, error) {
	var token *oauth2.Token
	f, err := os.Open(tokenFile)
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
	config.RedirectURL = fmt.Sprintf("http://%s", srv.Addr)

	srvCtx, cancel := context.WithCancel(ctx)
	s := RunOAuthServer(srvCtx, srv, config, zap.L())

	authURL, ch := s.AuthorizeToken()
	log.Printf("Authorize the application at this URL:\n\t%s", authURL)

	code := <-ch
	cancel()

	log.Printf("Got code: %q", code)

	token, err = config.Exchange(ctx, code)
	if err != nil {
		return nil, err
	}

	f, err = os.Create(tokenFile)
	if err != nil {
		return token, err
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(token); err != nil {
		return token, err
	}

	return token, nil
}
