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
	"regexp"
	"strings"

	"github.com/abcxyz/abc/templates/common/specutil"
)

const (
	Latest = "latest"

	LocalNonGit LocationType = "local" // local never appears in a manifest, because only canonical template sources appear in a manifest
	LocalGit    LocationType = "local_git"
	RemoteGit   LocationType = "remote_git"
)

// LocationType is an enum describing where we got a template from.
type LocationType string

// sourceParser is implemented for each particular kind of template source (git,
// local file, etc.).
type sourceParser interface {
	// sourceParse attempts to parse the given src. If the src is recognized as
	// being downloadable by this sourceParser, then it returns true, along with
	// a downloader that can download that template, and other metadata. See
	// ParsedSource.
	sourceParse(ctx context.Context, params *ParseSourceParams) (Downloader, bool, error)
}

// realSourceParsers contains the non-test sourceParsers.
var realSourceParsers = []sourceParser{
	// This source parser recognizes template sources like
	// "github.com/myorg/myrepo@v1.2.3" (and variants thereof).
	&remoteGitSourceParser{
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
		subdirExpansion:  `${subdir}`,
		versionExpansion: `${version}`,
	},

	&localSourceParser{}, // Handles a template source that's a local directory.

	&remoteGitSourceParser{
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
		subdirExpansion:  `${subdir}`,
		versionExpansion: `${version}`,
		defaultVersion:   Latest,
		warning:          `go-getter style URL support will be removed in mid-2024, please use the newer format instead, eg github.com/myorg/myrepo[/subdir]@v1.2.3 (or @latest)`,
	},
}

// ParseSourceParams contains the arguments to ParseSource.
type ParseSourceParams struct {
	// The working directory that we're in. Used to resolve relative paths.
	CWD string

	// Source could be any of the template source types we accept. Examples:
	//  - github.com/foo/bar@latest
	//  - /a/local/path
	//  - a/relative/path
	//
	// In the case where the source is a local filesystem path, it uses native
	// filesystem separators.
	Source string

	// The value of --git-protocol.
	FlagGitProtocol string

	// The value of --upgrade-channel.
	FlagUpgradeChannel string

	// Reject the user input with an error in the case where an upgrade channel
	// can't be determined from the combination of the location string and
	// flags.
	RequireUpgradeChannel bool
}

// ParseSource maps the input template source to a particular kind of
// source (e.g. git) and returns a downloader that will download that source.
//
// source is a template location, like "github.com/foo/bar@v1.2.3". protocol is
// the value of the --protocol flag, like "https".
//
// A list of sourceParsers is accepted as input for the purpose of testing,
// rather than hardcoding the real list of sourceParsers.
func ParseSource(ctx context.Context, params *ParseSourceParams) (Downloader, error) {
	if strings.HasSuffix(params.Source, specutil.SpecFileName) {
		return nil, fmt.Errorf("the template source argument should be the name of a directory *containing* %s; it should not be the full path to %s",
			specutil.SpecFileName, specutil.SpecFileName)
	}

	for _, sp := range realSourceParsers {
		downloader, ok, err := sp.sourceParse(ctx, params)
		if err != nil {
			return nil, err
		}
		if ok {
			return downloader, nil
		}
	}
	return nil, fmt.Errorf(`template source %q isn't a valid template name or doesn't exist; examples of valid names are: "github.com/myorg/myrepo/subdir@v1.2.3", "github.com/myorg/myrepo/subdir@latest", "./my-local-directory"`, params.Source)
}
