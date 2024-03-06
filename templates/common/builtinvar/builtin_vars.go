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

// Package builtinvar deals with so-called "built in vars". These are vars that
// are prefixed with an underscore and are automatically provided by abc.
// Examples are _git_tag and _flag_dest.
package builtinvar

import (
	"fmt"

	"github.com/abcxyz/abc/templates/model/spec/features"
	"github.com/abcxyz/pkg/sets"
)

// These are the so-called "built-in" variable names that can be used in
// spec.yaml. Not all variables are always in scope; it depends on the
// api_version and the location within the spec file.
const (
	// The _git_* vars are in scope if and only if api_version>=v1beta3. They
	// may in-scope-but-empty-string if the template source is not a git repo.
	GitTag      = "_git_tag"
	GitSHA      = "_git_sha"
	GitShortSHA = "_git_short_sha"

	// Now is the Unix millisecond timestamp (as a string) of template execution
	// time (aka "today's datetime").
	Now = "_now"

	// The value of the --dest flag (the render output directory).
	FlagDest = "_flag_dest"

	// The positional argument on the command line providing the template to be
	// rendered.
	FlagSource = "_flag_source"
)

// Validate returns error if any of the attemptedNames are not valid builtin
// var names. The "features" parameter is derived from the api_version, and it's
// needed because the set of variable names that are in scope depends on the
// api_version; we sometimes add new variables.
func Validate(f features.Features, attemptedNames []string) error {
	allowed := NamesInScope(f)
	unknown := sets.Subtract(attemptedNames, allowed)
	if len(unknown) > 0 {
		return fmt.Errorf("these builtin override var names are unknown and therefore invalid: %v; the set of valid builtin var names is %v",
			unknown, allowed)
	}
	return nil
}

// NamesInScope returns the set of builtin var names.
func NamesInScope(f features.Features) []string {
	// These vars have always existed in every api_version
	out := []string{Now, FlagDest, FlagSource}

	// v1beta3 added these new vars
	if !f.SkipGitVars {
		out = append(out, GitSHA, GitShortSHA, GitTag)
	}

	return out
}
