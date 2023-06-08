package model

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestErrWithPos(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		pos     *ConfigPos
		fmtStr  string
		args    []any
		wantErr string // comparison is exact
	}{
		{
			name:    "happy_path",
			pos:     &ConfigPos{10, 11},
			fmtStr:  "Oh no! some number %d: %w",
			args:    []any{345, fmt.Errorf("wrapped error")},
			wantErr: "failed executing template spec file at line 10: Oh no! some number 345: wrapped error",
		},
		{
			name:    "nil_position",
			pos:     nil,
			fmtStr:  "foo(): %w",
			args:    []any{fmt.Errorf("wrapped error")},
			wantErr: "foo(): wrapped error",
		},
		{
			name:    "zero_position",
			pos:     &ConfigPos{},
			fmtStr:  "foo(): %w",
			args:    []any{fmt.Errorf("wrapped error")},
			wantErr: "foo(): wrapped error",
		},
		{
			name:    "no_args",
			pos:     &ConfigPos{10, 11},
			fmtStr:  "abc def",
			args:    nil,
			wantErr: "failed executing template spec file at line 10: abc def",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := ErrWithPos(tc.pos, tc.fmtStr, tc.args...)
			if diff := cmp.Diff(got.Error(), tc.wantErr); diff != "" {
				t.Error(diff)
			}
		})
	}
}
