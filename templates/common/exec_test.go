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
	"context"
	"strings"
	"testing"
	"time"

	"github.com/abcxyz/pkg/testutil"
)

func TestExec(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		args       []string
		timeout    time.Duration
		wantStdout string
		wantStderr string
		wantErr    string
	}{
		{
			name:       "multi_args",
			args:       []string{"echo", "hello", "world"},
			wantStdout: "hello world",
		},
		{
			name:       "simple_stderr",
			args:       []string{"ls", "--nonexistent"},
			wantErr:    "exec of [ls --nonexistent] failed",
			wantStderr: "ls: unrecognized option",
		},
		{
			name:    "nonexistent_cmd",
			args:    []string{"nonexistent-cmd"},
			wantErr: `exec: "nonexistent-cmd": executable file not found`,
		},
		{
			name:    "timeout",
			args:    []string{"sleep", "1"},
			timeout: time.Millisecond,
			wantErr: "context deadline exceeded",
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
			stdout, stderr, err := Run(ctx, tc.args...)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}
			if len(tc.wantStdout) > 0 && !strings.Contains(stdout, tc.wantStdout) {
				t.Errorf("got stdout:\n%q\nbut wanted stdout to contain: %q", stdout, tc.wantStdout)
			}

			if len(tc.wantStderr) > 0 && !strings.Contains(stderr, tc.wantStderr) {
				t.Errorf("got stderr:\n%q\nbut wanted stderr to contain: %q", stderr, tc.wantStderr)
			}
		})
	}
}
