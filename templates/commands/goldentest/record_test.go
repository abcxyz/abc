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
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	abctestutil "github.com/abcxyz/abc/templates/testutil"
	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
)

func TestRecordCommand(t *testing.T) {
	t.Parallel()

	specYaml := `api_version: 'cli.abcxyz.dev/v1beta5'
kind: 'Template'

desc: 'A simple template'

steps:
  - desc: 'Include some files and directories'
    action: 'include'
    params:
      paths: ['.']
`
	testYaml := `api_version: 'cli.abcxyz.dev/v1beta5'
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
				"test/test.yaml":          testYaml,
				"test/data/.abc/.gitkeep": "",
				"test/data/a.txt":         "file A content",
				"test/data/b.txt":         "file B content",
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
				"test1/test.yaml":          testYaml,
				"test1/data/.abc/.gitkeep": "",
				"test1/data/a.txt":         "file A content",
				"test2/test.yaml":          testYaml,
				"test2/data/.abc/.gitkeep": "",
				"test2/data/a.txt":         "file A content",
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
				"test/test.yaml":          testYaml,
				"test/data/.abc/.gitkeep": "",
				"test/data/a.txt":         "file A content",
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
				"test/test.yaml":          testYaml,
				"test/data/.abc/.gitkeep": "",
				"test/data/a.txt":         "new content",
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
				"test/test.yaml":          testYaml,
				"test/data/.abc/.gitkeep": "",
				"test/data/a.txt":         "file A content",
			},
		},
		{
			name: "failed_to_render_test_case",
			filesContent: map[string]string{
				"spec.yaml": `apiVersion: 'cli.abcxyz.dev/v1beta5'
kind: 'Template'


inputs:
  - name: 'my_input_without_default'
    desc: 'An input without a default'

steps:
  - desc: 'Print input values'
    action: 'print'
    params:
      message: |
        The variable values are:
          my_input_without_default={{.my_input_without_default}}`,
				"testdata/golden/test/test.yaml": testYaml,
			},
			expectedGoldenContent: map[string]string{
				"test/test.yaml": testYaml,
			},
			wantErr: "failed to render test case [test] for template location",
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
				"test1/test.yaml":          testYaml,
				"test1/data/.abc/.gitkeep": "",
				"test1/data/a.txt":         "file A content",
				"test2/test.yaml":          testYaml,
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
				"test1/test.yaml":          testYaml,
				"test1/data/.abc/.gitkeep": "",
				"test1/data/a.txt":         "file A content",
				"test2/test.yaml":          testYaml,
				"test2/data/.abc/.gitkeep": "",
				"test2/data/a.txt":         "file A content",
				"test3/test.yaml":          testYaml,
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
		{
			name: "test_with_stdout_succeeds",
			filesContent: map[string]string{
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta5'
kind: 'Template'

desc: 'A template that outputs no files and do print'
steps:
  - desc: 'Print a message'
    action: 'print'
    params:
        message: 'Hello'`,
				"testdata/golden/test/test.yaml": testYaml,
			},
			expectedGoldenContent: map[string]string{
				"test/test.yaml":          testYaml,
				"test/data/.abc/.gitkeep": "",
				"test/data/.abc/stdout":   "Hello\n",
			},
		},
		{
			name: "test_with_v1beta3_stdout_skip_succeeds",
			filesContent: map[string]string{
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta3'
kind: 'Template'

desc: 'A template that outputs no files and do print'
steps:
  - desc: 'Print a message'
    action: 'print'
    params:
        message: 'Hello'`,
				"testdata/golden/test/test.yaml": `api_version: 'cli.abcxyz.dev/v1beta3'
kind: 'GoldenTest'`,
			},
			expectedGoldenContent: map[string]string{
				"test/test.yaml": `api_version: 'cli.abcxyz.dev/v1beta3'
kind: 'GoldenTest'`,
				"test/data/.abc/.gitkeep": "",
			},
		},
		{
			name: "test_with_git_succeeds",
			filesContent: map[string]string{
				"spec.yaml":                      specYaml,
				"a.txt":                          "file A content",
				"b.txt":                          "file B content",
				"testdata/golden/test/test.yaml": testYaml,
				".gitignore":                     "gitignore contents",
				".gitfoo/file1.txt":              "file1",
			},
			expectedGoldenContent: map[string]string{
				"test/test.yaml":                          testYaml,
				"test/data/.abc/.gitkeep":                 "",
				"test/data/a.txt":                         "file A content",
				"test/data/b.txt":                         "file B content",
				"test/data/.gitignore.abc_renamed":        "gitignore contents",
				"test/data/.gitfoo.abc_renamed/file1.txt": "file1",
			},
		},
		{
			name: "test_with_v1beta3_git_succeeds",
			filesContent: map[string]string{
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta3'
kind: 'Template'

desc: 'A simple template'

steps:
  - desc: 'Include some files and directories'
    action: 'include'
    params:
      paths: ['.']
`,
				"a.txt": "file A content",
				"b.txt": "file B content",
				"testdata/golden/test/test.yaml": `api_version: 'cli.abcxyz.dev/v1beta3'
kind: 'GoldenTest'`,
				".gitignore":        "gitignore contents",
				".gitfoo/file1.txt": "file1",
			},
			expectedGoldenContent: map[string]string{
				"test/test.yaml": `api_version: 'cli.abcxyz.dev/v1beta3'
kind: 'GoldenTest'`,
				"test/data/.abc/.gitkeep":     "",
				"test/data/a.txt":             "file A content",
				"test/data/b.txt":             "file B content",
				"test/data/.gitignore":        "gitignore contents",
				"test/data/.gitfoo/file1.txt": "file1",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()

			abctestutil.WriteAll(t, tempDir, tc.filesContent)

			ctx := logging.WithLogger(t.Context(), logging.TestLogger(t))

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

			gotDestContents := abctestutil.LoadDir(t, filepath.Join(tempDir, "testdata/golden"))
			if diff := cmp.Diff(gotDestContents, tc.expectedGoldenContent); diff != "" {
				t.Errorf("dest directory contents were not as expected (-got,+want): %s", diff)
			}
		})
	}
}

func TestRecordAllCommand(t *testing.T) {
	t.Parallel()

	specYaml := `api_version: 'cli.abcxyz.dev/v1beta5'
kind: 'Template'

desc: 'A simple template'

steps:
  - desc: 'Include some files and directories'
    action: 'include'
    params:
      paths: ['.']
`
	testYaml := `api_version: 'cli.abcxyz.dev/v1beta5'
kind: 'GoldenTest'`

	cases := []struct {
		name                string
		testNames           []string
		filesContent        map[string]string
		expectedFileContent map[string]string
		wantErr             string
	}{
		{
			name: "simple_test_succeeds",
			filesContent: map[string]string{
				"template1/spec.yaml":                      specYaml,
				"template1/a.txt":                          "template 1 file A content",
				"template1/b.txt":                          "template 1 file B content",
				"template1/testdata/golden/test/test.yaml": testYaml,
				"template2/spec.yaml":                      specYaml,
				"template2/a.txt":                          "template 2 file A content",
				"template2/b.txt":                          "template 2 file B content",
				"template2/testdata/golden/test/test.yaml": testYaml,
			},
			expectedFileContent: map[string]string{
				"template1/spec.yaml":                               specYaml,
				"template1/a.txt":                                   "template 1 file A content",
				"template1/b.txt":                                   "template 1 file B content",
				"template1/testdata/golden/test/test.yaml":          testYaml,
				"template1/testdata/golden/test/data/.abc/.gitkeep": "",
				"template1/testdata/golden/test/data/a.txt":         "template 1 file A content",
				"template1/testdata/golden/test/data/b.txt":         "template 1 file B content",
				"template2/spec.yaml":                               specYaml,
				"template2/a.txt":                                   "template 2 file A content",
				"template2/b.txt":                                   "template 2 file B content",
				"template2/testdata/golden/test/test.yaml":          testYaml,
				"template2/testdata/golden/test/data/.abc/.gitkeep": "",
				"template2/testdata/golden/test/data/a.txt":         "template 2 file A content",
				"template2/testdata/golden/test/data/b.txt":         "template 2 file B content",
			},
		},
		{
			name: "test_name_specified",
			filesContent: map[string]string{
				"template1/spec.yaml":                       specYaml,
				"template1/a.txt":                           "template 1 file A content",
				"template1/b.txt":                           "template 1 file B content",
				"template1/testdata/golden/test1/test.yaml": testYaml,
				"template2/spec.yaml":                       specYaml,
				"template2/a.txt":                           "template 2 file A content",
				"template2/b.txt":                           "template 2 file B content",
				"template2/testdata/golden/test/test.yaml":  testYaml,
			},
			testNames: []string{"test1"},
			expectedFileContent: map[string]string{
				"template1/spec.yaml":                                specYaml,
				"template1/a.txt":                                    "template 1 file A content",
				"template1/b.txt":                                    "template 1 file B content",
				"template1/testdata/golden/test1/test.yaml":          testYaml,
				"template1/testdata/golden/test1/data/.abc/.gitkeep": "",
				"template1/testdata/golden/test1/data/a.txt":         "template 1 file A content",
				"template1/testdata/golden/test1/data/b.txt":         "template 1 file B content",
				"template2/spec.yaml":                                specYaml,
				"template2/a.txt":                                    "template 2 file A content",
				"template2/b.txt":                                    "template 2 file B content",
				"template2/testdata/golden/test/test.yaml":           testYaml,
			},
		},
		{
			name: "template1_record_err_template2_success",
			filesContent: map[string]string{
				"template1/spec.yaml":                      specYaml,
				"template1/a.txt":                          "template 1 file A content",
				"template1/b.txt":                          "template 1 file B content",
				"template1/testdata/golden/test/test.yaml": "broken yaml",
				"template2/spec.yaml":                      specYaml,
				"template2/a.txt":                          "template 2 file A content",
				"template2/b.txt":                          "template 2 file B content",
				"template2/testdata/golden/test/test.yaml": testYaml,
			},
			expectedFileContent: map[string]string{
				"template1/spec.yaml":                               specYaml,
				"template1/a.txt":                                   "template 1 file A content",
				"template1/b.txt":                                   "template 1 file B content",
				"template1/testdata/golden/test/test.yaml":          "broken yaml",
				"template2/spec.yaml":                               specYaml,
				"template2/a.txt":                                   "template 2 file A content",
				"template2/b.txt":                                   "template 2 file B content",
				"template2/testdata/golden/test/test.yaml":          testYaml,
				"template2/testdata/golden/test/data/.abc/.gitkeep": "",
				"template2/testdata/golden/test/data/a.txt":         "template 2 file A content",
				"template2/testdata/golden/test/data/b.txt":         "template 2 file B content",
			},
			wantErr: "failed to parse golden test",
		},
		{
			name: "ignore_golden_test_dir_spec_yaml",
			filesContent: map[string]string{
				"template1/spec.yaml":                      specYaml,
				"template1/a.txt":                          "template 1 file A content",
				"template1/b.txt":                          "template 1 file B content",
				"template1/testdata/golden/test/test.yaml": testYaml,
				"template1/testdata/golden/test/spec.yaml": specYaml,
			},
			expectedFileContent: map[string]string{
				"template1/spec.yaml":                               specYaml,
				"template1/a.txt":                                   "template 1 file A content",
				"template1/b.txt":                                   "template 1 file B content",
				"template1/testdata/golden/test/test.yaml":          testYaml,
				"template1/testdata/golden/test/spec.yaml":          specYaml,
				"template1/testdata/golden/test/data/.abc/.gitkeep": "",
				"template1/testdata/golden/test/data/a.txt":         "template 1 file A content",
				"template1/testdata/golden/test/data/b.txt":         "template 1 file B content",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()

			abctestutil.WriteAll(t, tempDir, tc.filesContent)

			ctx := logging.WithLogger(t.Context(), logging.TestLogger(t))

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

			gotDestContents := abctestutil.LoadDir(t, filepath.Join(tempDir))
			if diff := cmp.Diff(gotDestContents, tc.expectedFileContent); diff != "" {
				t.Errorf("dest directory contents were not as expected (-got,+want): %s", diff)
			}
		})
	}
}

func TestNewRecordFlags_Parse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		args    []string
		want    Flags
		wantErr string
	}{
		{
			name: "all_flags_present",
			args: []string{
				"--test-name=test1",
				"/a/b/c",
			},
			want: Flags{
				TestNames: []string{"test1"},
				Location:  "/a/b/c",
			},
		},
		{
			name: "default_location",
			args: []string{
				"--test-name=test1",
			},
			want: Flags{
				TestNames: []string{"test1"},
				Location:  ".",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var cmd RecordCommand
			cmd.SetLookupEnv(cli.MapLookuper(nil))

			err := cmd.Flags().Parse(tc.args)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}
			if diff := cmp.Diff(cmd.flags, tc.want); diff != "" {
				t.Errorf("got %#v, want %#v, diff (-got, +want): %v", cmd.flags, tc.want, diff)
			}
		})
	}
}
