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

package commands

import (
	"context"
	"fmt"
	"testing"

	"github.com/abcxyz/abc/templates/model"
	abctestutil "github.com/abcxyz/abc/testutil"
	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
)

func TestActionStringReplace(t *testing.T) {
	t.Parallel()

	// These test cases are fairly minimal because all the heavy lifting is done
	// in walkAndModify which has its own comprehensive test suite.
	cases := []struct {
		name         string
		paths        []string
		replacements []*model.StringReplacement
		inputs       map[string]string

		initialContents map[string]string
		want            map[string]string
		wantErr         string

		readFileErr error // no need to test all errors here, see TestWalkAndModify
	}{
		{
			name:  "simple_success",
			paths: []string{"my_file.txt"},
			replacements: []*model.StringReplacement{
				{
					ToReplace: model.String{Val: "foo"},
					With:      model.String{Val: "bar"},
				},
			},
			initialContents: map[string]string{
				"my_file.txt": "abc foo def",
			},
			want: map[string]string{
				"my_file.txt": "abc bar def",
			},
		},
		{
			name:  "multiple_files_should_work",
			paths: []string{""},
			replacements: []*model.StringReplacement{
				{
					ToReplace: model.String{Val: "foo"},
					With:      model.String{Val: "bar"},
				},
			},
			initialContents: map[string]string{
				"my_file.txt":       "abc foo def",
				"my_other_file.txt": "abc foo def",
			},
			want: map[string]string{
				"my_file.txt":       "abc bar def",
				"my_other_file.txt": "abc bar def",
			},
		},
		{
			name:  "no_replacement_needed_should_noop",
			paths: []string{""},
			replacements: []*model.StringReplacement{
				{
					ToReplace: model.String{Val: "foo"},
					With:      model.String{Val: "bar"},
				},
			},
			initialContents: map[string]string{
				"my_file.txt": "abc def",
			},
			want: map[string]string{
				"my_file.txt": "abc def",
			},
		},
		{
			name:  "empty_file_should_noop",
			paths: []string{""},
			replacements: []*model.StringReplacement{
				{
					ToReplace: model.String{Val: "foo"},
					With:      model.String{Val: "bar"},
				},
			},
			initialContents: map[string]string{
				"my_file.txt": "",
			},
			want: map[string]string{
				"my_file.txt": "",
			},
		},
		{
			name:  "templated_replacement_should_succeed",
			paths: []string{"my_{{.filename_adjective}}_file.txt"},
			replacements: []*model.StringReplacement{
				{
					ToReplace: model.String{Val: "sand{{.old_suffix}}"},
					With:      model.String{Val: "hot{{.new_suffix}}"},
				},
			},
			initialContents: map[string]string{
				"my_cool_file.txt":  "sandwich",
				"ignored_filed.txt": "ignored",
			},
			inputs: map[string]string{
				"filename_adjective": "cool",
				"old_suffix":         "wich", //nolint:misspell
				"new_suffix":         "dog",
			},
			want: map[string]string{
				"my_cool_file.txt":  "hotdog",
				"ignored_filed.txt": "ignored",
			},
		},
		{
			name:  "templated_filename_missing_input_should_fail",
			paths: []string{"{{.myinput}}"},
			replacements: []*model.StringReplacement{
				{
					ToReplace: model.String{Val: "foo"},
					With:      model.String{Val: "bar"},
				},
			},
			initialContents: map[string]string{
				"my_file.txt": "foo",
			},
			inputs: map[string]string{},
			want: map[string]string{
				"my_file.txt": "foo",
			},
			wantErr: `nonexistent input variable name "myinput"`,
		},
		{
			name:  "templated_toreplace_missing_input_should_fail",
			paths: []string{""},
			replacements: []*model.StringReplacement{
				{
					ToReplace: model.String{Val: "{{.myinput}}"},
					With:      model.String{Val: "bar"},
				},
			},
			initialContents: map[string]string{
				"my_file.txt": "foo",
			},
			inputs: map[string]string{},
			want: map[string]string{
				"my_file.txt": "foo",
			},
			wantErr: `nonexistent input variable name "myinput"`,
		},
		{
			name:  "templated_with_missing_input_should_fail",
			paths: []string{""},
			replacements: []*model.StringReplacement{
				{
					ToReplace: model.String{Val: "foo"},
					With:      model.String{Val: "{{.myinput}}"},
				},
			},
			initialContents: map[string]string{
				"my_file.txt": "foo",
			},
			inputs: map[string]string{},
			want: map[string]string{
				"my_file.txt": "foo",
			},
			wantErr: `nonexistent input variable name "myinput"`,
		},
		{
			name:  "fs_errors_should_be_returned",
			paths: []string{"my_file.txt"},
			replacements: []*model.StringReplacement{
				{
					ToReplace: model.String{Val: "foo"},
					With:      model.String{Val: "bar"},
				},
			},
			initialContents: map[string]string{
				"my_file.txt": "abc foo def",
			},
			want: map[string]string{
				"my_file.txt": "abc foo def",
			},
			readFileErr: fmt.Errorf("fake error for testing"),
			wantErr:     "fake error for testing",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			scratchDir := t.TempDir()
			abctestutil.WriteAllDefaultMode(t, scratchDir, tc.initialContents)

			sr := &model.StringReplace{
				Paths:        modelStrings(tc.paths),
				Replacements: tc.replacements,
			}
			sp := &stepParams{
				fs: &errorFS{
					renderFS:    &realFS{},
					readFileErr: tc.readFileErr,
				},
				scratchDir: scratchDir,
				inputs:     tc.inputs,
			}
			err := actionStringReplace(context.Background(), sr, sp)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			got := abctestutil.LoadDirWithoutMode(t, scratchDir)
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("scratch directory contents were not as expected (-got,+want): %v", diff)
			}
		})
	}
}
