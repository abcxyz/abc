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

package render

import (
	"context"
	"fmt"
	"testing"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/abc/templates/model/spec"
	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
)

func TestActionAppend(t *testing.T) {
	t.Parallel()

	// These test cases are fairly minimal because all the heavy lifting is done
	// in walkAndModify which has its own comprehensive test suite.
	cases := []struct {
		name              string
		paths             []string
		with              string
		skipEnsureNewline bool
		inputs            map[string]string

		initialContents map[string]string
		want            map[string]string
		wantErr         string
		readFileErr     error // no need to test all errors here, see TestWalkAndModify
	}{
		{
			name:            "simple_success",
			paths:           []string{"my_file.txt"},
			with:            "foobar",
			initialContents: map[string]string{"my_file.txt": "abc foo def"},
			want:            map[string]string{"my_file.txt": "abc foo deffoobar\n"},
		},
		{
			name:  "multiple_files_should_work",
			paths: []string{"my_file.txt", "another_file.txt"},
			with:  "foobar",
			initialContents: map[string]string{
				"my_file.txt":      "abc foo def",
				"another_file.txt": "hi!",
				"don't_modify.txt": "don't change me!",
			},
			want: map[string]string{
				"my_file.txt":      "abc foo deffoobar\n",
				"another_file.txt": "hi!foobar\n",
				"don't_modify.txt": "don't change me!",
			},
		},
		{
			name:  "directory_path_should_work",
			paths: []string{"a/"},
			with:  "foobar",
			initialContents: map[string]string{
				"a/my_file.txt":        "abc foo def",
				"a/b/another_file.txt": "hi!",
				"don't_modify.txt":     "don't change me!",
			},
			want: map[string]string{
				"a/my_file.txt":        "abc foo deffoobar\n",
				"a/b/another_file.txt": "hi!foobar\n",
				"don't_modify.txt":     "don't change me!",
			},
		},
		{
			name:            "simple_success_newline",
			paths:           []string{"my_file.txt"},
			with:            "foobar",
			initialContents: map[string]string{"my_file.txt": "abc foo def\n"},
			want:            map[string]string{"my_file.txt": "abc foo def\nfoobar\n"},
		},
		{
			name:              "skip_ensure_newline_success_no_newline",
			paths:             []string{"my_file.txt"},
			with:              "foobar",
			skipEnsureNewline: true,
			initialContents:   map[string]string{"my_file.txt": "abc foo def"},
			want:              map[string]string{"my_file.txt": "abc foo deffoobar"},
		},
		{
			name:              "skip_ensure_newline_success_newline_provided",
			paths:             []string{"my_file.txt"},
			with:              "foobar\n",
			skipEnsureNewline: true,
			initialContents:   map[string]string{"my_file.txt": "abc foo def"},
			want:              map[string]string{"my_file.txt": "abc foo deffoobar\n"},
		},
		{
			name:            "empty_file_works",
			paths:           []string{"my_file.txt"},
			with:            "foo",
			initialContents: map[string]string{"my_file.txt": ""},
			want:            map[string]string{"my_file.txt": "foo\n"},
		},
		{
			name:            "missing_file_errors",
			paths:           []string{"my_file.txt"},
			with:            "foo",
			initialContents: map[string]string{},
			want:            map[string]string{},
			wantErr:         `glob "my_file.txt" did not match any files`,
		},
		{
			name:            "templated_name_and_text_should_succeed",
			paths:           []string{"my_{{.filename_adjective}}_file.txt"},
			with:            "{{.to_append}}",
			initialContents: map[string]string{"my_meow.wav_file.txt": "sandwich"},
			inputs: map[string]string{
				"filename_adjective": "meow.wav",
				"to_append":          "meowmoewmoewmoew\nmeow",
			},
			want: map[string]string{"my_meow.wav_file.txt": "sandwichmeowmoewmoewmoew\nmeow\n"},
		},
		{
			name:            "templated_filename_missing_input_should_fail",
			paths:           []string{"{{.bad_name}}"},
			initialContents: map[string]string{"uhoh.wmv": "foo"},
			inputs:          map[string]string{},
			want:            map[string]string{"uhoh.wmv": "foo"},
			wantErr:         `nonexistent input variable name "bad_name"`,
		},
		{
			name:            "templated_with_missing_input_should_fail",
			paths:           []string{"my_file.txt"},
			with:            "{{.bad_name}}",
			initialContents: map[string]string{"my_file.txt": "foo"},
			inputs:          map[string]string{"not": "right"},
			want:            map[string]string{"my_file.txt": "foo"},
			wantErr:         `nonexistent input variable name "bad_name"`,
		},
		{
			name:            "fs_errors_should_be_returned",
			paths:           []string{"my_file.txt"},
			with:            "foo",
			initialContents: map[string]string{"my_file.txt": "foo"},
			want:            map[string]string{"my_file.txt": "foo"},
			readFileErr:     fmt.Errorf("fake error for testing"),
			wantErr:         "fake error for testing",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Convert to OS-specific paths
			convertKeysToPlatformPaths(tc.want)

			scratchDir := t.TempDir()
			if err := common.WriteAllDefaultMode(scratchDir, tc.initialContents); err != nil {
				t.Fatal(err)
			}

			sr := &spec.Append{
				Paths: modelStrings(tc.paths),
				With: model.String{
					Pos: &model.ConfigPos{},
					Val: tc.with,
				},
				SkipEnsureNewline: model.Bool{
					Pos: &model.ConfigPos{},
					Val: tc.skipEnsureNewline,
				},
			}
			sp := &stepParams{
				fs: &errorFS{
					renderFS:    &realFS{},
					readFileErr: tc.readFileErr,
				},
				scratchDir: scratchDir,
				scope:      common.NewScope(tc.inputs),
			}
			err := actionAppend(context.Background(), sr, sp)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			got := loadDirWithoutMode(t, scratchDir)
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("scratch directory contents were not as expected (-got,+want): %v", diff)
			}
		})
	}
}
