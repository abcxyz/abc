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
	spec "github.com/abcxyz/abc/templates/model/spec/v1beta6"
	abctestutil "github.com/abcxyz/abc/templates/testutil"
	mdl "github.com/abcxyz/abc/templates/testutil/model"
	"github.com/abcxyz/pkg/testutil"
)

func TestActionStringReplace(t *testing.T) {
	t.Parallel()

	// These test cases are fairly minimal because all the heavy lifting is done
	// in walkAndModify which has its own comprehensive test suite.
	cases := []struct {
		name         string
		paths        []string
		replacements []*spec.StringReplacement
		inputs       map[string]string

		initialContents map[string]string
		want            map[string]string
		wantErr         string

		readFileErr error // no need to test all errors here, see TestWalkAndModify
	}{
		{
			name:  "simple_success",
			paths: []string{"my_file.txt"},
			replacements: []*spec.StringReplacement{
				{
					ToReplace: mdl.S("foo"),
					With:      mdl.S("bar"),
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
			name:  "same_file_only_processed_once",
			paths: []string{"my_file.txt", ".", "my_file.txt"},
			replacements: []*spec.StringReplacement{
				{
					ToReplace: mdl.S("foo"),
					With:      mdl.S("foofoo"),
				},
			},
			initialContents: map[string]string{
				"my_file.txt": "abc foo def",
			},
			want: map[string]string{
				"my_file.txt": "abc foofoo def",
			},
		},
		{
			name:  "multiple_files_should_work",
			paths: []string{""},
			replacements: []*spec.StringReplacement{
				{
					ToReplace: mdl.S("foo"),
					With:      mdl.S("bar"),
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
			replacements: []*spec.StringReplacement{
				{
					ToReplace: mdl.S("foo"),
					With:      mdl.S("bar"),
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
			replacements: []*spec.StringReplacement{
				{
					ToReplace: mdl.S("foo"),
					With:      mdl.S("bar"),
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
			replacements: []*spec.StringReplacement{
				{
					ToReplace: mdl.S("sand{{.old_suffix}}"),
					With:      mdl.S("hot{{.new_suffix}}"),
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
			replacements: []*spec.StringReplacement{
				{
					ToReplace: mdl.S("foo"),
					With:      mdl.S("bar"),
				},
			},
			initialContents: map[string]string{
				"my_file.txt": "foo",
			},
			inputs: map[string]string{},
			want: map[string]string{
				"my_file.txt": "foo",
			},
			wantErr: `nonexistent variable name "myinput"`,
		},
		{
			name:  "templated_toreplace_missing_input_should_fail",
			paths: []string{""},
			replacements: []*spec.StringReplacement{
				{
					ToReplace: mdl.S("{{.myinput}}"),
					With:      mdl.S("bar"),
				},
			},
			initialContents: map[string]string{
				"my_file.txt": "foo",
			},
			inputs: map[string]string{},
			want: map[string]string{
				"my_file.txt": "foo",
			},
			wantErr: `nonexistent variable name "myinput"`,
		},
		{
			name:  "templated_with_missing_input_should_fail",
			paths: []string{""},
			replacements: []*spec.StringReplacement{
				{
					ToReplace: mdl.S("foo"),
					With:      mdl.S("{{.myinput}}"),
				},
			},
			initialContents: map[string]string{
				"my_file.txt": "foo",
			},
			inputs: map[string]string{},
			want: map[string]string{
				"my_file.txt": "foo",
			},
			wantErr: `nonexistent variable name "myinput"`,
		},
		{
			name:  "fs_errors_should_be_returned",
			paths: []string{"my_file.txt"},
			replacements: []*spec.StringReplacement{
				{
					ToReplace: mdl.S("foo"),
					With:      mdl.S("bar"),
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
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			scratchDir := t.TempDir()
			abctestutil.WriteAll(t, scratchDir, tc.initialContents)

			sr := &spec.StringReplace{
				Paths:        mdl.Strings(tc.paths...),
				Replacements: tc.replacements,
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
			err := actionStringReplace(t.Context(), sr, sp)
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
