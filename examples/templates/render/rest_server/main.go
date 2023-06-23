// Copyright 2023 The Authors (see AUTHORS file)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package main implements a simple HTTP/JSON REST example.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/abcxyz/pkg/renderer"
	"github.com/go-chi/chi/v5"
)

var bind = flag.String("bind", ":8080", "Specifies server ip address and port to listen on.")

func handleHello(h *renderer.Renderer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.RenderJSON(w, http.StatusOK, map[string]any{"message": "hello world"})
	})
}

// realMain creates an example backend HTTP server.
// This server supports graceful stopping and cancellation by:
//   - using a cancellable context
//   - listening to incoming requests in a goroutine
func realMain(ctx context.Context) error {
	// Make a new renderer for rendering json.
	// Don't provide filesystem as we don't have templates to render.
	h, err := renderer.New(ctx, nil,
		renderer.WithOnError(func(err error) {
			log.Printf("failed to render: %v", err)
		}))
	if err != nil {
		return fmt.Errorf("failed to create renderer for main server %w", err)
	}

	r := chi.NewRouter()
	r.Mount("/", handleHello(h))
	walkFunc := func(method, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		return nil
	}

	if err := chi.Walk(r, walkFunc); err != nil {
		return fmt.Errorf("error walking routes: %w", err)
	}

	s := &http.Server{
		Addr:              *bind,
		Handler:           r,
		ReadHeaderTimeout: 2 * time.Second,
	}

	log.Printf("starting server on %v", *bind)
	errCh := make(chan error, 1)
	go func() {
		if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case errCh <- err:
			default:
			}
		}
	}()

	// Wait for cancellation
	select {
	case err := <-errCh:
		return fmt.Errorf("error from server listener: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	if err := s.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("failed to shutdown: %w", err)
	}
	return nil
}

func main() {
	ctx, done := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer done()

	flag.Parse()
	if err := realMain(ctx); err != nil {
		done()
		log.Fatal(err)
	}
	log.Print("completed")
}
