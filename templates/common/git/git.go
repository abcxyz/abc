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

package git

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/run"
)

var sha = regexp.MustCompile("^[0-9a-f]{40}$")

// Clone checks out the given branch, tag or long commit SHA from the given repo.
// It uses the git CLI already installed on the system.
//
// To optimize storage and bandwidth, the full git history is not fetched.
//
// "remote" may be any format accepted by git, such as
// https://github.com/abcxyz/abc.git or git@github.com:abcxyz/abc.git .
func Clone(ctx context.Context, remote, version, outDir string) error {
	if sha.MatchString(version) {
		_, _, err := run.Run(ctx, "git", "clone", remote, outDir)
		if err != nil {
			return err //nolint:wrapcheck
		}

		_, _, err = run.Run(ctx, "git", "-C", outDir, "reset", "--hard", version)
		if err != nil {
			return err //nolint:wrapcheck
		}
	} else {
		_, _, err := run.Run(ctx, "git", "clone", "--depth", "1", "--branch", version, remote, outDir)
		if err != nil {
			return err //nolint:wrapcheck
		}
	}

	links, err := findSymlinks(outDir)
	if err != nil {
		return fmt.Errorf("findSymlinks: %w", err)
	}
	if len(links) > 0 {
		return fmt.Errorf("one or more symlinks were found in %q at %v; for security reasons, git repos containing symlinks are not allowed", remote, links)
	}
	return nil
}

func findSymlinks(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relativeToOutDir, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("Rel(): %w", err)
		}
		if relativeToOutDir == ".git" {
			return fs.SkipDir // skip crawling the git directory to save time.
		}
		fi, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("Lstat(): %w", err)
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			return nil
		}

		out = append(out, relativeToOutDir)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("WalkDir: %w", err) // There was some filesystem error while crawling.
	}

	return out, nil
}

// RemoteTags looks up the tags in the given remote repo. If there are no tags,
// that's not an error, and the returned slice is len 0.
//
// "remote" may be any format accepted by git, such as
// https://github.com/abcxyz/abc.git or git@github.com:abcxyz/abc.git .
func RemoteTags(ctx context.Context, remote string) ([]string, error) {
	stdout, _, err := run.Run(ctx, "git", "ls-remote", "--tags", remote)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	lineScanner := bufio.NewScanner(strings.NewReader(stdout))
	var tags []string
	for lineScanner.Scan() {
		line := lineScanner.Text()
		fields := strings.Fields(line)
		prefixedTag := fields[len(fields)-1]
		if strings.HasSuffix(prefixedTag, "^{}") {
			// Skip the weird extra duplicate tags ending with "^{}" that git
			// prints for some reason.
			continue
		}
		tag := strings.TrimPrefix(prefixedTag, "refs/tags/")
		tags = append(tags, tag)
	}

	return tags, nil
}

// Workspace looks for the presence of a .git directory in parent directories
// to determine the root directory of the git workspace containing "path".
// Returns false if the given path is not inside a git workspace.
//
// The input path need not actually exist yet. For example, suppose "/a/b" is a
// git workspace, which means that "/a/b/.git" is a directory that exists.
// Calling Workspace("/a/b/c") will return "/a/b" whether or not "c" actually
// exists yet. This supports the case where the user is rendering into a
// directory that doesn't exist yet but will be created by the render operation.
func Workspace(ctx context.Context, path string) (string, bool, error) {
	// Alternative considered and rejected: use "git rev-parse --git-dir" to
	// print the .git dir. We can't use that here because that would require the
	// directory to already exist in the filesystem, but this function is called
	// for hypothetical directories that might not exist yet.
	for {
		fileInfo, err := os.Stat(filepath.Join(path, ".git"))
		if err != nil && !common.IsStatNotExistErr(err) {
			return "", false, err //nolint:wrapcheck
		}
		if fileInfo != nil && fileInfo.IsDir() {
			return path, true, nil
		}
		// At this point, we know that one of two things is true:
		//   - $path/.git doesn't exist
		//   - $path/.git is a file (not a directory)
		//
		// In both cases, we'll continue crawling upward in the directory tree
		// looking for a .git directory.
		pathBefore := path
		path = filepath.Dir(path)
		if path == pathBefore || len(path) <= 1 {
			// We crawled to the root of the filesystem without finding a .git
			// directory.
			return "", false, nil
		}
	}
}

// IsClean returns false if the given git workspace has any uncommitted changes,
// and otherwise returns true. Returns error if dir is not in a git workspace.
func IsClean(ctx context.Context, dir string) (bool, error) {
	// Design decision: use a single "git status" command rather than combine
	// "git diff-index" and "git ls-files" to detect all the possibilities of
	// staged/unstaged/untracked changes. "git status" is arguably less stable
	// because it's not a git "plumbing" command, but the --porcelain option
	// promises stable output, so it's good enough.
	// https://stackoverflow.com/a/2658301
	args := []string{"git", "-C", dir, "status", "--porcelain"}
	stdout, _, err := run.Run(ctx, args...)
	if err != nil {
		return false, err //nolint:wrapcheck
	}
	return strings.TrimSpace(stdout) == "", nil
}

// HeadTags looks at a local git workspace and returns the names of all tags
// that point to the current HEAD commit. If there are no such tags, returns
// empty slice, this is not an error.
func HeadTags(ctx context.Context, dir string) ([]string, error) {
	args := []string{"git", "-C", dir, "for-each-ref", "--points-at", "HEAD", "refs/tags/*"}
	stdout, _, err := run.Run(ctx, args...)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	trimmed := strings.TrimSpace(stdout)
	if len(trimmed) == 0 {
		return nil, nil
	}
	var out []string //nolint:prealloc
	for _, line := range strings.Split(trimmed, "\n") {
		tokens := strings.Split(line, "\t")
		const tagPrefix = "refs/tags/"
		if len(tokens) != 2 || !strings.HasPrefix(tokens[1], tagPrefix) {
			return nil, fmt.Errorf("internal error: unexpected output format from \"git for-each-ref\": %s", trimmed)
		}

		tag := tokens[1]
		tag = tag[len(tagPrefix):]
		tag = strings.TrimSpace(tag)
		out = append(out, tag)
	}
	return out, nil
}

// CurrentSHA returns the full SHA of the current HEAD in the given git
// workspace.
func CurrentSHA(ctx context.Context, dir string) (string, error) {
	args := []string{"git", "-C", dir, "rev-parse", "HEAD"}
	stdout, _, err := run.Run(ctx, args...)
	if err != nil {
		return "", err //nolint:wrapcheck
	}
	return strings.TrimSpace(stdout), nil
}

// ParseSemverTag parses a string of the form "v1.2.3" into a semver tag. In abc
// CLI, we require that tags begin with "v", and anything else is a parse error.
//
// WARNING: the returned semver.Version has had the "v" prefix stripped,
// so the string returned from .Original() will be missing the "v".
func ParseSemverTag(t string) (*semver.Version, error) {
	if !strings.HasPrefix(t, "v") {
		return nil, fmt.Errorf("tag is not a valid semver tag because it didn't begin with 'v': %q", t)
	}
	sv, err := semver.StrictNewVersion(t[1:])
	if err != nil {
		return nil, fmt.Errorf("error parsing %q as a semver: %w", t, err)
	}
	return sv, nil
}
