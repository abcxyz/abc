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
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/abc/templates/model/goldentest"
	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestParseTestCases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		testName string
		fs       GoldenTestFS
		want     []*TestCase
		wantErr  string
	}{
		{
			name:     "specified_test_name_succeed",
			testName: "test_case_1",
			fs: fstest.MapFS{
				"t/testdata/golden/test_case_1/test.yaml": {
					Data: []byte(`api_version: 'cli.abcxyz.dev/v1alpha1'`),
				},
			},
			want: []*TestCase{
				{
					TestName: "test_case_1",
					TestConfig: &goldentest.Test{
						APIVersion: model.String{Val: "cli.abcxyz.dev/v1alpha1"},
					},
				},
			},
		},
		{
			name:     "all_tests_succeed",
			testName: "",
			fs: fstest.MapFS{
				"t/testdata/golden/test_case_1/test.yaml": {
					Data: []byte(`api_version: 'cli.abcxyz.dev/v1alpha1'`),
				},
				"t/testdata/golden/test_case_2/test.yaml": {
					Data: []byte(`api_version: 'cli.abcxyz.dev/v1alpha1'`),
				},
			},
			want: []*TestCase{
				{
					TestName: "test_case_1",
					TestConfig: &goldentest.Test{
						APIVersion: model.String{Val: "cli.abcxyz.dev/v1alpha1"},
					},
				},
				{
					TestName: "test_case_2",
					TestConfig: &goldentest.Test{
						APIVersion: model.String{Val: "cli.abcxyz.dev/v1alpha1"},
					},
				},
			},
		},
		{
			name:     "template_not_exist",
			testName: "",
			fs:       fstest.MapFS{},
			want:     nil,
			wantErr:  "error reading template directory (t): open t: file does not exist",
		},
		{
			name:     "golden_test_dir_not_exist",
			testName: "",
			fs: fstest.MapFS{
				"t": {Mode: fs.ModeDir},
			},
			want:    nil,
			wantErr: "error reading golden test directory (t/testdata/golden): open t/testdata/golden: file does not exist",
		},
		{
			name:     "unexpected_file_in_golden_test_dir",
			testName: "",
			fs: fstest.MapFS{
				"t/testdata/golden/hello.txt": {},
			},
			want:    nil,
			wantErr: "unexpeted file entry under golden test directory: hello.txt",
		},
		{
			name:     "test_does_not_have_config",
			testName: "",
			fs: fstest.MapFS{
				"t/testdata/golden/test_case_1": {Mode: fs.ModeDir},
			},
			want:    nil,
			wantErr: "error opening test config (t/testdata/golden/test_case_1/test.yaml): open t/testdata/golden/test_case_1/test.yaml: file does not",
		},
		{
			name:     "test_bad_config",
			testName: "",
			fs: fstest.MapFS{
				"t/testdata/golden/test_case_1/test.yaml": {
					Data: []byte("bad yaml"),
				},
			},
			want:    nil,
			wantErr: "error reading golden test config file: error parsing test YAML file: got yaml node of kind 8, expected 4",
		},
		{
			name:     "specified_test_name_not_found",
			testName: "test_case_2",
			fs: fstest.MapFS{
				"t/testdata/golden/test_case_1/test.yaml": {},
			},
			want:    nil,
			wantErr: "error opening test config (t/testdata/golden/test_case_2/test.yaml): open t/testdata/golden/test_case_2/test.yaml: file does not exist",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseTestCases("t", tc.testName, tc.fs)
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
