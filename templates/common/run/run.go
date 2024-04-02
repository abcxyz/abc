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

package run

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/exp/slices"
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
// If the command fails, the error message will include the contents of stdout
// and stderr. This saves boilerplate in the caller.
func Run(ctx context.Context, args ...string) (stdout, stderr string, _ error) {
	stdout, stderr, _, err := run(ctx, false, args...)
	return stdout, stderr, err
}

// RunAllowNonzero is like Run, except that if the command has a nonzero exit
// code, that doesn't cause an error to be returned.
func RunAllowNonzero(ctx context.Context, args ...string) (stdout, stderr string, exitCode int, _ error) {
	return run(ctx, true, args...)
}

// if "allowNonzeroExit" is false, then a nonzero exit code from the command
// will cause an error to be returned.
func run(ctx context.Context, allowNonZeroExit bool, args ...string) (stdout, stderr string, exitCode int, _ error) {
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

	if err != nil {
		// Don't return error if both (a) the caller indicated they're OK with a
		// nonzero exit code and (b) the error is of a type that means the only
		// problem was a nonzero exit code.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && allowNonZeroExit {
			err = nil
		} else {
			err = fmt.Errorf(`exec of %v failed: error was "%w", context error was "%w"\nstdout: %s\nstderr: %s`,
				args, err, ctx.Err(), cmd.Stdout, cmd.Stderr)
		}
	}
	return stdout, stderr, cmd.ProcessState.ExitCode(), err
}

// RunMany calls [Run] for each command in args. If any command returns error,
// then no further commands will be run, and that error will be returned. For
// any commands that were actually executed (not aborted by a previous error),
// their stdout and stderr will be returned. It's guaranteed that
// len(stdouts)==len(stderrs).
func RunMany(ctx context.Context, args ...[]string) (stdouts, stderrs []string, _ error) {
	for _, cmd := range args {
		stdout, stderr, err := Run(ctx, cmd...)
		stdouts = append(stdouts, stdout)
		stderrs = append(stderrs, stderr)
		if err != nil {
			return stdouts, stderrs, err
		}
	}
	return stdouts, stderrs, nil
}

// RunDiff Returns len > 0 if there's a diff. Returns unified diff format.
// Returns empty string if neither file exists (this differs from the normal
// behavior of `/bin/diff -N`, which returns an error if both files are absent).
// TODO doc
func RunDiff(ctx context.Context, color bool, file1, file1RelTo, file2, file2RelTo string) (string, error) {
	file1Label, err := filepath.Rel(file1RelTo, file1)
	if err != nil {
		return "", fmt.Errorf("failed getting relative path for diff: %w", err)
	}
	file2Label, err := filepath.Rel(file2RelTo, file2)
	if err != nil {
		return "", fmt.Errorf("failed getting relative path for diff: %w", err)
	}
	args := []string{
		"diff",
		"-u", // Produce unified diff format (similar to git)
		"-N", // Treat nonexistent file as empty

		// Act like git, and name the files as a/foo and b/foo in the patch.
		"--label", "a/" + file1Label, file1,
		"--label", "b/" + file2Label, file2,
	}

	if color {
		args = slices.Insert(args, 1, "--color=always")
	}

	stdout, stderr, exitCode, err := RunAllowNonzero(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("error exec'ing diff: %w", err)
	}
	// The man page for diff says it returns code 2 on error.
	if exitCode == 2 {
		// A quirk of the diff command: the -N flag means "treat nonexistent
		// files as empty", but it still fails if both inputs are absent. Our
		// workaround is to detect the case where both files are nonexistent and
		// return empty string.
		file1Exists, err := exists(file1)
		if err != nil {
			return "", err //nolint:wrapcheck
		}
		file2Exists, err := exists(file2)
		if err != nil {
			return "", err //nolint:wrapcheck
		}
		if !file1Exists && !file2Exists {
			return "", nil
		}
		return "", fmt.Errorf("error exec'ing diff: %s", stderr)
	}
	return stdout, nil
}

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
