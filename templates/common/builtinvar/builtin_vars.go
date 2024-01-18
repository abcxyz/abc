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

package builtinvar

import (
	"fmt"

	"github.com/abcxyz/abc/templates/model/spec/features"
	"github.com/abcxyz/pkg/sets"
)

// TODO comment
const (
	GitTag      = "_git_tag"
	GitSHA      = "_git_sha"
	GitShortSHA = "_git_short_sha"
	FlagDest    = "_flag_dest"
	FlagSource  = "_flag_source"
)

// TODO comment
var PrintOnly = []string{FlagDest, FlagSource}s

// TODO comment
func Names(f features.Features) []string {
	// These vars have always existed in every api_version
	out := []string{FlagDest, FlagSource}

	// v1beta3 added these new vars
	if !f.SkipGitVars {
		out = append(out, GitSHA, GitShortSHA, GitTag)
	}

	return out
}

// TODO comment
func Validate(f features.Features, attemptedNames []string) error {
	allowed := Names(f)
	unknown := sets.Subtract(attemptedNames, allowed)
	if len(unknown) > 0 {
		return fmt.Errorf("these var names are unknown and therefore invalid: %v; the set of valid builtin var names is %v",
			unknown, allowed)
	}
	return nil
}

