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
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/model"
	spec "github.com/abcxyz/abc/templates/model/spec/v1beta6"
	abctestutil "github.com/abcxyz/abc/templates/testutil"
	mdl "github.com/abcxyz/abc/templates/testutil/model"
	"github.com/abcxyz/pkg/testutil"
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
			wantErr:         `no paths were matched by: [my_file.txt]`,
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
			wantErr:         `nonexistent variable name "bad_name"`,
		},
		{
			name:            "templated_with_missing_input_should_fail",
			paths:           []string{"my_file.txt"},
			with:            "{{.bad_name}}",
			initialContents: map[string]string{"my_file.txt": "foo"},
			inputs:          map[string]string{"not": "right"},
			want:            map[string]string{"my_file.txt": "foo"},
			wantErr:         `nonexistent variable name "bad_name"`,
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
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			scratchDir := t.TempDir()
			abctestutil.WriteAll(t, scratchDir, tc.initialContents)

			sr := &spec.Append{
				Paths: mdl.Strings(tc.paths...),
				With:  mdl.S(tc.with),
				SkipEnsureNewline: model.Bool{
					Pos: &model.ConfigPos{},
					Val: tc.skipEnsureNewline,
				},
			}
			sp := &stepParams{
				scope:      common.NewScope(tc.inputs, nil),
				scratchDir: scratchDir,
				rp: &Params{
					FS: &common.ErrorFS{
						FS:          &common.RealFS{},
						ReadFileErr: tc.readFileErr,
					},
				},
			}
			err := actionAppend(t.Context(), sr, sp)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			got := abctestutil.LoadDir(t, scratchDir)
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("scratch directory contents were not as expected (-got,+want): %v", diff)
			}
		})
	}
}
