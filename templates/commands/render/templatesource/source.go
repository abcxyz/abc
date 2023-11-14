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

// A Downloader is returned by a sourceParser. It offers the ability to
// download a template, and provides some metadata.
type Downloader interface {
	// Download downloads this template into the given directory.
	Download(ctx context.Context, outDir string) error

	// CanonicalSource() returns the canonical source location for this
	// template, if it exists.
	//
	// A "canonical" location is one that's the same for everybody. When
	// installing a template source like
	// "~/my_downloaded_templates/foo_template", that location is not canonical,
	// because not every has that directory downloaded on their machine. On the
	// other hand, a template location like
	// github.com/abcxyz/gcp-org-terraform-template *is* canonical because
	// everyone everywhere can access it by that name.
	//
	// Canonical template locations are preferred because they make automatic
	// template upgrades easier. Given a destination directory that is the
	// output of a template, we can easily upgrade it if we know the canonical
	// location of the template that created it. We just go look for new git
	// tags at the canonical location.
	//
	// A local template directory is not a canonical location except for one
	// special case: when the template source directory and the destination
	// directory are within the same repo. This supports the case where a single
	// git repo contains templates that are rendered into that repo. Since the
	// relative path between the template directory and the destination
	// directory are the same for everyone who clones the repo, that means the
	// relative path counts as a canonical source.
	//
	// CanonicalSource should only be called after Download() has returned
	// successfully. This lets us account for redirects encountered while
	// downloading.
	//
	// "dest" is the value of --dest. cwd is the current working directory.
	CanonicalSource(ctx context.Context, cwd, dest string) (string, bool, error)
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
