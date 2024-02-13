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
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"github.com/Masterminds/semver/v3"
	"golang.org/x/exp/slices"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/git"
	"github.com/abcxyz/abc/templates/common/tempdir"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/sets"
)

var _ sourceParser = (*remoteGitSourceParser)(nil)

// remoteGitSourceParser implements sourceParser for downloading templates from a
// remote git repo.
type remoteGitSourceParser struct {
	// re is a regular expression that must have a capturing group for each
	// group that is used in any of the "expansions" below. For example, if
	// sshRemoteExpansion mentions "${host}", then re must have a group like
	// "(?P<host>[a-zA-Z1-9]+)". See source.go for examples.
	re *regexp.Regexp

	// These fooExpansion strings are passed to
	// https://pkg.go.dev/regexp#Regexp.Expand, which uses the syntax "$1" or
	// "${groupname}" to refer to the values captured by the groups of the regex
	// above.

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

func (g *remoteGitSourceParser) sourceParse(ctx context.Context, params *ParseSourceParams) (Downloader, bool, error) {
	return newRemoteGitDownloader(&newRemoteGitDownloaderParams{
		re:             g.re,
		input:          params.Source,
		gitProtocol:    params.GitProtocol,
		defaultVersion: g.defaultVersion,
	})
}

// newRemoteGitDownloaderParams contains the parameters to
// newRemoteGitDownloader.
type newRemoteGitDownloaderParams struct {
	// defaultVersion is the template version (e.g. "latest", "v1.2.3") that
	// will be used if the "re" regular expression either doesn't have a
	// matching group named "version", or
	defaultVersion string
	gitProtocol    string
	input          string
	re             *regexp.Regexp
}

// newRemoteGitDownloader is basically a fancy constructor for
// remoteGitDownloader. It returns false if the provided input doesn't match the
// provided regex.
func newRemoteGitDownloader(p *newRemoteGitDownloaderParams) (Downloader, bool, error) {
	match := p.re.FindStringSubmatchIndex(p.input)
	if match == nil {
		return nil, false, nil
	}

	remote, err := gitRemote(p.re, match, p.input, p.gitProtocol)
	if err != nil {
		return nil, false, err
	}

	version := string(p.re.ExpandString(nil, "${version}", p.input, match))
	if version == "" {
		version = p.defaultVersion
	}

	canonicalSource := string(p.re.ExpandString(nil, "${host}/${org}/${repo}", p.input, match))
	if subdir := string(p.re.ExpandString(nil, "${subdir}", p.input, match)); subdir != "" {
		canonicalSource += "/" + subdir
	}

	subdir := string(p.re.ExpandString(nil, "${subdir}", p.input, match))

	return &remoteGitDownloader{
		canonicalSource: canonicalSource,
		cloner:          &realCloner{},
		remote:          remote,
		subdir:          subdir,
		tagser:          &realTagser{},
		version:         version,
	}, true, nil
}

// remoteGitDownloader implements templateSource for templates hosted in a
// remote git repo, regardless of which git hosting service it uses.
type remoteGitDownloader struct {
	// An HTTPS or SSH connection string understood by "git clone".
	remote string
	// An optional subdirectory within the git repo that we want.
	subdir string

	// A tag, branch, SHA, or the magic value "latest".
	version string

	canonicalSource string

	cloner cloner
	tagser tagser

	// It's too hard in tests to generate a clean git repo, so we provide
	// this option to just ignore the fact that the git repo is dirty.
	allowDirty bool
}

// Download implements Downloader.
func (g *remoteGitDownloader) Download(ctx context.Context, cwd, destDir string) (_ *DownloadMetadata, rErr error) {
	logger := logging.FromContext(ctx).With("logger", "remoteGitDownloader.Download")

	// Validate first before doing expensive work
	subdir, err := common.SafeRelPath(nil, g.subdir) // protect against ".." traversal attacks
	if err != nil {
		return nil, fmt.Errorf("invalid subdirectory: %w", err)
	}

	versionToDownload, err := resolveVersion(ctx, g.tagser, g.remote, g.version)
	if err != nil {
		return nil, err
	}
	logger.DebugContext(ctx, "resolved version from",
		"input", g.version,
		"to", versionToDownload)

	// Rather than cloning directly into destDir, we clone into a temp dir. It would
	// be incorrect to clone the whole repo into destDir if the caller only asked
	// for a subdirectory, e.g. "github.com/my-org/my-repo/my-subdir@v1.2.3".
	tempTracker := tempdir.NewDirTracker(&common.RealFS{}, false)
	defer tempTracker.DeferMaybeRemoveAll(ctx, &rErr)
	tmpDir, err := tempTracker.MkdirTempTracked("", "git-clone-")
	if err != nil {
		return nil, fmt.Errorf("MkdirTemp: %w", err)
	}
	subdirToCopy := filepath.Join(tmpDir, subdir)

	if err := g.cloner.Clone(ctx, g.remote, versionToDownload, tmpDir); err != nil {
		return nil, fmt.Errorf("Clone(): %w", err)
	}

	fi, err := os.Stat(subdirToCopy)
	if err != nil {
		if common.IsStatNotExistErr(err) {
			return nil, fmt.Errorf(`the repo %q at tag %q doesn't contain a subdirectory named %q; it's possible that the template exists in the "main" branch but is not part of the release %q`, g.remote, versionToDownload, subdir, versionToDownload)
		}
		return nil, err //nolint:wrapcheck // Stat() returns a decently informative error
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("the path %q is not a directory", subdir)
	}

	logger.DebugContext(ctx, "cloned repo",
		"remote", g.remote,
		"version", versionToDownload)

	// Copy only the requested subdir to destDir.
	if err := common.CopyRecursive(ctx, nil, &common.CopyParams{
		DstRoot: destDir,
		SrcRoot: subdirToCopy,
		FS:      &common.RealFS{},
		Visitor: func(relPath string, de fs.DirEntry) (common.CopyHint, error) {
			return common.CopyHint{
				Skip: relPath == ".git",
			}, nil
		},
	}); err != nil {
		return nil, err //nolint:wrapcheck
	}

	// You might wonder: why don't we just use the downloaded branch/tag/SHA as
	// the template version for the manifest? Multiple reasons:
	//   - There might be a "better" name. E.g. the user specified a SHA
	//     but there exists a semver tag pointing to the same SHA, which is
	//     "better."
	//   - The user may have specified a branch name, but we don't allow branches
	//     to be used as template versions in manifests because they change
	//     frequently.
	canonicalVersion, ok, err := gitCanonicalVersion(ctx, tmpDir, g.allowDirty)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("internal error: no version number was available after git clone")
	}

	vars, err := gitTemplateVars(ctx, tmpDir)
	if err != nil {
		return nil, err
	}

	dlMeta := &DownloadMetadata{
		IsCanonical:     true, // Remote git sources are always canonical.
		CanonicalSource: g.canonicalSource,
		LocationType:    LocTypeRemoteGit,
		HasVersion:      true, // Remote git sources always have a tag or SHA.
		Version:         canonicalVersion,
		Vars:            *vars,
	}

	return dlMeta, nil
}

func (g *remoteGitDownloader) CanonicalSource(context.Context, string, string) (string, bool, error) {
	return g.canonicalSource, true, nil
}

func gitTemplateVars(ctx context.Context, srcDir string) (*DownloaderVars, error) {
	_, ok, err := git.Workspace(ctx, srcDir)
	if err != nil {
		return nil, fmt.Errorf("failed determining git workspace for %q: %w", srcDir, err)
	}
	if !ok {
		// The source path isn't a git repo, so leave all the _git_tag, etc
		// fields as empty string.
		return &DownloaderVars{}, nil
	}

	sha, err := git.CurrentSHA(ctx, srcDir)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}

	// The boolean return is ignored because we want empty string in the case where there's no tag.
	tag, _, err := bestHeadTag(ctx, srcDir)
	if err != nil {
		return nil, err
	}

	return &DownloaderVars{
		GitSHA:      sha,
		GitShortSHA: sha[:7],
		GitTag:      tag,
	}, nil
}

// resolveVersion returns the latest release tag if version is "latest", and otherwise
// just returns the input version. The return value is either a branch, tag, or
// a long commit SHA (unless there's an error).
func resolveVersion(ctx context.Context, t tagser, remote, version string) (string, error) {
	logger := logging.FromContext(ctx).With("logger", "resolveVersion")

	switch version {
	case "":
		return "", fmt.Errorf("the template source version cannot be empty")
	case "latest":
		return resolveLatest(ctx, t, remote, version)
	default:
		logger.DebugContext(ctx, "using user provided version and skipping remote tags lookup", "version", version)
		return version, nil
	}
}

// resolveLatest retrieves the tags from the remote repository and returns the
// highest semver tag. An error is thrown if no semver tags are found.
func resolveLatest(ctx context.Context, t tagser, remote, version string) (string, error) {
	logger := logging.FromContext(ctx).With("logger", "resolveVersion")

	logger.DebugContext(ctx, `looking up semver tags to resolve "latest"`, "git_remote", remote)
	tags, err := t.Tags(ctx, remote)
	if err != nil {
		return "", fmt.Errorf("Tags(): %w", err)
	}
	versions := make([]*semver.Version, 0, len(tags))
	for _, t := range tags {
		sv, err := git.ParseSemverTag(t)
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
	Clone(ctx context.Context, remote, version, destDir string) error
}

type realCloner struct{}

func (r *realCloner) Clone(ctx context.Context, remote, version, destDir string) error {
	return git.Clone(ctx, remote, version, destDir) //nolint:wrapcheck
}

// A fakeable interface around the lower-level git Tags function, for testing.
type tagser interface {
	Tags(ctx context.Context, remote string) ([]string, error)
}

type realTagser struct{}

func (r *realTagser) Tags(ctx context.Context, remote string) ([]string, error) {
	return git.RemoteTags(ctx, remote) //nolint:wrapcheck
}

// gitRemote returns a git remote string (see "man git-remote") for the given
// remote git repo.
//
// The host, org, and repo name are provided by the given regex match. The
// "match" parameter must be the result of calling re.FindStringSubmatchIndex(),
// and must not be nil. reInput must be the string passed to
// re.FindStringSubmatchIndex(), this allows us to extract the matched host,
// org, and repo names that were match by the regex.
//
// The given regex must have matching groups (i.e. P<foo>) named "host", "org",
// and "repo".
func gitRemote(re *regexp.Regexp, match []int, reInput, gitProtocol string) (string, error) {
	// Sanity check that the regular expression has the necessary named subgroups.
	wantSubexps := []string{"host", "org", "repo"}
	missingSubexps := sets.Subtract(wantSubexps, re.SubexpNames())
	if len(missingSubexps) > 0 {
		return "", fmt.Errorf("internal error: regexp expansion didn't have a named subgroup for: %v", missingSubexps)
	}

	switch gitProtocol {
	case "https", "":
		return string(re.ExpandString(nil, "https://${host}/${org}/${repo}.git", reInput, match)), nil
	case "ssh":
		return string(re.ExpandString(nil, "git@${host}:${org}/${repo}.git", reInput, match)), nil
	default:
		return "", fmt.Errorf("protocol %q isn't usable with a template sourced from a remote git repo", gitProtocol)
	}
}
