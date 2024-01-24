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

import "fmt"

// UnknownVarError is an error that will be returned when a template
// references a variable that's nonexistent.
type UnknownVarError struct {
	VarName       string
	AvailableVars []string
	Wrapped       error
}

func (n *UnknownVarError) Error() string {
	return fmt.Sprintf("the template referenced a nonexistent variable name %q; available variable names are %v",
		n.VarName, n.AvailableVars)
}

func (n *UnknownVarError) Unwrap() error {
	return n.Wrapped
}

func (n *UnknownVarError) Is(other error) bool {
	_, ok := other.(*UnknownVarError)
	return ok
}
