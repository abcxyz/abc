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

package common

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// DefaultRunTimeout is how long we'll wait for commands to run in the case
// where the context doesn't already have a timeout. This was chosen
// arbitrarily.
const DefaultRunTimeout = time.Minute

// Run is a wrapper around exec.CommandContext and Run() that captures stdout
// and stderr as strings. The input args must have len>=1.
//
// This is intended to be used for commands that run non-interactively then
// exit.
//
// This doesn't execute a shell (unless of course args[0] is the name of a shell
// binary).
//
// If the incoming context doesn't already have a timeout, then a default
// timeout will be added (see DefaultRunTimeout).
//
// exitCode is -1 if the command could not be executed.
//
// If the command fails, the error message will include the contents of stdout
// and stderr. This saves boilerplate in the caller.
func Run(ctx context.Context, args ...string) (stdout, stderr string, exitCode int, _ error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultRunTimeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // exec'ing the input args is fundamentally the whole point

	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	err := cmd.Run()
	stdout, stderr = stdoutBuf.String(), stderrBuf.String()
	exitCode = cmd.ProcessState.ExitCode()
	if err != nil {
		err = fmt.Errorf(`exec of %v failed: error was "%w", context error was "%w", exitCode was %d\nstdout: %s\nstderr: %s`, args, err, ctx.Err(), exitCode, cmd.Stdout, cmd.Stderr)
	}
	return stdout, stderr, exitCode, err
}

// RunAllowNonZero is like Run(), except that if the command runs to completion
// with a nonzero exit code, that's not treated as an error. This is a
// workaround for the fact that some commands routinely use nonzero exit codes
// to communicate information, and we don't want to have to do complex error
// processing to handle this.
func RunAllowNonZero(ctx context.Context, args ...string) (stdout, stderr string, exitCode int, err error) {
	stdout, stderr, exitCode, err = Run(ctx, args...)
	// exitCode will be -1 for "real" errors, and will be >=0 for causes where
	// the command ran successfully but returned a nonzero exit code.
	if err != nil && exitCode > 0 {
		return stdout, stderr, exitCode, nil
	}
	return stdout, stderr, exitCode, err
}
