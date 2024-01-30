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
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/renderer"
)

func TestRealMain(t *testing.T) {
	t.Parallel()
	ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))
	ctx, done := context.WithCancel(ctx)
	defer done()

	var realMainErr error
	finishedCh := make(chan struct{})
	go func() {
		defer close(finishedCh)
		realMainErr = realMain(ctx)
	}()

	time.Sleep(100 * time.Millisecond)                                // wait for server startup
	resp, err := http.Get(fmt.Sprintf("http://localhost:%s/", *port)) //nolint:noctx
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	want := "hello world"
	if !strings.Contains(string(b), want) {
		t.Errorf("unexpected response: (-got,+want)\n%s", cmp.Diff(string(b), want))
	}

	// stop server
	done()

	// Wait for done
	select {
	case <-finishedCh:
	case <-time.After(time.Second):
		t.Fatalf("expected server to be stopped")
	}

	if realMainErr != nil {
		t.Errorf("realMain(): %v", realMainErr)
	}
}

func TestHandleHello(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := renderer.NewTesting(ctx, t, nil)

	cases := []struct {
		name string
		want string
	}{
		{
			name: "success",
			want: "hello world",
		},
	}
	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(handleHello(h))
			t.Cleanup(func() { server.Close() })

			req, err := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}

			resp, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			b, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(b), tc.want) {
				t.Errorf("unexpected response: (-got,+want)\n%s", cmp.Diff(string(b), tc.want))
			}
		})
	}
}
