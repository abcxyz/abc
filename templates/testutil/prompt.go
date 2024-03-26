// Copyright 2024 The Authors (see AUTHORS file)
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

// Package testutil contains util functions to facilitate tests.
package testutil

import (
	"context"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abcxyz/pkg/cli"
)

// DialogTest is a helper for running tests against a CLI command that involve
// communicating over stdin and stdout. The expected conversation is defined as
// a sequence of DialogSteps.
//
// If the observed dialog doesn't match the expected dialog, then tb.Fatalf()
// will be called. If the the
func DialogTest(ctx context.Context, tb testing.TB, steps []DialogStep, cmd cli.Command, runArgs []string) error {
	tb.Helper()

	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()
	stderrReader, stderrWriter := io.Pipe()
	defer stdinWriter.Close()
	defer stdinReader.Close()
	defer stdoutWriter.Close()
	defer stdoutWriter.Close()
	defer stderrWriter.Close()

	cmd.SetStdin(stdinReader)
	cmd.SetStdout(stdoutWriter)
	cmd.SetStderr(stderrWriter)

	wg := new(sync.WaitGroup)
	// errCh := make(chan error, 1)
	var err error
	wg.Add(1)
	go func() {
		// defer close(errCh)
		defer wg.Done()
		err = cmd.Run(ctx, runArgs)
		// errCh <- cmd.Run(ctx, runArgs)
		// errCh <- cmd(stdinReader, stdoutWriter, stderrWriter)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, ds := range steps {
			ReadWithTimeout(tb, stdoutReader, ds.WaitForPrompt)
			WriteWithTimeout(tb, stdinWriter, ds.ThenRespond)
		}
	}()

	// Even though we don't care about the contents of stderr, we still have to
	// read from the pipe to prevent any writes to the pipe from blocking.
	go func() {
		buf := make([]byte, 1_000_000) // size is arbitrary
		_, _ = stderrReader.Read(buf)
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-time.After(time.Second):
		buf := make([]byte, 1_000_000) // size is arbitrary
		length := runtime.Stack(buf, true)
		tb.Fatalf("timed out waiting for background goroutine to finish. Here's a stack trace to show where things are blocked: %s", buf[:length])
	case <-done:
	}

	if err != nil {
		return err
	}

	return nil
}

func ReadAndExpect(tb testing.TB, r io.Reader, wantSubstr string) {
	buf := make([]byte, 1_000_000)
	tb.Log("to Read")
	n, err := r.Read(buf)
	if err != nil {
		tb.Fatal(err)
	}
	got := string(buf[:n])

	if !strings.Contains(got, wantSubstr) {
		tb.Fatalf("got a prompt %q, but wanted a prompt containing %q", got, wantSubstr)
	}
}

// TODO remove?
// ReadWithTimeout does a single read from the given reader. It calls Fatal if
// that read fails or the returned string doesn't contain wantSubStr. May leak a
// goroutine on timeout.
func ReadWithTimeout(tb testing.TB, r io.Reader, wantSubstr string) {
	tb.Helper()

	tb.Logf("readWith starting with %q", wantSubstr)

	var got string
	errCh := make(chan error)
	go func() {
		defer close(errCh)
		ReadAndExpect(tb, r, wantSubstr)
		// buf := make([]byte, 64*1_000)
		// tb.Log("to Read")
		// n, err := r.Read(buf)
		// if err != nil {
		// 	errCh <- err
		// 	return
		// }
		// got = string(buf[:n])
		tb.Logf("Read returned: %s", got)
	}()

	select {
	case err := <-errCh:
		if err != nil {
			tb.Fatal(err)
		}
	case <-time.After(time.Second):
		tb.Fatalf("timed out waiting to read %q", wantSubstr)
	}

	// if !strings.Contains(got, wantSubstr) {
	// 	tb.Fatalf("got a prompt %q, but wanted a prompt containing %q", got, wantSubstr)
	// }
}

// TODO remove?
// WriteWithTimeout does a single write to the given writer. It calls Fatal
// if that read doesn't contain wantSubStr. May leak a goroutine on timeout.
func WriteWithTimeout(tb testing.TB, w io.Writer, msg string) {
	tb.Helper()

	tb.Logf("WriteWithTimeout starting with %q", msg)

	errCh := make(chan error)
	go func() {
		defer close(errCh)
		tb.Log("to Write")
		_, err := w.Write([]byte(msg))
		tb.Log("from Write")
		errCh <- err
	}()

	select {
	case err := <-errCh:
		if err != nil {
			tb.Fatal(err)
		}
	case <-time.After(time.Second):
		tb.Fatalf("timed out waiting to write %q", msg)
	}
}

// DialogStep describe the prompt and respond msg.
type DialogStep struct {
	WaitForPrompt string
	ThenRespond   string // should end with newline
}
