// Copyright 2023 Lumberjack authors (see AUTHORS file)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package version

import (
	"golang.org/x/mod/semver"

	"github.com/abcxyz/pkg/buildinfo"
)

var (
	// Name is the name of the binary. This can be overridden by the build
	// process.
	name string
	Name = valueOrFallback(name, func() string {
		return "abc"
	})

	// Version is the main package version. This can be overridden by the build
	// process.
	version string
	Version = valueOrFallback(version, buildinfo.Version)

	// Commit is the git sha. This can be overridden by the build process.
	commit string
	Commit = valueOrFallback(commit, buildinfo.Commit)

	// OSArch is the operating system and architecture combination.
	OSArch = buildinfo.OSArch()

	// HumanVersion is the compiled version.
	HumanVersion = Name + " " + Version + " (" + Commit + ", " + OSArch + ")"
)

// IsReleaseBuild returns true if this binary was built by goreleaser as part of
// the official release process (as opposed to a user just running "go build",
// or running in a CI environment, or something else).
func IsReleaseBuild() bool {
	// Binary from release build should follow semver format.
	// When installing via `go install github.com/abcxyz/abc/cmd/abc@vXXX`,
	// `Version` is `vXXX` for example `v0.1.0` with `v` prefix.
	// When installing via downloading artifacts in GitHub directly,
	// `Version` is `XXX` for example `0.1.0` without `v` prefix.
	return semver.IsValid("v"+Version) || semver.IsValid(Version)
}

func valueOrFallback(val string, fn func() string) string {
	if val != "" {
		return val
	}
	return fn()
}
