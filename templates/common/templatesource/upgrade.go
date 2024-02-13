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

	"github.com/abcxyz/abc/templates/common/git"
)

var (
	// A manifest file has the "location type" that controls how the location
	// will be parsed. E.g. with location type "remote_git" we expect the
	// location to look like "github.com/foo/bar/baz". With location type
	// "local_git" we expect the location to look like a local path like "a/b".
	upgradeDownloaderFactories = map[string]upgradeDownloaderFactory{
		LocTypeRemoteGit: remoteGitUpgradeDownloaderFactory,
		LocTypeLocalGit:  localGitUpgradeDownloaderFactory,
	}

	// Used only when location type is remote_git. Parses a string like
	// github.com/foo/bar/baz.
	remoteGitUpgradeLocationRE = regexp.MustCompile(
		`^` + // Anchor the start, must match the entire input
			`(?P<host>github\.com|gitlab\.com)` + // The domain names of known git hosting services
			`/` +
			`(?P<org>[a-zA-Z0-9_-]+)` + // the github org name, e.g. "abcxyz"
			`/` +
			`(?P<repo>[a-zA-Z0-9_-]+)` + // the github repo name, e.g. "abc"
			`(/(?P<subdir>[^@]*))?` + // Optional subdir with leading slash; the leading slash is not part of capturing group ${subdir}
			// Note: there's no "@version" in the context of a manifest file.
			`$`) // Anchor the end, must match the entire input
)

type upgradeDownloaderFactory func(_ context.Context, canonicalLocation, gitProtocol, destDir string) (Downloader, error)

// ForUpgrade takes a location type and canonical location from a manifest file,
// and returns a downloader that will download the latest version of that
// template.
func ForUpgrade(ctx context.Context, canonicalLocation, locType, gitProtocol, destDir string) (Downloader, error) {
	factory, ok := upgradeDownloaderFactories[locType]
	if !ok {
		return nil, fmt.Errorf("unknown location type %q", locType)
	}
	return factory(ctx, canonicalLocation, gitProtocol, destDir)
}

func remoteGitUpgradeDownloaderFactory(ctx context.Context, canonicalLocation, gitProtocol, destDir string) (Downloader, error) {
	downloader, ok, err := newRemoteGitDownloader(&newRemoteGitDownloaderParams{
		re:             remoteGitUpgradeLocationRE,
		input:          canonicalLocation,
		gitProtocol:    gitProtocol,
		defaultVersion: "latest",
	})
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf(`failed parsing canonical location %q with regex "%s"`,
			canonicalLocation, remoteGitUpgradeLocationRE)
	}

	return downloader, nil
}

func localGitUpgradeDownloaderFactory(ctx context.Context, canonicalLocation, gitProtocol, destDir string) (Downloader, error) {
	// When upgrading from a local directory, we enforce that the upgrade source
	// and destination dirs are in the same git workspace. This is a security
	// consideration: if you clone a git workspace that contains a malicious
	// manifest, that manifest shouldn't be able to touch any files outside of
	// the git workspace that it's in.
	//
	// We could relax this in the future if we encounter a legitimate use case.
	sourceGitWorkspace, ok, err := git.Workspace(ctx, canonicalLocation)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	if !ok {
		return nil, fmt.Errorf("for now, upgrading is currently only supported in a git workspace, and %q is not in a git workspace", canonicalLocation)
	}
	destGitWorkspace, ok, err := git.Workspace(ctx, destDir)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	if !ok {
		return nil, fmt.Errorf("for now, when upgrading, the upgrade template source must in a git workspace, and %q is not in a git workspace", destDir)
	}
	if sourceGitWorkspace != destGitWorkspace {
		return nil, fmt.Errorf("for now, when upgrading, the template source and destination directories must be in the same git workspace, but they are %q and %q respectively", sourceGitWorkspace, destGitWorkspace)
	}

	return &LocalDownloader{
		SrcPath: canonicalLocation,
	}, nil
}
