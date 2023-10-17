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
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Clone checks out the given branch or tag from the given repo. It uses the git
// CLI already installed on the system.
//
// To optimize storage and bandwidth, the full git history is not fetched.
//
// "remote" may be any format accepted by git, such as
// https://github.com/abcxyz/abc.git or git@github.com:abcxyz/abc.git .
func Clone(ctx context.Context, remote, branchOrTag, outDir string) error {
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--branch", branchOrTag, remote, outDir)
	cmd.Stderr = &bytes.Buffer{}
	cmd.Stdout = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git exec of %v failed: %w\nstdout: %s\nstderr: %s", cmd.Args, err, cmd.Stdout, cmd.Stderr)
	}

	// Make sure there are no symlinks.
	if err := filepath.WalkDir(outDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err // There was some filesystem error while crawling.
		}
		if path == filepath.Join(outDir, ".git") {
			return nil // skip crawling the git directory to save time.
		}
		fi, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("Lstat(): %w", err)
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			return nil
		}
		relPathToSymlink, err := filepath.Rel(outDir, path)
		if err != nil {
			return fmt.Errorf("Rel(): %w", err)
		}
		return fmt.Errorf("a symlink was found in %q at %q; for security reasons and to support windows, git repos containing symlinks are not allowed", remote, relPathToSymlink)
	}); err != nil {
		return err //nolint:wrapcheck
	}

	return nil
}

// Tags looks up the tags in the given remote repo.
//
// "remote" may be any format accepted by git, such as
// https://github.com/abcxyz/abc.git or git@github.com:abcxyz/abc.git .
func Tags(ctx context.Context, remote string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--tags", remote)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git exec of %v failed: %w\nstdout: %s\nstderr: %s", cmd.Args, err, cmd.Stdout, cmd.Stderr)
	}
	lineScanner := bufio.NewScanner(stdout)
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
