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

package templatesource

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/abcxyz/abc/templates/common/git"
)

var sha = regexp.MustCompile("^[0-9a-f]{40}$")

// gitCanonicalVersion examines a template directory and tries to determine the
// "best" template version by looking at .git. The "best" template version is
// defined as (in decreasing order of precedence):
//
//   - tags in decreasing order of semver (recent releases first)
//   - other non-semver tags in reverse alphabetical order
//   - the HEAD SHA
//
// It returns false if the given directory is not in a git workspace.
//
// It returns error only if something weird happened when running git commands.
// The returned string is always empty if the boolean is false.
func gitCanonicalVersion(ctx context.Context, dir string) (string, bool, error) {
	_, ok, err := git.Workspace(ctx, dir)
	if err != nil {
		return "", false, err //nolint:wrapcheck
	}
	if !ok {
		return "", false, nil
	}

	tag, ok, err := bestHeadTag(ctx, dir)
	if err != nil {
		return "", false, err
	}
	if ok {
		return tag, true, nil
	}

	sha, err := git.CurrentSHA(ctx, dir)
	if err != nil {
		return "", false, err //nolint:wrapcheck
	}
	return sha, true, nil
}

// bestHeadTag returns the tag that points to the current HEAD SHA. If there are
// multiple such tags, the precedence order is:
//   - tags in decreasing order of semver (recent releases first)
//   - other non-semver tags in reverse alphabetical order
//
// Returns false if there are no tags pointing to HEAD.
func bestHeadTag(ctx context.Context, dir string) (string, bool, error) {
	tags, err := git.HeadTags(ctx, dir)
	if err != nil {
		return "", false, err //nolint:wrapcheck
	}

	var nonSemverTags []string
	var semverTags []*semver.Version
	for _, tag := range tags {
		semverTag, err := parseSemverTag(tag)
		if err != nil {
			nonSemverTags = append(nonSemverTags, tag)
		} else {
			semverTags = append(semverTags, semverTag)
		}
	}

	if len(semverTags) > 0 {
		sort.Sort(sort.Reverse(semver.Collection(semverTags)))
		// The "v" was trimmed off during parsing. Add it back.
		return "v" + semverTags[0].Original(), true, nil
	}

	if len(nonSemverTags) > 0 {
		sort.Sort(sort.Reverse(sort.StringSlice(nonSemverTags)))
		return nonSemverTags[0], true, nil
	}

	return "", false, nil
}

// parseSemverTag parses a string of the form "v1.2.3" into a semver tag. In abc
// CLI, we require that tags begin with "v", and anything else is a parse error.
//
// WARNING: the returned semver.Version has had the "v" prefix stripped,
// so the string returned from .Original() will be missing the "v".
func parseSemverTag(t string) (*semver.Version, error) {
	if !strings.HasPrefix(t, "v") {
		return nil, fmt.Errorf("tag is not a valid semver tag because it didn't begin with 'v': %q", t)
	}
	sv, err := semver.StrictNewVersion(t[1:])
	if err != nil {
		return nil, fmt.Errorf("error parsing %q as a semver: %w", t, err)
	}
	return sv, nil
}
