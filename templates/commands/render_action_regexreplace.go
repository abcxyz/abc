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
	"regexp"

	"github.com/abcxyz/abc/templates/model"
)

// The regex_replace action replaces a regex match (or a subgroup thereof) with
// a given string. The replacement string can use subgroup references like
// ${groupname}, and can also use Go template expressions. An example is:
//
//   - regex: '([a-zA-Z]+), (?P<adjective>[a-zA-Z]+) world!'
//     subgroup_to_replace: 'adjective'
//     with: 'fresh'
//
// This would transform a file containing "Hello, cool world!" to "Hello, fresh
// world!".
func actionRegexReplace(ctx context.Context, rr *model.RegexReplace, sp *stepParams) error {
	uncompiled := make([]model.String, len(rr.Replacements))
	for i, rp := range rr.Replacements {
		uncompiled[i] = rp.Regex
	}
	compiledRegexes, err := templateAndCompileRegexes(uncompiled, sp.scope)
	if err != nil {
		return err
	}

	// For the sake of readable spec files, we require that all regex capturing
	// groups be named.
	for i, rp := range rr.Replacements {
		compiled := compiledRegexes[i]
		for subexpIdx, name := range compiled.SubexpNames() {
			if subexpIdx == 0 {
				// Subexp 0 is "the whole regex match", and not a subexp, so
				// it's never named. Therefore we skip the name check for subexp
				// 0.
				continue
			}
			if name == "" {
				return model.ErrWithPos(rp.Regex.Pos, "all capturing groups in regexes must be named, like (?P<myname>re) . The %d'th capturing group in regex %s is an unnamed group, like (re) . Please use either a named capturing group or an non-capturing group like (?:re)", subexpIdx, rp.Regex.Val) //nolint:wrapcheck
			}
		}
	}

	// For the sake of readable spec files, we require that all regex expansions reference
	// the subgroup by name (like ${mygroup}) rather than number (like ${1}).
	for _, rp := range rr.Replacements {
		if err := rejectNumberedSubgroupExpand(rp.With); err != nil {
			return err
		}
	}

	paths := make([]model.String, 0, len(rr.Paths))
	for _, p := range rr.Paths {
		path, err := parseAndExecuteGoTmpl(p.Pos, p.Val, sp.scope)
		if err != nil {
			return err
		}
		paths = append(paths, model.String{Pos: p.Pos, Val: path})
	}

	if err := walkAndModify(ctx, sp.fs, sp.scratchDir, paths, func(b []byte) ([]byte, error) {
		for i, rr := range rr.Replacements {
			cr := compiledRegexes[i]
			allMatches := cr.FindAllSubmatchIndex(b, -1)

			var err error
			b, err = replaceWithTemplate(allMatches, b, rr, cr, sp.scope)
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

func replaceWithTemplate(allMatches [][]int, b []byte, rr *model.RegexReplaceEntry, re *regexp.Regexp, scope *scope) ([]byte, error) {
	// Why iterate in reverse? We have to replace starting at the end of the
	// file working toward the beginning, so when we replace part of
	// the buffer it doesn't invalidate the indices of the other
	// matches indices.
	for allMatchesIdx := len(allMatches) - 1; allMatchesIdx >= 0; allMatchesIdx-- {
		oneMatch := allMatches[allMatchesIdx]
		// Expand transforms "${mygroup}" from "With" into the
		// corresponding matched subgroups from oneMatch.
		replacementRegexExpanded := re.Expand(nil, []byte(rr.With.Val), b, oneMatch)
		// Why do regex expansion first before go-template execution? So we can
		// use regex subgroups to reference template variables to support people
		// trying to be super clever with their templates. Like:
		// {{.${mysubgroup}}}
		replacementTemplateExpanded, err := parseAndExecuteGoTmpl(rr.With.Pos, string(replacementRegexExpanded), scope)
		if err != nil {
			return nil, err
		}

		subgroupNum := 0
		// If the user didn't specify a subgroup to replace, then replace
		// subgroup 0, which is the entire string matched by the regex.
		if rr.SubgroupToReplace.Val != "" {
			subgroupNum = re.SubexpIndex(rr.SubgroupToReplace.Val)
			if subgroupNum < 0 {
				return nil, model.ErrWithPos(rr.SubgroupToReplace.Pos, "subgroup name %q is not a named subgroup in the regex %s", rr.SubgroupToReplace.Val, re.String()) //nolint:wrapcheck
			}
		}
		replaceAtStartIdx := oneMatch[subgroupNum*2] // bounds have already been checked in the caller
		replaceAtEndIdx := oneMatch[subgroupNum*2+1]
		b = append(b[:replaceAtStartIdx],
			append([]byte(replacementTemplateExpanded),
				b[replaceAtEndIdx:]...)...)
	}
	return b, nil
}

// A regular expression that matches regex subgroup references like "$5" or
// "${5}" in a string that will be passed to Regexp.Expand().
var subGroupExtractRegex = regexp.MustCompile(`(?P<dollars>\$+)` + // some number of dollar signs
	`[{]?` + // then optionally has a brace character
	`[0-9]+`) // and then has some number of decimal digits (as a capturing group)

// Given a string that will be passed to Regexp.Expand(), make sure it that it
// doesn't use any numbered subgroup expansions (like ${1}). Named subgroup
// expansions are allowed (like ${mygroup}). This is a policy decision because
// we consider the numbered form to be harder to read.
func rejectNumberedSubgroupExpand(with model.String) error {
	matches := subGroupExtractRegex.FindAllStringSubmatch(with.Val, -1)
	for _, oneMatch := range matches {
		dollars := oneMatch[subGroupExtractRegex.SubexpIndex("dollars")]
		if len(dollars)%2 == 0 {
			// This is a literal dollar sign ("$$" expands to "$") and not a
			// subgroup reference. It could any even number of dollar signs,
			// like "$$$$$$" expands to "$$$" and is not a subgroup reference.
			continue
		}

		return model.ErrWithPos(with.Pos, "regex expansions must reference the subgroup by name, like ${mygroup}, rather than by number, like ${1}; we saw %s", oneMatch[0]) //nolint:wrapcheck
	}
	return nil
}
