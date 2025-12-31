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
	"math/rand/v2"
	"net/http"
	"os"
	"sync"

	"go.uber.org/zap"
	"golang.org/x/oauth2"
)

type GetTokenForUserResult struct {
	Token *oauth2.Token
	Error error
}

type OAuthServer interface {
	GetTokenForUser(ctx context.Context, id string) <-chan GetTokenForUserResult
	MakeClient(context.Context, *oauth2.Token) *http.Client
}

type oauthServer struct {
	log       *zap.Logger
	sc        OAuthServerConfig
	o2c       *oauth2.Config
	mu        sync.Mutex
	tokenReqs map[string]chan<- string
}

const tokenStoreVersion = 1

type (
	tokenMap map[string]*oauth2.Token

	tokenStore struct {
		Version int
		Tokens  tokenMap
	}
)

func readTokenStore(path string) (*tokenStore, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &tokenStore{Version: tokenStoreVersion, Tokens: make(tokenMap)}, nil
		}
		return nil, err
	}
	defer f.Close()
	var ts *tokenStore
	if err := json.NewDecoder(f).Decode(&ts); err != nil {
		return nil, err
	}
	if ts.Version != tokenStoreVersion {
		return nil, fmt.Errorf("Invalid tokenStore version, got %d, expected %d", ts.Version, tokenStoreVersion)
	}
	return ts, nil
}

func (ts *tokenStore) Save(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(ts)
}

func RunOAuthServer(ctx context.Context, sc OAuthServerConfig, o2c *oauth2.Config, log *zap.Logger) OAuthServer {
	o2c.RedirectURL = sc.RedirectURL
	s := &oauthServer{
		sc:        sc,
		o2c:       o2c,
		log:       log,
		tokenReqs: make(map[string]chan<- string),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleRequest)
	srv := &http.Server{
		Handler: mux,
		Addr:    sc.ListenAddr,
	}
	go func() {
		log.Info("Starting OAuth server", zap.String("addr", srv.Addr))
		err := srv.ListenAndServe()
		if err == http.ErrServerClosed {
			log.Info("Stopping OAuth server")
		} else {
			log.Error("ListenAndServe", zap.Error(err))
		}
	}()
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	return s
}

func (s *oauthServer) GetTokenForUser(ctx context.Context, userid string) <-chan GetTokenForUserResult {
	ch := make(chan GetTokenForUserResult)

	go func() {
		log := s.log.With(zap.String("userid", userid))

		s.mu.Lock()
		defer s.mu.Unlock()

		ts, err := readTokenStore(s.sc.TokenStore)
		if err != nil {
			ch <- GetTokenForUserResult{Error: err}
			return
		}
		token, ok := ts.Tokens[userid]
		if ok {
			ch <- GetTokenForUserResult{Token: token}
			return
		}

		// No token is stored, so put in a request.
		nonce := fmt.Sprintf("rd%d", rand.Int64())
		codeCh := make(chan string)
		s.tokenReqs[nonce] = codeCh

		// `ApprovalForce` is needed in combination with `AccessTypeOffline` in order
		// to get a refresh token.
		url := s.o2c.AuthCodeURL(nonce, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
		log.Info("Requesting authorization", zap.String("nonce", nonce), zap.String("url", url))

		// Drop the lock until the code is received.
		s.mu.Unlock()
		code := <-codeCh
		log.Info("Received code, exchanging for token")
		token, err = s.o2c.Exchange(ctx, code)
		s.mu.Lock()

		if err != nil {
			ch <- GetTokenForUserResult{Error: err}
			return
		}

		ts, err = readTokenStore(s.sc.TokenStore)
		if err != nil {
			ch <- GetTokenForUserResult{Error: err}
			return
		}
		ts.Tokens[userid] = token
		if err := ts.Save(s.sc.TokenStore); err != nil {
			ch <- GetTokenForUserResult{Error: err}
			return
		}

		ch <- GetTokenForUserResult{Token: token}
	}()

	return ch
}

func (s *oauthServer) handleRequest(rw http.ResponseWriter, req *http.Request) {
	id := req.FormValue("state")
	s.mu.Lock()
	ch, ok := s.tokenReqs[id]
	if ok {
		delete(s.tokenReqs, id)
		defer close(ch)
	}
	s.mu.Unlock()

	log := s.log.With(zap.String("id", id))

	if !ok {
		log.Error("No channel for token", zap.String("id", id))
		http.Error(rw, "Invalid State", http.StatusBadRequest)
		return
	}
	if code := req.FormValue("code"); code != "" {
		fmt.Fprintln(rw, "<h1>Authorized!</h1>")
		log.Info("Received authorization code", zap.String("id", id))
		ch <- code
		return
	}
	log.Error("Invalid request - missing code", zap.String("id", id))
	http.Error(rw, "Invalid Code", http.StatusBadRequest)
}

func (s *oauthServer) MakeClient(ctx context.Context, token *oauth2.Token) *http.Client {
	return s.o2c.Client(ctx, token)
}
