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

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/abcxyz/pkg/logging"
	"github.com/google/go-cmp/cmp"
)

func TestRealMain(t *testing.T) {
	ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))
	ctx, done := context.WithCancel(ctx)
	defer done()

	errCh := make(chan error, 1)
	doneCh := make(chan struct{}, 1)
	go func() {
		defer close(doneCh)

		if err := realMain(ctx); err != nil {
			select {
			case errCh <- err:
			default:
			}
		}
	}()

	resp, err := http.Get(fmt.Sprintf("http://localhost:%s", defaultPort))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(b))

	want := "hello world"
	if !strings.Contains(string(b), want) {
		t.Errorf("unexpected response:\n%s", cmp.Diff(string(b), want))
	}

	// stop server
	done()

	// Read any errors first
	select {
	case err := <-errCh:
		t.Fatal(err)
	default:
	}

	// Wait for done
	select {
	case <-doneCh:
	case <-time.After(time.Second):
		t.Errorf("expected server to be stopped")
	}
}
