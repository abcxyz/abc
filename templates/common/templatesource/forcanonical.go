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

// TODO tests
// TODO rename to indicate upgrade? And change error messages to be upgrade specific?
var (
	// TODO doc
	canonicalDownloaderFactories = map[string]canonicalDownloaderFactory{
		LocTypeRemoteGit: remoteGitCanonicalFactory,
		LocTypeLocalGit:  localGitCanonicalFactory,
	}

	remoteGitCanonicalLocationRE = regexp.MustCompile(
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

type canonicalDownloaderFactory func(_ context.Context, canonicalLocation, gitProtocol, destDir string) (Downloader, error)

func ForCanonical(ctx context.Context, canonicalLocation, locType, gitProtocol, destDir string) (Downloader, error) {
	factory, ok := canonicalDownloaderFactories[locType]
	if !ok {
		return nil, fmt.Errorf("unknown location type %q", locType)
	}
	return factory(ctx, canonicalLocation, gitProtocol, destDir)
}

func remoteGitCanonicalFactory(ctx context.Context, canonicalLocation, gitProtocol, destDir string) (Downloader, error) {
	downloader, ok, err := newRemoteGitDownloader(&newRemoteGitDownloaderParams{
		re:             remoteGitCanonicalLocationRE,
		input:          canonicalLocation,
		gitProtocol:    gitProtocol,
		defaultVersion: "latest",
	})
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf(`failed parsing canonical location %q with regex "%s"`,
			canonicalLocation, remoteGitCanonicalLocationRE)
	}

	return downloader, nil
}

func localGitCanonicalFactory(ctx context.Context, canonicalLocation, gitProtocol, destDir string) (Downloader, error) {
	// TODO test

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
