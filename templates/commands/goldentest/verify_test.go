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

// Package goldentest implements golden test related subcommands.
package goldentest

import (
	"context"
	"testing"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
)

func TestVerifyCommand(t *testing.T) {
	t.Parallel()

	specYaml := `api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'

desc: 'A simple template'

steps:
  - desc: 'Include some files and directories'
    action: 'include'
    params:
      paths: ['.']
`
	testYaml := `api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'GoldenTest'`

	cases := []struct {
		name         string
		testName     string
		filesContent map[string]string
		wantErr      string
	}{
		{
			name: "simple_test_verify_succeeds",
			filesContent: map[string]string{
				"spec.yaml":                       specYaml,
				"a.txt":                           "file A content",
				"b.txt":                           "file B content",
				"testdata/golden/test/test.yaml":  testYaml,
				"testdata/golden/test/data/a.txt": "file A content",
				"testdata/golden/test/data/b.txt": "file B content",
			},
		},
		{
			name: "multiple_tests_verify_succeeds",
			filesContent: map[string]string{
				"spec.yaml":                        specYaml,
				"a.txt":                            "file A content",
				"testdata/golden/test1/test.yaml":  testYaml,
				"testdata/golden/test1/data/a.txt": "file A content",
				"testdata/golden/test2/test.yaml":  testYaml,
				"testdata/golden/test2/data/a.txt": "file A content",
			},
		},
		{
			name: "redundant_file",
			filesContent: map[string]string{
				"spec.yaml":                       specYaml,
				"a.txt":                           "file A content",
				"b.txt":                           "file A content",
				"testdata/golden/test/test.yaml":  testYaml,
				"testdata/golden/test/data/a.txt": "file A content",
			},
			wantErr: "b.txt] generated, however not recoreded in test data",
		},
		{
			name: "missing_file",
			filesContent: map[string]string{
				"spec.yaml":                       specYaml,
				"a.txt":                           "file A content",
				"testdata/golden/test/test.yaml":  testYaml,
				"testdata/golden/test/data/a.txt": "file A content",
				"testdata/golden/test/data/b.txt": "file B content",
			},
			wantErr: "b.txt] expected, however missing",
		},
		{
			name: "insert_file_content",
			filesContent: map[string]string{
				"spec.yaml":                       specYaml,
				"a.txt":                           "file A content",
				"testdata/golden/test/test.yaml":  testYaml,
				"testdata/golden/test/data/a.txt": "file A content\n",
			},
			wantErr: "a.txt] file content mismatch",
		},
		{
			name: "remove_file_content",
			filesContent: map[string]string{
				"spec.yaml":                       specYaml,
				"a.txt":                           "file A content",
				"testdata/golden/test/test.yaml":  testYaml,
				"testdata/golden/test/data/a.txt": "file A",
			},
			wantErr: "a.txt] file content mismatch",
		},
		{
			name: "one_of_the_tests_fails",
			filesContent: map[string]string{
				"spec.yaml":                        specYaml,
				"a.txt":                            "file A content",
				"testdata/golden/test1/test.yaml":  testYaml,
				"testdata/golden/test1/data/a.txt": "file A content",
				"testdata/golden/test2/test.yaml":  testYaml,
				"testdata/golden/test2/data/a.txt": "file A content\n",
			},
			wantErr: "golden test verification failure",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()

			common.WriteAllDefaultMode(t, tempDir, tc.filesContent)

			ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))
			args := []string{"--location", tempDir}
			if tc.testName != "" {
				args = append(args, tc.testName)
			}

			r := &VerifyCommand{}
			if err := r.Run(ctx, args); err != nil {
				if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
					t.Fatal(diff)
				}
			}
		})
	}
}
