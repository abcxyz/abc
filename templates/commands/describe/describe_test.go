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

package describe

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
)

func TestDescribeFlags_Parse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		args    []string
		want    DescribeFlags
		wantErr string
	}{
		{
			name: "all_flags_present",
			args: []string{
				"--git-protocol", "https",
				"helloworld@v1",
			},
			want: DescribeFlags{
				Source:      "helloworld@v1",
				GitProtocol: "https",
			},
		},
		{
			name: "default_git_protocol_value",
			args: []string{
				"helloworld@v1",
			},
			want: DescribeFlags{
				Source:      "helloworld@v1",
				GitProtocol: "https",
			},
		},
		{
			name:    "required_source_is_missing",
			args:    []string{},
			wantErr: "missing <source> file",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var cmd Command
			cmd.SetLookupEnv(cli.MapLookuper(nil))

			err := cmd.Flags().Parse(tc.args)
			if err != nil || tc.wantErr != "" {
				if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
					t.Fatal(diff)
				}
				return
			}
			if diff := cmp.Diff(cmd.flags, tc.want); diff != "" {
				t.Errorf("got %#v, want %#v, diff (-got, +want): %v", cmd.flags, tc.want, diff)
			}
		})
	}
}

func TestRealRun(t *testing.T) {
	t.Parallel()
	specContents := `
api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'
desc: 'A template for the ages'
inputs:
  - name: 'name1'
    desc: 'desc1'
    default: '.'
    rules:
      - rule: 'test rule 0'
        message: 'test rule 0 message'
      - rule: 'test rule 1'
  - name: 'name2'
    desc: 'desc2'

steps:
- desc: 'Include some files and directories'
  action: 'include'
  params:
    paths:
      - paths: ['file1.txt', 'dir1', 'dir2/file2.txt']
`

	cases := []struct {
		name             string
		templateContents map[string]string
		wantAttrList     []string
		wantErr          string
	}{
		{
			name: "success",
			templateContents: map[string]string{
				"spec.yaml": specContents,
			},
			wantAttrList: []string{
				"Description", "A template for the ages",
				"Input name", "name1",
				"Description", "desc1",
				"Default", ".",
				"Rule 0", "test rule 0",
				"Rule 0 msg", "test rule 0 message",
				"Rule 1", "test rule 1",
				"Input name", "name2",
				"Description", "desc2",
			},
		},
		{
			name: "failed to read spec file",
			templateContents: map[string]string{
				"spec.yaml": "invalid yaml",
			},
			wantErr: "error reading template spec file",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()
			sourceDir := filepath.Join(tempDir, "source")
			common.WriteAllDefaultMode(t, sourceDir, tc.templateContents)
			rfs := &common.RealFS{}
			stdoutBuf := &strings.Builder{}
			r := &Command{
				flags: DescribeFlags{
					Source: sourceDir,
				},
			}

			rp := &runParams{
				stdout: stdoutBuf,
				fs: &common.ErrorFS{
					FS: rfs,
				},
			}

			ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))
			err := r.realRun(ctx, rp)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			if diff := cmp.Diff(tc.wantAttrList, r.attrList); diff != "" {
				t.Errorf(diff)
			}
		})
	}
}
