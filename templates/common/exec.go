package common

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// Exec is a wrapper around exec.CommandContext that captures stdout and stderr
// as strings. The input args must have len>=1.
//
// If the command fails, the error message will include the contents of stdout
// and stderr. This saves boilerplate in the caller.
func Exec(ctx context.Context, args ...string) (stdout, stderr string, _ error) {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	err := cmd.Run()
	stdout, stderr = stdoutBuf.String(), stderrBuf.String()
	if err != nil {
		err = fmt.Errorf("exec of %v failed: %w\nstdout: %s\nstderr: %s", args, err, cmd.Stdout, cmd.Stderr)
	}
	return stdout, stderr, err
}
