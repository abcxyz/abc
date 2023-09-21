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
	"context"
	"regexp"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/abc/templates/model/spec"
	"golang.org/x/exp/maps"
)

// actionRegexNameLookup replaces named regex capturing groups with the template
// variable of the same name.
//
// For example, suppose we have these inputs:
//
//	template inputs: {person: Alice}
//	regex: (?P<person>__insert_here__)
//	file contents: "Hello, __insert_here__"
//
// Then the output would be "Hello, Alice".
func actionRegexNameLookup(ctx context.Context, rn *spec.RegexNameLookup, sp *stepParams) error {
	uncompiled := make([]model.String, len(rn.Replacements))
	for i, rp := range rn.Replacements {
		uncompiled[i] = rp.Regex
	}
	compiledRegexes, err := templateAndCompileRegexes(uncompiled, sp.scope)
	if err != nil {
		return err
	}

	paths, err := processPaths(rn.Paths, sp.scope)
	if err != nil {
		return err
	}

	if err := walkAndModify(ctx, sp.fs, sp.scratchDir, paths, func(b []byte) ([]byte, error) {
		for i, rn := range rn.Replacements {
			cr := compiledRegexes[i]
			allMatches := cr.FindAllSubmatchIndex(b, -1)

			var err error
			b, err = replaceWithNameLookup(allMatches, b, rn, cr, sp.scope)
			if err != nil {
				return nil, err
			}
		}
		return b, nil
	}); err != nil {
		return err
	}
	return nil
}

func replaceWithNameLookup(allMatches [][]int, b []byte, rn *spec.RegexNameLookupEntry, re *regexp.Regexp, scope *common.Scope) ([]byte, error) {
	for i := 1; i < len(re.SubexpNames()); i++ { // skip group 0, which is always unnamed because it's "the whole regex match"
		if re.SubexpNames()[i] == "" {
			return nil, rn.Regex.Pos.Errorf(`all capturing groups in a regex_name_lookup must be named, like (?P<myinputvar>myregex), not like (myregex)`)
		}
	}

	// Why iterate in reverse? We have to replace starting at the end of the
	// file working toward the beginning, so when we replace part of
	// the buffer it doesn't invalidate the indices of the other
	// matches indices.
	for allMatchesIdx := len(allMatches) - 1; allMatchesIdx >= 0; allMatchesIdx-- {
		oneMatch := allMatches[allMatchesIdx]
		// allMatches looks like [group0StartIdx, group0EndIdx, group1StartIdx, group1EndIdx, ... ].
		// That's why we have the "divide by two" stuff below; it's a
		// concatenated list of pairs.
		for subGroupIdx := len(oneMatch)/2 - 1; subGroupIdx > 0; subGroupIdx-- {
			subGroupName := re.SubexpNames()[subGroupIdx]
			replacementVal, ok := scope.Lookup(subGroupName)
			if !ok {
				return nil, rn.Regex.Pos.Errorf("there was no template input variable matching the subgroup name %q; available variables are %v",
					subGroupName, maps.Keys(scope.All()))
			}
			replaceAtStartIdx := oneMatch[subGroupIdx*2]
			replaceAtEndIdx := oneMatch[subGroupIdx*2+1]
			b = append(b[:replaceAtStartIdx],
				append([]byte(replacementVal), b[replaceAtEndIdx:]...)...)
		}
	}

	return b, nil
}
