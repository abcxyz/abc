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

// Package prompt allows testing of "prompting" that simulates asking the user
// for input using stdin and stdout.
package prompt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/abcxyz/abc/templates/common/input"
	"github.com/abcxyz/pkg/cli"
)

const (
	// Set this to a large value like 9999 if you're debugging a test and you
	// want it to try forever instead of timing out. Set it back to 1 when
	// you're done debugging.
	waitMultiplier = 1

	// Timeouts are arbitrary, basically just long enough to finish the test
	// even if we're running on an overloaded CICD VM.
	readTimeout    = time.Second * waitMultiplier
	writeTimeout   = time.Second * waitMultiplier
	overallTimeout = 5 * time.Second * waitMultiplier
)

// DialogTest is a helper for running tests against a CLI command that involve
// communicating over stdin and stdout. The expected conversation is defined as
// a sequence of DialogSteps.
//
// If the observed dialog doesn't match the expected dialog, or if the test
// times out, then tb.Fatalf() will be called. In either of these cases, a
// goroutine could be leaked, but we consider that OK, because this is just a
// test.
//
// cmd.Run() will be called with runArgs. If Run() returns an error, that error
// will be returned from this function. That is the only error that will ever be
// returned by this function.
//
// If *both* (1) cmd.Run() returns error and (2) the observed dialog doesn't
// match the expected dialog, then tb.Fatalf() will be called (so no error will
// be returned. This allows the dialog to be verified even for cases that return
// error.
func DialogTest(ctx context.Context, tb testing.TB, steps []DialogStep, cmd cli.Command, runArgs []string) error {
	tb.Helper()

	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()
	stderrReader, stderrWriter := io.Pipe()

	defer func() {
		stderrWriter.Close()
		stdoutWriter.Close()
		stdinWriter.Close()
		stderrReader.Close()
		stdoutReader.Close()
		stdinReader.Close()
	}()

	cmd.SetStdin(stdinReader)
	cmd.SetStdout(stdoutWriter)
	cmd.SetStderr(stderrWriter)

	wg := new(sync.WaitGroup)
	var err error
	wg.Add(1)
	go func() {
		defer wg.Done()
		err = cmd.Run(ctx, runArgs)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, ds := range steps {
			ReadWithTimeout(tb, stdoutReader, ds.WaitForPrompt)
			if ds.ThenRespond != "" {
				WriteWithTimeout(tb, stdinWriter, ds.ThenRespond)
			}
		}
	}()

	// Even though we don't care about the contents of stderr, we still have to
	// read from the pipe to prevent any writes to the pipe from blocking.
	go func() {
		buf := make([]byte, 1_000) // size is arbitrary
		for {
			if _, err := stderrReader.Read(buf); errors.Is(err, io.EOF) {
				return
			}
		}
	}()

	// Start a background goroutine to wait for the waitgroup and close a
	// channel, so we can wait for the waitgroup in a "select".
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-time.After(overallTimeout):
		buf := make([]byte, 1_000_000) // size is arbitrary
		length := runtime.Stack(buf, true)
		tb.Fatalf("timed out waiting for background goroutine to finish. Here's a stack trace to show where things are blocked: %s", buf[:length])
	case <-done:
	}

	if err != nil {
		return err //nolint:wrapcheck
	}

	return nil
}

type FakePrompter struct {
	steps     []DialogStep // Immutable.
	stepIndex int          // Mutable. Incremented after each back-and-forth
}

func (f *FakePrompter) Prompt(ctx context.Context, msg string, args ...any) (string, error) {
	// Note for maintainers: we call panic() rather than t.Fatal() in this
	// function because panic() will stop all goroutines while t.Fatal() won't.
	// If the other goroutines are left running, they will produce misleading
	// failure messages that will obscure the true cause of the problem.

	if f.stepIndex >= len(f.steps) {
		if len(f.steps) == 0 {
			panic("there was an unexpected prompt for user input")
		}
		panic(fmt.Sprintf("there were %d prompts for user input, but expected only %d", f.stepIndex+1, f.stepIndex))
	}
	dialogStep := f.steps[f.stepIndex]

	gotPrompt := fmt.Sprintf(msg, args...)
	if gotPrompt != dialogStep.WaitForPrompt {
		diff := cmp.Diff(gotPrompt, dialogStep.WaitForPrompt)
		panic(fmt.Sprintf("Got prompt %q at step index %d but wanted %q. Diff was (-got, +want):\n%s", gotPrompt, f.stepIndex, dialogStep.WaitForPrompt, diff))
	}

	f.stepIndex++
	return dialogStep.ThenRespond, nil
}

func (f *FakePrompter) Stdin() io.Reader {
	panic("should never be called")
}

// This function doesn't do anything and is never called. It just satisfies an
// interface so a type assertion can check for a fake prompter.
func (f *FakePrompter) IsTestFake() {
	panic("this function is never called, it just satisfies an interface")
}

func DialogTestNoCmd(ctx context.Context, tb testing.TB, steps []DialogStep, f func(input.Prompter)) {
	tb.Helper()

	done := make(chan struct{})
	go func() {
		defer close(done)
		p := &FakePrompter{
			steps: steps,
		}
		f(p)
	}()

	select {
	case <-time.After(overallTimeout):
		buf := make([]byte, 1_000_000) // size is arbitrary
		length := runtime.Stack(buf, true)
		tb.Fatalf("timed out waiting for background goroutine to finish. Here's a stack trace to show where things are blocked: %s", buf[:length])
	case <-done:
	}
}

// ReadWithTimeout does a single read from the given reader. It calls Fatal if
// that read fails or the returned string doesn't contain wantSubStr. May leak a
// goroutine on timeout.
func ReadWithTimeout(tb testing.TB, r io.Reader, wantSubstr string) {
	tb.Helper()

	var got string
	errCh := make(chan error)
	go func() {
		defer close(errCh)
		buf := make([]byte, 64*1_000) // size is arbitrary
		tb.Logf("dialoger goroutine trying to read, expecting %q", wantSubstr)
		n, err := r.Read(buf)
		if err != nil {
			errCh <- err
			return
		}
		got = string(buf[:n])
		tb.Logf("dialoger goroutine read: %q", got)
	}()

	select {
	case err := <-errCh:
		if err != nil {
			tb.Fatal(err)
		}
	case <-time.After(readTimeout): // time is arbitrary
		tb.Fatalf("dialoger goroutine timed out waiting to read, expected to read %q", wantSubstr)
	}

	if !strings.Contains(got, wantSubstr) {
		tb.Fatalf("got a prompt %q, but wanted a prompt containing %q", got, wantSubstr)
	}
}

// WriteWithTimeout does a single write to the given writer. It calls Fatal
// if that read doesn't contain wantSubStr. May leak a goroutine on timeout.
func WriteWithTimeout(tb testing.TB, w io.Writer, msg string) {
	tb.Helper()

	errCh := make(chan error)
	go func() {
		defer close(errCh)
		tb.Logf("dialoger goroutine trying to write %q", msg)
		_, err := w.Write([]byte(msg))
		tb.Logf("dialoger goroutine wrote %q", msg)
		errCh <- err
	}()

	select {
	case err := <-errCh:
		if err != nil {
			tb.Fatal(err)
		}
	case <-time.After(writeTimeout): // time is arbitrary
		tb.Fatalf("dialoger goroutine timed out waiting to write %q", msg)
	}
}

// DialogStep describes the prompt and respond msg.
type DialogStep struct {
	WaitForPrompt string
	ThenRespond   string // should end with newline
}
