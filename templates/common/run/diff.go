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
	"strings"

	"github.com/acarl005/stripansi"

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
	tempDir, err := tempTracker.MkdirTempTracked("", tempdir.GitDiffDirNamePart)
	if err != nil {
		return "", err //nolint:wrapcheck
	}

	srcRelPath := "a/" + file1Label
	dstRelPath := "b/" + file2Label
	if err := copyToTempIfExists(ctx, file1, tempDir, srcRelPath); err != nil {
		return "", err
	}
	if err := copyToTempIfExists(ctx, file2, tempDir, dstRelPath); err != nil {
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
		fmt.Sprintf("--color=%s", colorParam),
		"--src-prefix", "", // we created a/ and b/ dirs in the temp dir, so no need for prefixes.
		"--dst-prefix", "",
		srcRelPath,
		dstRelPath,
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

	switch exitCode {
	case 0:
		return "", nil
	case 1:
		return trimMetadata(stdout.String()), nil
	default:
		return "", fmt.Errorf("error exec'ing diff: %s", stderr.String())
	}
}

// Copies absPath (if it exists) into tempDir at the given relative path. If
// absPath doesn't exist, then an empty file is create at the given relative
// path instead.
func copyToTempIfExists(ctx context.Context, absPath, tempDir, linkRelPath string) error {
	srcExists, err := common.Exists(absPath)
	if err != nil {
		return err //nolint:wrapcheck
	}

	dest := filepath.Join(tempDir, linkRelPath)
	if err := os.MkdirAll(filepath.Dir(dest), common.OwnerRWXPerms); err != nil {
		return err //nolint:wrapcheck
	}

	if !srcExists {
		return os.WriteFile(dest, nil, common.OwnerRWPerms) //nolint:wrapcheck
	}

	if err := common.Copy(ctx, &common.RealFS{}, absPath, dest); err != nil {
		return err //nolint:wrapcheck
	}
	return nil
}

// The git diff command includes extra metadata header lines that we don't want,
// like this:
//
//	diff --git a/file1.txt b/file2.txt
//	index 84d55c5..e69de29 100644
//
// ... so we remove them, but only if they are present at the beginning of the
// file.
//
// These metadata lines aren't presented in regular non-diff git output, and
// they're just clutter. Also, the "index" line leaks metadata that we don't
// want to leak.
func trimMetadata(diffOutput string) string {
	const linesToCheck = 2
	splits := strings.SplitN(diffOutput, "\n", linesToCheck+1)
	out := make([]string, 0, 1)
	linesToIgnorePrefixes := []string{
		"diff --git",
		"index ",
	}
	for _, split := range splits {
		anyMatched := false
		for _, p := range linesToIgnorePrefixes {
			if strings.HasPrefix(stripansi.Strip(split), p) {
				anyMatched = true
				break
			}
		}
		if anyMatched {
			continue
		}
		out = append(out, split)
	}

	return strings.Join(out, "\n")
}
