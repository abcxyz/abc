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

package run

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/tempdir"
)

// RunDiff execs the diff binary. Returns len > 0 if there's a diff. Returns
// unified diff format. Returns empty string if neither file exists (this
// differs from the normal behavior of `/bin/diff -N`, which returns an error if
// both files are absent).
//
// For the purposes of determining the paths in the returned diff, we use the
// relative path of file1 relative to file1RelTo, and the same for file1. So if
// file1 is "/x/y/z.tzt" and file1RelTo is "/x", then the filename label in the
// returned diff will be "y/z.txt".
func RunDiff(ctx context.Context, color bool, file1, file1RelTo, file2, file2RelTo string) (_ string, outErr error) {
	file1Label, err := filepath.Rel(file1RelTo, file1)
	if err != nil {
		return "", fmt.Errorf("failed getting relative path for diff: %w", err)
	}

	file2Label, err := filepath.Rel(file2RelTo, file2)
	if err != nil {
		return "", fmt.Errorf("failed getting relative path for diff: %w", err)
	}

	tempTracker := tempdir.NewDirTracker(&common.RealFS{}, false)
	defer tempTracker.DeferMaybeRemoveAll(ctx, &outErr)
	tempDir, err := tempTracker.MkdirTempTracked("", tempdir.GitDiffSymlinkDirNamePart)
	if err != nil {
		return "", err
	}

	src, err := symlinkIfExistsOrNull(file1, tempDir, "a/"+file1Label)
	if err != nil {
		return "", err
	}
	dst, err := symlinkIfExistsOrNull(file2, tempDir, "b/"+file2Label)
	if err != nil {
		return "", err
	}

	colorParam := "never"
	if color {
		colorParam = "always"
	}

	args := []string{
		"git",
		"diff",
		"--no-index", // Ignore the git repo and use "git diff" as a plain diff tool
		// "-C", tempDir,
		fmt.Sprintf("--color=%s", colorParam), // TODO can separate into two args and avoid Sprintf?
		"--src-prefix", "",                    // we created a/ and b/ dirs in the temp dir, so no need for prefixes.
		"--dst-prefix", "",
		src,
		dst,
	}

	var stdout, stderr bytes.Buffer
	opts := []*Option{
		AllowNonzeroExit(),
		WithCwd(tempDir),
		WithStderr(&stderr),
		WithStdout(&stdout),
	}
	exitCode, err := Run(ctx, opts, args...)
	if err != nil {
		return "", fmt.Errorf("error exec'ing diff: %w", err)
	}
	// TODO check exit codes
	// The man page for diff says it returns code 2 on error.
	if exitCode > 1 {
		// A quirk of the diff command: the -N flag means "treat nonexistent
		// files as empty", but it still fails if both inputs are absent. Our
		// workaround is to detect the case where both files are nonexistent and
		// return empty string.
		// TODO remove
		file1Exists, err := common.Exists(file1)
		if err != nil {
			return "", err //nolint:wrapcheck
		}
		file2Exists, err := common.Exists(file2)
		if err != nil {
			return "", err //nolint:wrapcheck
		}
		if !file1Exists && !file2Exists {
			return "", nil
		}
		return "", fmt.Errorf("error exec'ing diff: %s", stderr.String())
	}
	return stdout.String(), nil
}

const devNull = "/dev/null"

// TODO doc
// Returns linkRelPath if absPath exists. Otherwise returns "/dev/null".
func symlinkIfExistsOrNull(absPath, tempDir, linkRelPath string) (string, error) {
	exists, err := common.Exists(absPath)
	if err != nil {
		return "", err
	}

	linkTarget := devNull
	if exists {
		linkTarget = absPath
	}

	symlinkPath1 := filepath.Join(tempDir, linkRelPath)
	os.MkdirAll(filepath.Dir(symlinkPath1), common.OwnerRWXPerms)
	if err := os.Symlink(linkTarget, symlinkPath1); err != nil {
		return "", err
	}
	return linkRelPath, nil
}

// var (
// 	diffColorOnce    sync.Once
// 	diffColorSupport bool
// 	diffColorErr     error //nolint:errname
// )

// // diffColorSupported returns whether we're running on a machine that supports
// // --color=always. MacOS <= 12 seems not to have this.
// func diffColorSupported(ctx context.Context) (bool, error) {
// 	diffColorOnce.Do(func() {
// 		diffColorSupport, diffColorErr = diffColorCheck(ctx)
// 	})
// 	return diffColorSupport, diffColorErr
// }

// // diffColorCheck tests whether we're running on a machine that supports
// // --color=always. MacOS <= 12 seems not to have this.
// func diffColorCheck(ctx context.Context) (bool, error) {
// 	var stderr bytes.Buffer
// 	opts := []*Option{
// 		WithStderr(&stderr),
// 		AllowNonzeroExit(),
// 	}
// 	exitCode, err := Run(ctx, opts, "diff", "--color=always", "/dev/null", "/dev/null")
// 	if err != nil {
// 		return false, fmt.Errorf("failed determining whether the diff command supports color: %w", err)
// 	}

// 	if exitCode == 2 && strings.Contains(stderr.String(), "unrecognized option `--color=always'") {
// 		return false, nil
// 	}
// 	if exitCode != 0 {
// 		return false, fmt.Errorf("something strange happened when testing diff for color support. Exit code %d, stderr: %q", exitCode, stderr)
// 	}

// 	return true, nil
// }
