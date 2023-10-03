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
	"testing"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/model"
	goldentest "github.com/abcxyz/abc/templates/model/goldentest/v1alpha1"
	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestParseTestCases(t *testing.T) {
	t.Parallel()

	validYaml := `api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'GoldenTest'`
	invalidYaml := "bad yaml"
	validTestCase := &goldentest.Test{}

	cases := []struct {
		name         string
		testName     string
		filesContent map[string]string
		want         []*TestCase
		wantErr      string
	}{
		{
			name:     "specified_test_name_succeed",
			testName: "test_case_1",
			filesContent: map[string]string{
				"testdata/golden/test_case_1/test.yaml": validYaml,
			},
			want: []*TestCase{
				{
					TestName:   "test_case_1",
					TestConfig: validTestCase,
				},
			},
		},
		{
			name:     "all_tests_succeed",
			testName: "",
			filesContent: map[string]string{
				"testdata/golden/test_case_1/test.yaml": validYaml,
				"testdata/golden/test_case_2/test.yaml": validYaml,
			},
			want: []*TestCase{
				{
					TestName:   "test_case_1",
					TestConfig: validTestCase,
				},
				{
					TestName:   "test_case_2",
					TestConfig: validTestCase,
				},
			},
		},
		{
			name:     "golden_test_dir_not_exist",
			testName: "",
			filesContent: map[string]string{
				"myfile": invalidYaml,
			},
			want:    nil,
			wantErr: "error reading golden test directory",
		},
		{
			name:     "unexpected_file_in_golden_test_dir",
			testName: "",
			filesContent: map[string]string{
				"testdata/golden/hello.txt": invalidYaml,
			},
			want:    nil,
			wantErr: "unexpected file entry under golden test directory",
		},
		{
			name:     "test_does_not_have_config",
			testName: "",
			filesContent: map[string]string{
				"testdata/golden/test_case_1/hello.txt": invalidYaml,
			},
			want:    nil,
			wantErr: "error opening test config",
		},
		{
			name:     "test_bad_config",
			testName: "",
			filesContent: map[string]string{
				"testdata/golden/test_case_1/test.yaml": invalidYaml,
			},
			want:    nil,
			wantErr: "error reading golden test config file",
		},
		{
			name:     "specified_test_name_not_found",
			testName: "test_case_2",
			filesContent: map[string]string{
				"testdata/golden/test_case_1/test.yaml": validYaml,
			},
			want:    nil,
			wantErr: "error opening test config",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()

			if err := common.WriteAllDefaultMode(tempDir, tc.filesContent); err != nil {
				t.Fatal(err)
			}

			got, err := parseTestCases(tempDir, tc.testName)
			if err != nil {
				if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
					t.Fatal(diff)
				}
				return
			}

			opt := cmpopts.IgnoreTypes(&model.ConfigPos{}, model.ConfigPos{})
			if diff := cmp.Diff(got, tc.want, opt); diff != "" {
				t.Fatalf("Output test cases wasn't as expected (-got,+want): %s", diff)
			}
		})
	}
}

func TestClearTestDir(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		filesContent map[string]string
		expected     map[string]string
	}{
		{
			name: "outdated_test_artifacts_removed",
			filesContent: map[string]string{
				"test.yaml": "yaml",
				"a.txt":     "file A content",
				"b/b.txt":   "file B content",
			},
			expected: map[string]string{
				"test.yaml": "yaml",
			},
		},
		{
			name: "test_config_in_sub_dir_removed",
			filesContent: map[string]string{
				"test.yaml":       "yaml",
				"teset/test.yaml": "yaml",
			},
			expected: map[string]string{
				"test.yaml": "yaml",
			},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()

			if err := common.WriteAllDefaultMode(tempDir, tc.filesContent); err != nil {
				t.Fatal(err)
			}

			if err := clearTestDir(tempDir); err != nil {
				t.Fatal(err)
			}

			gotDestContents := common.LoadDirWithoutMode(t, tempDir)
			if diff := cmp.Diff(gotDestContents, tc.expected); diff != "" {
				t.Errorf("dest directory contents were not as expected (-got,+want): %s", diff)
			}
		})
	}
}

func TestRenderTestCase(t *testing.T) {
	t.Parallel()

	specYaml := `api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'

desc: 'A simple template'

inputs:
  - name: 'input_a'
    desc: 'input of A'
    default: 'default'

  - name: 'input_b'
    desc: 'input of B'
    default: 'default'

steps:
  - desc: 'Include some files and directories'
    action: 'include'
    params:
      paths: ['.']
`

	cases := []struct {
		name                  string
		testCase              *TestCase
		filesContent          map[string]string
		expectedGoldenContent map[string]string
		wantErr               string
	}{
		{
			name: "simple_test_succeeds",
			testCase: &TestCase{
				TestName:   "test",
				TestConfig: &goldentest.Test{},
			},
			filesContent: map[string]string{
				"spec.yaml":                      specYaml,
				"a.txt":                          "file A content",
				"b.txt":                          "file B content",
				"testdata/golden/test/test.yaml": "yaml",
			},
			expectedGoldenContent: map[string]string{
				"test.yaml": "yaml",
				"a.txt":     "file A content",
				"b.txt":     "file B content",
			},
		},
		{
			name: "test_with_inputs_succeeds",
			testCase: &TestCase{
				TestName: "test",
				TestConfig: &goldentest.Test{
					Inputs: []*goldentest.InputValue{
						{
							Name:  model.String{Val: "input_a"},
							Value: model.String{Val: "a"},
						},
						{
							Name:  model.String{Val: "input_b"},
							Value: model.String{Val: "b"},
						},
					},
				},
			},
			filesContent: map[string]string{
				"spec.yaml":                      specYaml,
				"a.txt":                          "file A content",
				"b.txt":                          "file B content",
				"testdata/golden/test/test.yaml": "yaml",
			},
			expectedGoldenContent: map[string]string{
				"test.yaml": "yaml",
				"a.txt":     "file A content",
				"b.txt":     "file B content",
			},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()

			if err := common.WriteAllDefaultMode(tempDir, tc.filesContent); err != nil {
				t.Fatal(err)
			}

			err := renderTestCase(tempDir, tempDir, tc.testCase)
			if err != nil {
				if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
					t.Fatal(diff)
				}
				return
			}

			gotDestContents := common.LoadDirWithoutMode(t, filepath.Join(tempDir, "testdata/golden/test"))
			if diff := cmp.Diff(gotDestContents, tc.expectedGoldenContent, common.CmpFileMode); diff != "" {
				t.Errorf("dest directory contents were not as expected (-got,+want): %s", diff)
			}
		})
	}
}
