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

// Package render implements the template rendering related subcommands.
package templatesource

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/pkg/logging"
)

var _ sourceParser = (*localSourceParser)(nil)

// localSourceParser implements sourceParser for reading a template from a local
// directory.
type localSourceParser struct{}

func (l *localSourceParser) sourceParse(ctx context.Context, cwd string, params *ParseSourceParams) (Downloader, bool, error) {
	logger := logging.FromContext(ctx).With("logger", "localSourceParser.sourceParse")

	// Design decision: we could try to look at src and guess whether it looks
	// like a local directory name, but that's going to have false positives and
	// false negatives (e.g. you have a directory named "github.com/..."). Instead,
	// we'll just check if the given path actually exists, and if so, then treat
	// src as a local directory name.
	//
	// This sourceParser should run after the sourceParser that recognizes remote
	// git repos, so this code won't run if the source looks like a git repo.

	// If the filepath was not absolute, convert it to be relative to the cwd.
	absSource := params.Source
	if !filepath.IsAbs(params.Source) {
		absSource = filepath.Join(cwd, params.Source)
	}

	if _, err := os.Stat(absSource); err != nil {
		if common.IsStatNotExistErr(err) {
			logger.DebugContext(ctx, "will not treat template location as a local path because the path does not exist", "src", absSource)
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("Stat(): %w", err)
	}

	logger.InfoContext(ctx, "treating src as a local path", "src", absSource)

	return &localDownloader{
		srcPath: absSource,
	}, true, nil
}

// localDownloader implements Downloader.
type localDownloader struct {
	// This path uses the OS-native file separator and is an absolute path.
	srcPath string
}

func (l *localDownloader) Download(ctx context.Context, cwd, destDir string) (*DownloadMetadata, error) {
	logger := logging.FromContext(ctx).With("logger", "localTemplateSource.Download")

	logger.DebugContext(ctx, "copying local template source",
		"srcPath", l.srcPath,
		"destDir", destDir)

	canonicalSource, isCanonical, err := canonicalize(ctx, cwd, l.srcPath, destDir)
	if err != nil {
		return nil, err
	}

	if err := common.CopyRecursive(ctx, nil, &common.CopyParams{
		SrcRoot: l.srcPath,
		DstRoot: destDir,
		RFS:     &common.RealFS{},
	}); err != nil {
		return nil, err //nolint:wrapcheck
	}

	dlMeta := &DownloadMetadata{
		IsCanonical:     isCanonical,
		CanonicalSource: canonicalSource,
	}
	return dlMeta, nil
}

// canonicalize determines whether the given combination of src and dest
// directories qualify as a canonical source, and if so, returns the
// canonicalized version of the source. See the docs on DownloadMetadata for an
// explanation of canonical sources.
func canonicalize(ctx context.Context, cwd, src, dest string) (string, bool, error) {
	logger := logging.FromContext(ctx).With("logger", "localDownloader.CanonicalSource")

	absDest := dest
	if !filepath.IsAbs(dest) {
		absDest = filepath.Join(cwd, dest)
	}

	// See the docs on DownloadMetadata for an explanation of why we compare the git
	// workspaces to decide if source is canonical.
	sourceGitWorkspace, templateIsGit, err := gitWorkspace(ctx, src)
	if err != nil {
		return "", false, err
	}
	destGitWorkspace, destIsGit, err := gitWorkspace(ctx, absDest)
	if err != nil {
		return "", false, err
	}
	if !templateIsGit || !destIsGit || sourceGitWorkspace != destGitWorkspace {
		logger.DebugContext(ctx, "local template source is not canonical, template dir and dest dir do not share a git workspace",
			"source", src,
			"dest", absDest,
			"source_git_workspace", sourceGitWorkspace,
			"dest_git_workspace", destGitWorkspace)
		return "", false, nil
	}

	logger.DebugContext(ctx, "local template source is canonical because template dir and dest dir are both in the same git workspace",
		"source", src,
		"dest", absDest,
		"git_workspace", destGitWorkspace)
	out, err := filepath.Rel(absDest, src)
	if err != nil {
		return "", false, fmt.Errorf("filepath.Rel(%q,%q): %w", dest, src, err)
	}
	return filepath.ToSlash(out), true, nil
}

// gitWorkspace looks for the presence of a .git directory in parent directories
// to determine the root directory of the git workspace containing "path".
// Returns false if the given path is not inside a git workspace.
//
// The input path need not actually exist yet. For example, suppose "/a/b" is a
// git workspace, which means that "/a/b/.git" is a directory that exists.
// Calling gitWorkspace("/a/b/c") will return "/a/b" whether or not "c" actually
// exists yet. This supports the case where the user is rendering into a
// directory that doesn't exist yet but will be created by the render operation.
func gitWorkspace(ctx context.Context, path string) (string, bool, error) {
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
