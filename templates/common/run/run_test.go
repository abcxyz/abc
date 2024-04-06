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
	"strings"
	"testing"
	"time"

	"github.com/abcxyz/pkg/testutil"
)

func TestRun(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		args             []string
		timeout          time.Duration
		stdin            string
		allowNonzeroExit bool
		wantStdout       string
		wantStderr       string
		wantExitCode     int
		wantErr          string
	}{
		{
			name:       "multi_args",
			args:       []string{"echo", "hello", "world"},
			wantStdout: "hello world",
		},
		{
			name:         "simple_stderr",
			args:         []string{"ls", "--nonexistent"},
			wantErr:      "exec of [ls --nonexistent] failed",
			wantStderr:   "ls: unrecognized option",
			wantExitCode: 2,
		},
		{
			name:         "nonexistent_cmd",
			args:         []string{"nonexistent-cmd"},
			wantErr:      `exec: "nonexistent-cmd": executable file not found`,
			wantExitCode: -1,
		},
		{
			name:         "timeout",
			args:         []string{"sleep", "1"},
			timeout:      time.Millisecond,
			wantErr:      "context deadline exceeded",
			wantExitCode: -1,
		},
		{
			name:             "nonzero_exit",
			args:             []string{"false"},
			allowNonzeroExit: true,
			wantExitCode:     1,
		},
		{
			name:             "nonzero_exit",
			args:             []string{"false"},
			allowNonzeroExit: false,
			wantExitCode:     1,
			wantErr:          "exit status 1",
		},
		{
			name:       "stdin",
			args:       []string{"cat"},
			stdin:      "hello",
			wantStdout: "hello",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			if tc.timeout != 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tc.timeout)
				defer cancel()
			}
			var stdout, stderr bytes.Buffer
			opts := []*Option{
				WithStdout(&stdout),
				WithStderr(&stderr),
			}
			if tc.allowNonzeroExit {
				opts = append(opts, AllowNonzeroExit())
			}
			if tc.stdin != "" {
				opts = append(opts, WithStdinStr(tc.stdin))
			}
			exitCode, err := Run(ctx, opts, tc.args...)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}
			if exitCode != tc.wantExitCode {
				t.Errorf("got exit code %d, want %d", exitCode, tc.wantExitCode)
			}
			if len(tc.wantStdout) > 0 && !strings.Contains(stdout.String(), tc.wantStdout) {
				t.Errorf("got stdout:\n%q\nbut wanted stdout to contain: %q", stdout.String(), tc.wantStdout)
			}

			if len(tc.wantStderr) > 0 && !strings.Contains(stderr.String(), tc.wantStderr) {
				t.Errorf("got stderr:\n%q\nbut wanted stderr to contain: %q", stderr.String(), tc.wantStderr)
			}
		})
	}
}
