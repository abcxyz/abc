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
	"runtime"
	"strings"
	"testing"

	"github.com/abcxyz/pkg/testutil"
)

func TestExec(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		args           []string
		windowsOnly    bool
		nonWindowsOnly bool
		wantStdout     string
		wantStderr     string
		wantErr        string
	}{
		{
			name:       "multi_args",
			args:       []string{"echo", "hello", "world"},
			wantStdout: "hello world",
		},
		{
			name:           "nonwindows_simple_stderr",
			args:           []string{"ls", "--nonexistent"},
			nonWindowsOnly: true,
			wantErr:        "exec of [ls --nonexistent] failed",
			wantStderr:     "ls: unrecognized option",
		},
		{
			name:        "windows_simple_stderr",
			args:        []string{"dir", "/nonexistent"},
			windowsOnly: true,
			wantErr:     "exec of [dir /nonexistent] failed",
			wantStderr:  `Parameter format not correct - "nonexistent"`,
		},
		{
			name:    "nonexistent_cmd",
			args:    []string{"nonexistent-cmd"},
			wantErr: `exec: "nonexistent-cmd": executable file not found in $PATH`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if tc.nonWindowsOnly && runtime.GOOS == "windows" {
				return
			}
			if tc.windowsOnly && runtime.GOOS != "windows" {
				return
			}

			ctx := context.Background()
			stdout, stderr, err := Exec(ctx, tc.args...)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}
			if len(tc.wantStdout) > 0 && !strings.Contains(stdout, tc.wantStdout) {
				t.Errorf("got stdout:\n%q\nbut wanted stdout to contain: %q", stdout, tc.wantStdout)
			}

			if len(tc.wantStderr) > 0 && !strings.Contains(stderr, tc.wantStderr) {
				t.Errorf("got stderr:\n%q\nbut wanted stderr to contain: %q", stdout, tc.wantStderr)
			}
		})
	}
}
