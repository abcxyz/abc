// Copyright 2024 The Authors (see AUTHORS file)
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

// Package errs contains errors that are shared across packages. It's named
// this way to avoid colliding with "errors" (stdlib), "error" (a builtin type),
// and "err" (a common variable name).
package errs

import (
	"errors"
	"testing"
)

func TestUnknownVar_ErrorsIsAs(t *testing.T) {
	t.Parallel()

	err := &UnknownVarError{
		VarName:       "my_var",
		AvailableVars: []string{"other_var"},
		Wrapped:       errors.New("wrapped"),
	}

	is := &UnknownVarError{}
	if !errors.Is(err, is) {
		t.Errorf("errors.Is() returned false, should return true when called with an error of type %T", is)
	}

	as := &UnknownVarError{}
	if !errors.As(err, &as) {
		t.Errorf("errors.As() returned false, should return true when called with an error of type %T", as)
	}
}
