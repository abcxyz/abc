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
	"fmt"
	"regexp"
	"strconv"

	"github.com/abcxyz/abc/templates/model"
)

// The regex_replace action replaces a regex match (or a subgroup thereof) with
// a given string. The replacement string can use subgroup references like $1 or
// ${groupname}, and can also use Go template expressions. An example is:
//
//   - regex: '([a-zA-Z]+), (?P<adjective>[a-zA-Z]+) world!'
//     subgroup: 1
//     with: 'fresh'
//
// This would transform a file containing "Hello, cool world!" to "Hello, fresh
// world!"
func actionRegexReplace(ctx context.Context, rr *model.RegexReplace, sp *stepParams) error {
	uncompiled := make([]model.String, len(rr.Replacements))
	for i, rp := range rr.Replacements {
		uncompiled[i] = rp.Regex
	}
	compiledRegexes, err := templateAndCompileRegexes(uncompiled, sp.inputs)
	if err != nil {
		return err
	}

	for i, rp := range rr.Replacements {
		subexps := compiledRegexes[i].NumSubexp()
		if max := maxSubgroup([]byte(rp.With.Val)); max > subexps {
			// Note to maintainers: subgroups are 1-indexed, and index i
			// corresponds to subgroup i because subgroup 0 is just the whole
			// regex match. Therefore we use ">" in the check above, and
			// NumSubexp() is the index of the final subgroup.
			return model.ErrWithPos(rp.With.Pos, "subgroup $%d is out of range; the largest subgroup in this regex is %d", max, subexps)
		}
	}

	for _, p := range rr.Paths {
		if err := walkAndModify(p.Pos, sp.fs, sp.scratchDir, p.Val, func(b []byte) ([]byte, error) {
			for i, rr := range rr.Replacements {
				cr := compiledRegexes[i]

				// Why reverse()? We have to replace starting at the end of the
				// file working toward the beginning, so when we replace part of
				// the buffer it doesn't invalidate the indices of the other
				// matches indices.
				allMatches := reverse(cr.FindAllSubmatchIndex(b, -1))

				var err error
				b, err = replaceWithTemplate(allMatches, b, rr, cr, sp.inputs)
				if err != nil {
					return nil, err
				}
			}
			return b, nil
		}); err != nil {
			return err
		}
	}

	return nil
}

func replaceWithTemplate(allMatches [][]int, b []byte, rr *model.RegexReplaceEntry, re *regexp.Regexp, inputs map[string]string) ([]byte, error) {
	for _, oneMatch := range allMatches {
		// Expand transforms "$1" and "${mygroup}" from "With" into the corresponding matched subgroups from oneMatch.
		replacementRegexExpanded := re.Expand(nil, []byte(rr.With.Val), b, oneMatch)
		// Why do regex expansion first before go-template execution? So we can
		// use regex subgroups to reference template variables to support people
		// trying to be super clever with their templates.
		replacementTemplateExpanded, err := parseAndExecuteGoTmpl(rr.With.Pos, string(replacementRegexExpanded), inputs)
		if err != nil {
			return nil, err
		}

		// Subgroup 0 means "the whole string matched by the regex, not just a
		// subgroup". This is a neat coincidence because the default Subgroup
		// number in the config is 0 if not specified by the user. So this gives
		// us the desired behavior of "if user doesn't specify a subgroup to
		// replace, then replace the whole matched string."
		replaceAtStartIdx := oneMatch[rr.Subgroup.Val*2] // bounds have already been checked in the caller
		replaceAtEndIdx := oneMatch[rr.Subgroup.Val*2+1]
		b = append(b[:replaceAtStartIdx],
			append([]byte(replacementTemplateExpanded),
				b[replaceAtEndIdx:]...)...)
	}
	return b, nil
}

// A regular expression that matches regex subgroup references like "$5" or
// "${5}" in a string that will be passed to Regexp.Expand().
var subGroupExtractRegex = regexp.MustCompile(`\$+` + // some number of dollar signs
	`[{]?` + // then optionally has a brace character
	`([0-9]+)`) // and then has some number of decimal digits (as a capturing group)

// Given a string that will be passed to Regexp.Expand(), try to find the
// highest-numbered subgroup reference. This lets us show a friendly error
// message instead of failing weirdly. If Regexp.Expand() sees a subgroup
// reference for a nonexistent subgroup, it will just expand it to the empty
// string. This is likely to be quite confusing for users; they'll get bad
// output without knowing why.
//
// This could be problematic if we find false positive or false negative
// matches, because the parser in Expand() might have a slightly different idea
// of what a subgroup reference looks like. We must accept this risk in exchange
// for the ability to detect problems and provide a decent error message to the
// user.
//
// Returns 0 when no subgroups were found. This makes sense because subgroup 0
// means the entire string matched by the regex, and therefore subgroup 0 will
// always be a valid "subgroup."
func maxSubgroup(in []byte) int {
	var maxSoFar int
	matches := subGroupExtractRegex.FindAllSubmatchIndex(in, -1)
	for _, oneMatch := range matches {
		// Count leading '$' bytes
		numDollars := 0
		for ; in[oneMatch[0]+numDollars] == '$'; numDollars++ {
		}

		if numDollars%2 == 0 {
			// This is is a literal dollar sign ("$$" expands to "$") and not a
			// subgroup reference. It could any even number of dollar signs,
			// like "$$$$$$" expands to "$$$" and is not a subgroup reference.
			continue
		}

		numberStartIdx, numberEndIdx := oneMatch[2*1], oneMatch[2*1+1]

		groupNum, err := strconv.Atoi(string(in[numberStartIdx:numberEndIdx]))
		if err != nil {
			// We're guaranteed that subgroup 1 is just decimal digits because
			// of the regex definition. An Atoi failure is not possible.
			panic(fmt.Errorf("impossible error: numeric subgroup couldn't be parsed as int: %w", err))
		}
		if groupNum > maxSoFar {
			maxSoFar = groupNum
		}
	}
	return maxSoFar
}
