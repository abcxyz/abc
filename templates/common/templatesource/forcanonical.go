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
	"fmt"
	"regexp"
)

// TODO rename, doc
var (
	m = map[string]upgradeDownloaderFactory{
		LocTypeRemoteGit: remoteGitUpgradeFactory,
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

func ForCanonical(canonicalLocation, locType, gitProtocol string) (Downloader, error) {
	factory, ok := m[locType]
	if !ok {
		return nil, fmt.Errorf("unknown location type %q", locType)
	}
	return factory(canonicalLocation, gitProtocol)
}

// TODO name: "canonical" instead of "upgrade"?
type upgradeDownloaderFactory func(canonicalLocation, gitProtocol string) (Downloader, error)

func remoteGitUpgradeFactory(canonicalLocation, gitProtocol string) (Downloader, error) {
	downloader, ok, err := newRemoteGitDownloader(&ngdParams{
		re:             remoteGitCanonicalLocationRE,
		input:          canonicalLocation,
		gitProtocol:    gitProtocol,
		defaultVersion: "latest",
	})
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf(`failed parsing upgrade location %q with regex "%s"`,
			canonicalLocation, remoteGitCanonicalLocationRE)
	}

	return downloader, nil
}
