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
		re:                    g.re,
		input:                 params.Source,
		gitProtocol:           params.FlagGitProtocol,
		defaultVersion:        g.defaultVersion,
		flagUpgradeChannel:    params.FlagUpgradeChannel,
		requireUpgradeChannel: params.RequireUpgradeChannel,
	})
}

// newRemoteGitDownloaderParams contains the parameters to
// newRemoteGitDownloader.
type newRemoteGitDownloaderParams struct {
	// defaultVersion is the template version (e.g. "latest", "v1.2.3") that
	// will be used if the "re" regular expression either doesn't have a
	// matching group named "version", or
	defaultVersion        string
	gitProtocol           string
	input                 string
	flagUpgradeChannel    string
	requireUpgradeChannel bool
	re                    *regexp.Regexp
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
		canonicalSource:       canonicalSource,
		cloner:                &realCloner{},
		remote:                remote,
		subdir:                subdir,
		version:               version,
		flagUpgradeChannel:    p.flagUpgradeChannel,
		requireUpgradeChannel: p.requireUpgradeChannel,
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

	// The value of --upgrade-channel.
	flagUpgradeChannel string

	// Return an error if we can't infer an upgrade channel to put in the
	// manifest.
	requireUpgradeChannel bool
}

// Download implements Downloader.
func (g *remoteGitDownloader) Download(ctx context.Context, _, templateDir, _ string) (_ *DownloadMetadata, rErr error) {
	logger := logging.FromContext(ctx).With("logger", "remoteGitDownloader.Download")

	// Validate first before doing expensive work
	subdir, err := common.SafeRelPath(nil, g.subdir) // protect against ".." traversal attacks
	if err != nil {
		return nil, fmt.Errorf("invalid subdirectory: %w", err)
	}

	// Rather than cloning directly into templateDir, we clone into a temp dir.
	// It would be incorrect to clone the whole repo into templateDir if the
	// caller only asked for a subdirectory, e.g.
	// "github.com/my-org/my-repo/my-subdir@v1.2.3".
	tempTracker := tempdir.NewDirTracker(&common.RealFS{}, false)
	defer tempTracker.DeferMaybeRemoveAll(ctx, &rErr)
	tmpDir, err := tempTracker.MkdirTempTracked("", "git-clone-")
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	subdirToCopy := filepath.Join(tmpDir, subdir)

	if err := g.cloner.Clone(ctx, g.remote, tmpDir); err != nil {
		return nil, fmt.Errorf("Clone() of %s: %w", g.remote, err)
	}

	versionToCheckout, defaultUpgradeChannel, err := resolveVersion(ctx, tmpDir, g.version)
	if err != nil {
		return nil, err
	}

	upgradeChannel := defaultUpgradeChannel
	if g.flagUpgradeChannel != "" {
		upgradeChannel = g.flagUpgradeChannel
	}

	if upgradeChannel == "" && g.requireUpgradeChannel {
		return nil, fmt.Errorf("when installing from a SHA, you must provide the --upgrade-channel flag to make upgrading easy in the future; this will control which branch/tag upgrades will be pulled from; common values are --upgrade-channel=main (to track the main branch) or --upgrade-channel=latest (to track the then-latest semver release tag); alternatively you can provide the --skip-manifest flag which will disable the ability to upgrade this template installation")
	}

	logger.DebugContext(ctx, "resolved version from",
		"input", g.version,
		"to", versionToCheckout)

	if err := git.Checkout(ctx, versionToCheckout, tmpDir); err != nil {
		return nil, fmt.Errorf("Checkout(): %w", err)
	}

	fi, err := os.Stat(subdirToCopy)
	if err != nil {
		if common.IsNotExistErr(err) {
			return nil, fmt.Errorf(`the repo %q at version %q doesn't contain a subdirectory named %q; it's possible that the template exists in the "main" branch but is not part of the release %q`, g.remote, versionToCheckout, subdir, versionToCheckout)
		}
		return nil, err //nolint:wrapcheck // Stat() returns a decently informative error
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("the path %q is not a directory", subdir)
	}
	logger.DebugContext(ctx, "cloned repo",
		"remote", g.remote,
		"version", versionToCheckout)

	// Copy only the requested subdir to templateDir.
	if err := common.CopyRecursive(ctx, nil, &common.CopyParams{
		DstRoot: templateDir,
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
	canonicalVersion, ok, err := gitCanonicalVersion(ctx, tmpDir)
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
		LocationType:    RemoteGit,
		Version:         canonicalVersion,
		UpgradeChannel:  upgradeChannel,
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

// resolveVersion returns the latest release tag if version is "latest", and
// otherwise just returns the input version. The returned tagBranchOrSHA is
// either a branch, tag, or a long commit SHA (unless there's an error). The
// returned upgradeChannel is an auto-detected upgrade channel that should only
// be used if the user didn't specify one with --upgrade-channel.
func resolveVersion(ctx context.Context, tmpDir, version string) (tagBranchOrSHA, upgradeChannel string, _ error) {
	isSemver := false
	if len(version) > 0 {
		_, err := semver.StrictNewVersion(version[1:])
		isSemver = err == nil
	}

	switch {
	case version == "":
		return "", "", fmt.Errorf(`the template source version cannot be empty; consider providing one of @main, @latest, @tagname, or @branchname`)
	case version == Latest:
		tagBranchOrSHA, err := resolveLatest(ctx, tmpDir)
		if err != nil {
			return "", "", err
		}
		return tagBranchOrSHA, Latest, nil
	case sha.MatchString(version):
		// If the requested version is a SHA, then we'll require the user to
		// specify which upgrade channel they want by returning empty string for
		// upgradeChannel.
		return version, "", nil
	case isSemver:
		// If the requested version is a vX.Y.Z semver version, then the
		// behavior on upgrade should be to upgrade to the latest semver
		// release.
		return version, Latest, nil
	}

	isBranch, err := common.Exists(filepath.Join(tmpDir, ".git", "refs", "heads", version))
	if err != nil {
		return "", "", err //nolint:wrapcheck
	}
	if isBranch {
		// When the user is installing from a given branch like "abc render
		// github.com/foo/bar@main", then when they upgrade in the future,
		// we should upgrade to the tip of that same branch.
		return version, version, nil
	}

	isTag, err := common.Exists(filepath.Join(tmpDir, ".git", "refs", "tags", version))
	if err != nil {
		return "", "", err //nolint:wrapcheck
	}
	if isTag {
		// When the user is installing from a given tag like "abc render
		// github.com/foo/bar@my-tag", then when they upgrade in the future,
		// we should upgrade to the latest tag.
		return version, Latest, nil
	}
	return "", "", fmt.Errorf("%q is not a tag, branch, or SHA that exists in this repo", version)
}

// resolveLatest retrieves the tags from the locally cloned repository and returns the
// highest semver tag. An error is thrown if no semver tags are found.
func resolveLatest(ctx context.Context, tmpDir string) (string, error) {
	logger := logging.FromContext(ctx).With("logger", "resolveLatest")

	logger.DebugContext(ctx, `looking up semver tags to resolve "latest"`)
	tags, err := git.LocalTags(ctx, tmpDir)
	if err != nil {
		return "", fmt.Errorf("Tags(): %w", err)
	}
	versions := make([]*semver.Version, 0, len(tags))
	for _, t := range tags {
		sv, err := parseSemverTag(t)
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
		return "", fmt.Errorf(`the template source requested the "latest" release, but there were no semver-formatted tags beginning with "v". Available tags were: %v`, tags)
	}

	maxVer := slices.MaxFunc(versions, func(l, r *semver.Version) int {
		return l.Compare(r)
	})

	return "v" + maxVer.Original(), nil
}

// A fakeable interface around the lower-level git Clone function, for testing.
type cloner interface {
	Clone(ctx context.Context, remote, destDir string) error
}

type realCloner struct{}

func (r *realCloner) Clone(ctx context.Context, remote, destDir string) error {
	return git.Clone(ctx, remote, destDir) //nolint:wrapcheck
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
