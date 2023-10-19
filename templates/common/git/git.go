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
	"strings"

	"github.com/abcxyz/abc/templates/common"
)

// Clone checks out the given branch or tag from the given repo. It uses the git
// CLI already installed on the system.
//
// To optimize storage and bandwidth, the full git history is not fetched.
//
// "remote" may be any format accepted by git, such as
// https://github.com/abcxyz/abc.git or git@github.com:abcxyz/abc.git .
func Clone(ctx context.Context, remote, branchOrTag, outDir string) error {
	_, _, err := common.Run(ctx, "git", "clone", "--depth", "1", "--branch", branchOrTag, remote, outDir)
	if err != nil {
		return err //nolint:wrapcheck
	}

	links, err := findSymlinks(outDir)
	if err != nil {
		return fmt.Errorf("findSymlinks: %w", err)
	}
	if len(links) > 0 {
		return fmt.Errorf("one or more symlinks were found in %q at %v; for security reasons and to support windows, git repos containing symlinks are not allowed", remote, links)
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

// Tags looks up the tags in the given remote repo.
//
// "remote" may be any format accepted by git, such as
// https://github.com/abcxyz/abc.git or git@github.com:abcxyz/abc.git .
func Tags(ctx context.Context, remote string) ([]string, error) {
	stdout, _, err := common.Run(ctx, "git", "ls-remote", "--tags", remote)
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
