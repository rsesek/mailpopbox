// mailpopbox
// Copyright 2025 Blue Static <https://www.bluestatic.org>
// This program is free software licensed under the GNU General Public License,
// version 3.0. The full text of the license can be found in LICENSE.txt.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/http"
	"sync"

	"go.uber.org/zap"
	"golang.org/x/oauth2"
)

type OAuthServer struct {
	log       *zap.Logger
	c         *oauth2.Config
	mu        sync.Mutex
	tokenReqs map[string]chan<- string
}

func RunOAuthServer(ctx context.Context, srv *http.Server, config *oauth2.Config, log *zap.Logger) *OAuthServer {
	s := &OAuthServer{c: config,
		log:       log,
		tokenReqs: make(map[string]chan<- string),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleRequest)
	srv.Handler = mux
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

func (s *OAuthServer) AuthorizeToken() (string, <-chan string) {
	id := fmt.Sprintf("rd%d", rand.Int64())
	ch := make(chan string)

	s.mu.Lock()
	s.tokenReqs[id] = ch
	s.mu.Unlock()

	url := s.c.AuthCodeURL(id)
	s.log.Info("Requesting authorization", zap.String("id", id), zap.String("url", url))
	return url, ch
}

func (s *OAuthServer) handleRequest(rw http.ResponseWriter, req *http.Request) {
	id := req.FormValue("state")
	s.mu.Lock()
	ch, ok := s.tokenReqs[id]
	if ok {
		delete(s.tokenReqs, id)
	}
	s.mu.Unlock()
	defer close(ch)

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
