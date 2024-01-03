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
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
)

func TestRecordCommand(t *testing.T) {
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
		name                  string
		testNames             []string
		filesContent          map[string]string
		expectedGoldenContent map[string]string
		wantErr               string
	}{
		{
			name: "simple_test_succeeds",
			filesContent: map[string]string{
				"spec.yaml":                      specYaml,
				"a.txt":                          "file A content",
				"b.txt":                          "file B content",
				"testdata/golden/test/test.yaml": testYaml,
			},
			expectedGoldenContent: map[string]string{
				"test/test.yaml":  testYaml,
				"test/data/a.txt": "file A content",
				"test/data/b.txt": "file B content",
			},
		},
		{
			name: "multiple_tests_succeeds",
			filesContent: map[string]string{
				"spec.yaml":                       specYaml,
				"a.txt":                           "file A content",
				"testdata/golden/test1/test.yaml": testYaml,
				"testdata/golden/test2/test.yaml": testYaml,
			},
			expectedGoldenContent: map[string]string{
				"test1/test.yaml":  testYaml,
				"test1/data/a.txt": "file A content",
				"test2/test.yaml":  testYaml,
				"test2/data/a.txt": "file A content",
			},
		},
		{
			name: "outdated_golden_file_removed",
			filesContent: map[string]string{
				"spec.yaml":                              specYaml,
				"a.txt":                                  "file A content",
				"testdata/golden/test/test.yaml":         testYaml,
				"testdata/golden/test/data/outdated.txt": "outdated file",
			},
			expectedGoldenContent: map[string]string{
				"test/test.yaml":  testYaml,
				"test/data/a.txt": "file A content",
			},
		},
		{
			name: "outdated_golden_file_overwritten",
			filesContent: map[string]string{
				"spec.yaml":                       specYaml,
				"a.txt":                           "new content",
				"testdata/golden/test/test.yaml":  testYaml,
				"testdata/golden/test/data/a.txt": "old content",
			},
			expectedGoldenContent: map[string]string{
				"test/test.yaml":  testYaml,
				"test/data/a.txt": "new content",
			},
		},
		{
			name: "non_golden_test_data_removed",
			filesContent: map[string]string{
				"spec.yaml":                      specYaml,
				"a.txt":                          "file A content",
				"testdata/golden/test/test.yaml": testYaml,
				"testdata/golden/test/data/unexpected_file.txt": "oh",
			},
			expectedGoldenContent: map[string]string{
				"test/test.yaml":  testYaml,
				"test/data/a.txt": "file A content",
			},
		},
		{
			name:      "test_name_specified",
			testNames: []string{"test1"},
			filesContent: map[string]string{
				"spec.yaml":                       specYaml,
				"a.txt":                           "file A content",
				"testdata/golden/test1/test.yaml": testYaml,
				"testdata/golden/test2/test.yaml": testYaml,
			},
			expectedGoldenContent: map[string]string{
				"test1/test.yaml":  testYaml,
				"test1/data/a.txt": "file A content",
				"test2/test.yaml":  testYaml,
			},
		},
		{
			name:      "multiple_test_names_specified",
			testNames: []string{"test1", "test2"},
			filesContent: map[string]string{
				"spec.yaml":                       specYaml,
				"a.txt":                           "file A content",
				"testdata/golden/test1/test.yaml": testYaml,
				"testdata/golden/test2/test.yaml": testYaml,
				"testdata/golden/test3/test.yaml": testYaml,
			},
			expectedGoldenContent: map[string]string{
				"test1/test.yaml":  testYaml,
				"test1/data/a.txt": "file A content",
				"test2/test.yaml":  testYaml,
				"test2/data/a.txt": "file A content",
				"test3/test.yaml":  testYaml,
			},
		},
		{
			name: "error_in_test_will_not_write_file",
			filesContent: map[string]string{
				"spec.yaml":                       specYaml,
				"a.txt":                           "file A content",
				"testdata/golden/test1/test.yaml": "broken yaml",
				"testdata/golden/test2/test.yaml": "broken yaml",
			},
			expectedGoldenContent: map[string]string{
				"test1/test.yaml": "broken yaml",
				"test2/test.yaml": "broken yaml",
			},
			wantErr: "failed to parse golden test",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()

			common.WriteAllDefaultMode(t, tempDir, tc.filesContent)

			ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))

			args := []string{}
			if len(tc.testNames) > 0 {
				args = append(args, "--test-name", strings.Join(tc.testNames, ","))
			}
			args = append(args, tempDir)

			r := &RecordCommand{}
			if err := r.Run(ctx, args); err != nil {
				if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
					t.Fatal(diff)
				}
			}

			gotDestContents := common.LoadDirWithoutMode(t, filepath.Join(tempDir, "testdata/golden"))
			if diff := cmp.Diff(gotDestContents, tc.expectedGoldenContent); diff != "" {
				t.Errorf("dest directory contents were not as expected (-got,+want): %s", diff)
			}
		})
	}
}
