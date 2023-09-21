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
	"testing"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/abc/templates/model/goldentest"
	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestParseTestCases(t *testing.T) {
	t.Parallel()

	validYaml := `api_version: 'cli.abcxyz.dev/v1alpha1'`
	invalidYaml := "bad yaml"
	validTestCase := &goldentest.Test{
		APIVersion: model.String{Val: "cli.abcxyz.dev/v1alpha1"},
	}

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
