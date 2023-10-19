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
)

// Run is a wrapper around exec.CommandContext and Run() that captures stdout
// and stderr as strings. The input args must have len>=1.
//
// This is intended to be used for commands that run non-interactively then
// exit.
//
// This doesn't execute a shell (unless of course args[0] is the name of a shell
// binary).
//
// If the command fails, the error message will include the contents of stdout
// and stderr. This saves boilerplate in the caller.
func Run(ctx context.Context, args ...string) (stdout, stderr string, _ error) {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // exec'ing the input args is fundamentally the whole point

	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	err := cmd.Run()
	stdout, stderr = stdoutBuf.String(), stderrBuf.String()
	if err != nil {
		err = fmt.Errorf(`exec of %v failed: error was "%w", context error was "%w"\nstdout: %s\nstderr: %s`, args, err, ctx.Err(), cmd.Stdout, cmd.Stderr)
	}
	return stdout, stderr, err
}
