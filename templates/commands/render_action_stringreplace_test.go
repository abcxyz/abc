package commands

import (
	"context"
	"fmt"
	"testing"

	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
)

func TestActionStringReplace(t *testing.T) {
	t.Parallel()

	// These test cases are fairly minimal because all the heavy lifting is done
	// in walkAndModify which has its own comprehensive test suite.
	cases := []struct {
		name      string
		paths     []string
		toReplace string
		with      string
		inputs    map[string]string

		initialContents map[string]string
		want            map[string]string
		wantErr         string

		readFileErr error // no need to test all errors here, see TestWalkAndModify
	}{
		{
			name:      "simple_success",
			paths:     []string{"my_file.txt"},
			toReplace: "foo",
			with:      "bar",
			initialContents: map[string]string{
				"my_file.txt": "abc foo def",
			},
			want: map[string]string{
				"my_file.txt": "abc bar def",
			},
		},
		{
			name:      "multiple_files_should_work",
			paths:     []string{""},
			toReplace: "foo",
			with:      "bar",
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
			name:      "no_replacement_needed_should_noop",
			paths:     []string{""},
			toReplace: "foo",
			with:      "bar",
			initialContents: map[string]string{
				"my_file.txt": "abc def",
			},
			want: map[string]string{
				"my_file.txt": "abc def",
			},
		},
		{
			name:      "empty_file_should_noop",
			paths:     []string{""},
			toReplace: "foo",
			with:      "bar",
			initialContents: map[string]string{
				"my_file.txt": "",
			},
			want: map[string]string{
				"my_file.txt": "",
			},
		},
		{
			name:      "templated_replacement_should_succeed",
			paths:     []string{"my_{{.filename_adjective}}_file.txt"},
			toReplace: "sand{{.old_suffix}}",
			with:      "hot{{.new_suffix}}",
			initialContents: map[string]string{
				"my_cool_file.txt":  "sandwich",
				"ignored_filed.txt": "ignored",
			},
			inputs: map[string]string{
				"filename_adjective": "cool",
				"old_suffix":         "wich",
				"new_suffix":         "dog",
			},
			want: map[string]string{
				"my_cool_file.txt":  "hotdog",
				"ignored_filed.txt": "ignored",
			},
		},
		{
			name:      "templated_filename_missing_input_should_fail",
			paths:     []string{"{{.myinput}}"},
			toReplace: "foo",
			with:      "bar",
			initialContents: map[string]string{
				"my_file.txt": "foo",
			},
			inputs: map[string]string{},
			want: map[string]string{
				"my_file.txt": "foo",
			},
			wantErr: `no entry for key "myinput"`,
		},
		{
			name:      "templated_toreplace_missing_input_should_fail",
			paths:     []string{""},
			toReplace: "{{.myinput}}",
			with:      "bar",
			initialContents: map[string]string{
				"my_file.txt": "foo",
			},
			inputs: map[string]string{},
			want: map[string]string{
				"my_file.txt": "foo",
			},
			wantErr: `no entry for key "myinput"`,
		},
		{
			name:      "templated_with_missing_input_should_fail",
			paths:     []string{""},
			toReplace: "foo",
			with:      "{{.myinput}}",
			initialContents: map[string]string{
				"my_file.txt": "foo",
			},
			inputs: map[string]string{},
			want: map[string]string{
				"my_file.txt": "foo",
			},
			wantErr: `no entry for key "myinput"`,
		},
		{
			name:      "fs_errors_should_be_returned",
			paths:     []string{"my_file.txt"},
			toReplace: "foo",
			with:      "bar",
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
			if err := writeAllDefaultMode(scratchDir, tc.initialContents); err != nil {
				t.Fatal(err)
			}

			sr := &model.StringReplace{
				Paths:     modelStrings(tc.paths),
				ToReplace: model.String{Val: tc.toReplace},
				With:      model.String{Val: tc.with},
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

			got := loadDirWithoutMode(t, scratchDir)
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("scratch directory contents were not as expected (-got,+want): %v", diff)
			}
		})
	}
}
