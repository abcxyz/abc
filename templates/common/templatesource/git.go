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
	"sort"

	"github.com/Masterminds/semver/v3"

	"github.com/abcxyz/abc/templates/common/git"
	"github.com/abcxyz/pkg/logging"
)

const (
	LocTypeLocalGit  = "local_git"
	LocTypeRemoteGit = "remote_git"
)

// gitCanonicalVersion examines a template directory and tries to determine the
// "best" template version by looking at .git. The "best" template version is
// defined as (in decreasing order of precedence):
//
//   - tags in decreasing order of semver (recent releases first)
//   - other non-semver tags in reverse alphabetical order
//   - the HEAD SHA
//
// It returns false if:
//
//   - the given directory is not in a git workspace
//   - the git workspace is not clean (uncommitted changes) (for testing, you
//     can provide allowDirty=true to override this)
//
// It returns error only if something weird happened when running git commands.
// The returned string is always empty if the boolean is false.
func gitCanonicalVersion(ctx context.Context, dir string, allowDirty bool) (string, bool, error) {
	logger := logging.FromContext(ctx).With("logger", "CanonicalVersion")

	_, ok, err := git.Workspace(ctx, dir)
	if err != nil {
		return "", false, err //nolint:wrapcheck
	}
	if !ok {
		return "", false, nil
	}

	if !allowDirty {
		ok, err = git.IsClean(ctx, dir)
		if err != nil {
			return "", false, err //nolint:wrapcheck
		}
		if !ok {
			logger.WarnContext(ctx, "omitting template git version from manifest because the workspace is dirty",
				"source_git_workspace", dir)
			return "", false, nil
		}
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
		semverTag, err := git.ParseSemverTag(tag)
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
