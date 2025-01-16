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

package model

import (
	"fmt"
	"testing"

	"github.com/abcxyz/pkg/testutil"
)

var (
	passValidator = &fakeValidator{}
	failValidator = &fakeValidator{err: fmt.Errorf("fake error for testing")}
)

func TestValidateEach(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      []*fakeValidator
		wantErr string
	}{
		{
			name: "one_valid",
			in:   []*fakeValidator{passValidator},
		},
		{
			name:    "one_invalid",
			in:      []*fakeValidator{failValidator},
			wantErr: "fake error for testing",
		},
		{
			name:    "nil_entry",
			in:      []*fakeValidator{nil},
			wantErr: "list element was unexpectedly nil",
		},
		{
			name:    "one_valid_one_invalid",
			in:      []*fakeValidator{passValidator, failValidator},
			wantErr: "fake error for testing",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := ValidateEach(tc.in)
			if diff := testutil.DiffErrString(got, tc.wantErr); diff != "" {
				t.Error(diff)
			}
		})
	}
}

type fakeValidator struct {
	err error
}

func (f *fakeValidator) Validate() error {
	return f.err
}
