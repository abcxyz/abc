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

package templatesource

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/abcxyz/abc/templates/common/git"
	"github.com/abcxyz/abc/templates/common/specutil"
	"github.com/abcxyz/pkg/logging"
)

const (
	LocTypeLocalGit  = "local_git"
	LocTypeRemoteGit = "remote_git"
)

// sourceParser is implemented for each particular kind of template source (git,
// local file, etc.).
type sourceParser interface {
	// sourceParse attempts to parse the given src. If the src is recognized as
	// being downloadable by this sourceParser, then it returns true, along with
	// a downloader that can download that template, and other metadata. See
	// ParsedSource.
	sourceParse(ctx context.Context, cwd string, params *ParseSourceParams) (Downloader, bool, error)
}

// realSourceParsers contains the non-test sourceParsers.
var realSourceParsers = []sourceParser{
	// This source parser recognizes template sources like
	// "github.com/myorg/myrepo@v1.2.3" (and variants thereof).
	&gitSourceParser{
		re: regexp.MustCompile(
			`^` + // Anchor the start, must match the entire input
				`(?P<host>github\.com|gitlab\.com)` + // The domain names of known git hosting services
				`/` +
				`(?P<org>[a-zA-Z0-9_-]+)` + // the github org name, e.g. "abcxyz"
				`/` +
				`(?P<repo>[a-zA-Z0-9_-]+)` + // the github repo name, e.g. "abc"
				`(/(?P<subdir>[^@]*))?` + // Optional subdir with leading slash; the leading slash is not part of capturing group ${subdir}
				`@(?P<version>[a-zA-Z0-9_/.-]+)` + // The "@latest" or "@v1.2.3" or "@v1.2.3-foo" at the end; the "@" is not part of the capturing group
				`$`), // Anchor the end, must match the entire input
		httpsRemoteExpansion: `https://${host}/${org}/${repo}.git`,
		sshRemoteExpansion:   `git@${host}:${org}/${repo}.git`,
		subdirExpansion:      `${subdir}`,
		versionExpansion:     `${version}`,
	},

	&localSourceParser{}, // Handles a template source that's a local directory.

	&gitSourceParser{
		// This source parser recognizes template sources like
		// github.com/abcxyz/abc.git//t/react_template?ref=latest.
		// This is the old template location format from abc <=0.2
		// when we used the go-getter library. We don't attempt to
		// handle all the cases supported by go-getter, just the
		// ones that we know people use.
		re: regexp.MustCompile(
			`^` + // Anchor the start, must match the entire input
				`(?P<host>[a-zA-Z0-9_.-]+)` +
				`/` +
				`(?P<org>[a-zA-Z0-9_-]+)` +
				`/` +
				`(?P<repo>[a-zA-Z0-9_-]+)` +
				`\.git` +
				`(//(?P<subdir>[^?]*))?` + // Optional subdir
				`(\?ref=(?P<version>[a-zA-Z0-9_/.-]+))?` + // optional ?ref=branch_or_tag
				`$`), // Anchor the end, must match the entire input
		httpsRemoteExpansion: `https://${host}/${org}/${repo}.git`,
		sshRemoteExpansion:   `git@${host}:${org}/${repo}.git`,
		subdirExpansion:      `${subdir}`,
		versionExpansion:     `${version}`,
		defaultVersion:       "latest",
		warning:              `go-getter style URL support will be removed in mid-2024, please use the newer format instead, eg github.com/myorg/myrepo[/subdir]@v1.2.3 (or @latest)`,
	},
}

// ParseSourceParams contains the arguments to ParseSource.
type ParseSourceParams struct {
	// Source could be any of the template source types we accept. Examples:
	//  - github.com/foo/bar@latest
	//  - /a/local/path
	//  - a/relative/path
	//
	// In the case where the source is a local filesystem path, it uses native
	// filesystem separators.
	Source string

	// The value of --git-protocol.
	GitProtocol string
}

// parseSourceWithCwd maps the input template source to a particular kind of
// source (e.g. git) and returns a downloader that will download that source.
//
// source is a template location, like "github.com/foo/bar@v1.2.3". protocol is
// the value of the --protocol flag, like "https".
//
// A list of sourceParsers is accepted as input for the purpose of testing,
// rather than hardcoding the real list of sourceParsers.
func parseSourceWithCwd(ctx context.Context, cwd string, params *ParseSourceParams) (Downloader, error) {
	if strings.HasSuffix(params.Source, specutil.SpecFileName) {
		return nil, fmt.Errorf("the template source argument should be the name of a directory *containing* %s; it should not be the full path to %s",
			specutil.SpecFileName, specutil.SpecFileName)
	}

	for _, sp := range realSourceParsers {
		downloader, ok, err := sp.sourceParse(ctx, cwd, params)
		if err != nil {
			return nil, err //nolint:wrapcheck
		}
		if ok {
			return downloader, nil
		}
	}
	return nil, fmt.Errorf(`template source %q isn't a valid template name or doesn't exist; examples of valid names are: "github.com/myorg/myrepo/subdir@v1.2.3", "github.com/myorg/myrepo/subdir@latest", "./my-local-directory"`, params.Source)
}

// ParseSource is the same as [ParseSourceWithWorkingDir], but it uses the
// current working directory [os.Getwd] as the base path.
func ParseSource(ctx context.Context, params *ParseSourceParams) (Downloader, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current working directory: %w", err)
	}
	return parseSourceWithCwd(ctx, cwd, params)
}

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
