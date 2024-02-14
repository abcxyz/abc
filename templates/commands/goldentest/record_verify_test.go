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

// Package goldentest implements golden test related subcommands.
package goldentest

import (
	"context"
	"testing"

	abctestutil "github.com/abcxyz/abc/templates/testutil"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
)

// TestRecordVerify tests that the output of the record command can be verified by the verify
// command.
func TestRecordVerify(t *testing.T) {
	t.Parallel()

	specYAMLContents := `
api_version: 'cli.abcxyz.dev/v1beta5'
kind: 'Template'
desc: 'A simple template'
inputs:
  - name: 'path_to_include'
    desc: 'Path to include'
    default: '.'
steps:
  - desc: 'Include some files and directories'
    action: 'include'
    params:
      paths: ['{{.path_to_include}}']
`

	testYAMLContents := `
api_version: 'cli.abcxyz.dev/v1beta5'
kind: 'GoldenTest'
inputs:
- name: 'path_to_include'
  value: '.'
`

	templateContent := map[string]string{
		"a.txt":                          "file A content",
		"b.txt":                          "file B content",
		"spec.yaml":                      specYAMLContents,
		"testdata/golden/test/test.yaml": testYAMLContents,
	}

	cases := []struct {
		name      string
		testNames []string
		// messWith allows the testcase to break/mutate the recorded test output
		// directory, to see if verification will catch the problem.
		messWith      func(_ *testing.T, recordedDir string)
		wantVerifyErr string
	}{
		{
			name: "simple_test_succeeds",
		},
		{
			name: "mismatch_should_fail",
			messWith: func(t *testing.T, dir string) {
				t.Helper()
				abctestutil.WriteAllDefaultMode(t, dir, map[string]string{
					"a.txt": "mismatched content",
				})
			},
			wantVerifyErr: "file content mismatch",
		},
	}
	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()

			abctestutil.WriteAllDefaultMode(t, tempDir, templateContent)

			ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))

			args := []string{}
			if len(tc.testNames) > 0 {
				args = append(args, "--test-name", "test")
			}
			args = append(args, tempDir)

			r := &RecordCommand{}
			if err := r.Run(ctx, args); err != nil {
				t.Fatal(err)
			}

			if tc.messWith != nil {
				tc.messWith(t, tempDir)
			}

			v := &VerifyCommand{}
			if err := v.Run(ctx, args); err != nil {
				if diff := testutil.DiffErrString(err, tc.wantVerifyErr); diff != "" {
					t.Error(diff)
				}
			}
		})
	}
}
