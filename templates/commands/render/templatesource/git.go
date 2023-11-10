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
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/git"
	"github.com/abcxyz/pkg/logging"
	"golang.org/x/exp/slices"
)

var _ sourceParser = (*gitSourceParser)(nil)

// gitSourceParser implements sourceParser for downloading templates from a
// remote git repo.
type gitSourceParser struct {
	// re is a regular expression that must have a capturing group for each
	// group that is used in any of the "expansions" below. For example, if
	// sshRemoteExpansion mentions "${host}", then re must have a group like
	// "(?P<host>[a-zA-Z1-9]+)". See source.go for examples.
	re *regexp.Regexp

	// These fooExpansion strings are passed to
	// https://pkg.go.dev/regexp#Regexp.Expand, which uses the syntax "$1" or
	// "${groupname}" to refer to the values captured by the groups of the regex
	// above.

	// Example: `https://${host}/${org}/${repo}.git`
	httpsRemoteExpansion string
	// Example: `git@${host}:${org}/${repo}.git`
	sshRemoteExpansion string
	// Example: `${subdir}`
	subdirExpansion string
	// Example: `${version}`
	versionExpansion string
	// Will be used as the version if versionExpansion expands to ""
	defaultVersion string

	// If non-empty, will be logged as a warning when parsing succeeds. It's
	// intended for deprecation notices.
	warning string
}

func (g *gitSourceParser) sourceParse(ctx context.Context, cwd string, params *ParseSourceParams) (*ParsedSource, bool, error) {
	logger := logging.FromContext(ctx).With("logger", "gitSourceParser.sourceParse")

	match := g.re.FindStringSubmatchIndex(params.Source)
	if match == nil {
		// It's not an error if this regex match fails, it just means that src
		// isn't formatted as the kind of template source that we're looking
		// for. It's probably something else, like a local directory name, and
		// the caller should continue and try a different sourceParser.
		return nil, false, nil
	}

	var remote string
	switch params.GitProtocol {
	case "https", "":
		remote = string(g.re.ExpandString(nil, g.httpsRemoteExpansion, params.Source, match))
	case "ssh":
		remote = string(g.re.ExpandString(nil, g.sshRemoteExpansion, params.Source, match))
	default:
		return nil, false, fmt.Errorf("protocol %q isn't usable with a template sourced from a remote git repo", params.GitProtocol)
	}

	if g.warning != "" {
		logger.WarnContext(ctx, g.warning)
	}

	version := string(g.re.ExpandString(nil, g.versionExpansion, params.Source, match))
	if version == "" {
		version = g.defaultVersion
	}

	canonicalSource := string(g.re.ExpandString(nil, "${host}/${org}/${repo}", params.Source, match))
	if subdir := string(g.re.ExpandString(nil, "${subdir}", params.Source, match)); subdir != "" {
		canonicalSource += "/" + subdir
	}

	out := &ParsedSource{
		Downloader: &gitDownloader{
			remote:  remote,
			subdir:  string(g.re.ExpandString(nil, g.subdirExpansion, params.Source, match)),
			version: version,
			cloner:  &realCloner{},
			tagser:  &realTagser{},
		},
		HasCanonicalSource: true,
		CanonicalSource:    canonicalSource,
	}

	return out, true, nil
}

// gitDownloader implements templateSource for templates hosted in a remote git
// repo, regardless of which git hosting service it uses.
type gitDownloader struct {
	// An HTTPS or SSH connection string understood by "git clone"
	remote string
	// An optional subdirectory within the git repo that we want
	subdir string

	// either a semver-formatted tag, or the magic value "latest"
	version string

	cloner cloner
	tagser tagser
}

// Download implements Downloader.
func (g *gitDownloader) Download(ctx context.Context, outDir string) error {
	logger := logging.FromContext(ctx).With("logger", "gitDownloader.Download")

	// Validate first before doing expensive work
	subdir, err := common.SafeRelPath(nil, g.subdir) // protect against ".." traversal attacks
	if err != nil {
		return fmt.Errorf("invalid subdirectory: %w", err)
	}

	branchOrTag, err := resolveBranchOrTag(ctx, g.tagser, g.remote, g.version)
	if err != nil {
		return err
	}
	logger.DebugContext(ctx, "resolved version to branchOrTag", "version", g.version, "branchOrTag", branchOrTag)

	// Rather than cloning directly into outDir, we clone into a temp dir. It would
	// be incorrect to clone the whole repo into outDir if the caller only asked
	// for a subdirectory, e.g. "github.com/my-org/my-repo/my-subdir@v1.2.3".
	tmpDir, err := os.MkdirTemp(os.TempDir(), "git-clone-")
	if err != nil {
		return fmt.Errorf("MkdirTemp: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	subdirToCopy := filepath.Join(tmpDir, filepath.FromSlash(subdir))

	if err := g.cloner.Clone(ctx, g.remote, branchOrTag, tmpDir); err != nil {
		return fmt.Errorf("Clone(): %w", err)
	}

	fi, err := os.Stat(subdirToCopy)
	if err != nil {
		if common.IsStatNotExistErr(err) {
			return fmt.Errorf(`the repo %q at tag %q doesn't contain a subdirectory named %q; it's possible that the template exists in the "main" branch but is not part of the release %q`, g.remote, branchOrTag, subdir, branchOrTag)
		}
		return err //nolint:wrapcheck // Stat() returns a decently informative error
	}
	if !fi.IsDir() {
		return fmt.Errorf("the path %q is not a directory", subdir)
	}

	logger.DebugContext(ctx, "cloned repo", "remote", g.remote, "branchOrTag", branchOrTag)

	// Copy only the requested subdir to outDir.
	if err := common.CopyRecursive(ctx, nil, &common.CopyParams{
		DstRoot: outDir,
		SrcRoot: subdirToCopy,
		RFS:     &common.RealFS{},
	}); err != nil {
		return err //nolint:wrapcheck
	}

	return nil
}

// resolveBranchOrTag returns the latest release tag if branchOrTag is "latest", and otherwise
// just returns the input branchOrTag after validating it. The return value always begins
// with "v" (unless there's an error).
func resolveBranchOrTag(ctx context.Context, t tagser, remote, branchOrTag string) (string, error) {
	logger := logging.FromContext(ctx).With("logger", "resolveBranchOrTag")

	if branchOrTag != "latest" {
		if !strings.HasPrefix(branchOrTag, "v") {
			return "", fmt.Errorf(`the template source version %q must start with "v", like "v1.2.3"`, branchOrTag)
		}
		version := branchOrTag[1:] // trim off "v" prefix
		if _, err := semver.StrictNewVersion(version); err != nil {
			return "", fmt.Errorf("the template source requested git tag %q, which is not a valid format for a semver.org version", branchOrTag)
		}
		logger.DebugContext(ctx, `using user provided branchOrTag, no need to look up remote tags`, "branchOrTag", branchOrTag)
		return branchOrTag, nil
	}

	// If we're here, then the user requested version "latest", so we need to
	// look up the latest version.
	logger.DebugContext(ctx, `looking up semver tags to resolve "latest"`, "git_remote", remote)
	tags, err := t.Tags(ctx, remote)
	if err != nil {
		return "", fmt.Errorf("Tags(): %w", err)
	}
	versions := make([]*semver.Version, 0, len(tags))
	for _, t := range tags {
		if !strings.HasPrefix(t, "v") {
			logger.DebugContext(ctx, "ignoring tag that doesn't start with 'v'", "tag", t)
			continue
		}
		sv, err := semver.StrictNewVersion(t[1:])
		if err != nil {
			logger.DebugContext(ctx, "ignoring non-semver-formatted tag", "tag", t)
			continue // This is not a semver release tag
		}

		// Only tags that look like vN.N.N (with no suffix like "-alpha") are
		// eligible to be considered "latest". This avoids somebody accidentally
		// getting a template that wasn't intended to be released.
		if len(sv.Prerelease()) > 0 || len(sv.Metadata()) > 0 {
			logger.DebugContext(ctx, "ignoring tag that has extra prelease or metadata suffixes", "tag", t)
			continue
		}
		versions = append(versions, sv)
	}

	if len(versions) == 0 {
		return "", fmt.Errorf(`the template source requested the "latest" release, but there were no semver-formatted tags beginning with "v" in %q. Available tags were: %v`, remote, tags)
	}

	max := slices.MaxFunc(versions, func(l, r *semver.Version) int {
		return l.Compare(r)
	})

	return "v" + max.Original(), nil
}

// A fakeable interface around the lower-level git Clone function, for testing.
type cloner interface {
	Clone(ctx context.Context, remote, branchOrTag, outDir string) error
}

type realCloner struct{}

func (r *realCloner) Clone(ctx context.Context, remote, branchOrTag, outDir string) error {
	return git.Clone(ctx, remote, branchOrTag, outDir) //nolint:wrapcheck
}

// A fakeable interface around the lower-level git Tags function, for testing.
type tagser interface {
	Tags(ctx context.Context, remote string) ([]string, error)
}

type realTagser struct{}

func (r *realTagser) Tags(ctx context.Context, remote string) ([]string, error) {
	return git.Tags(ctx, remote) //nolint:wrapcheck
}
