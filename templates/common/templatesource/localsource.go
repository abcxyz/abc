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
	"github.com/abcxyz/abc/templates/common/git"
	"github.com/abcxyz/pkg/logging"
)

var _ sourceParser = (*localSourceParser)(nil)

// localSourceParser implements sourceParser for reading a template from a local
// directory.
type localSourceParser struct{}

func (l *localSourceParser) sourceParse(ctx context.Context, params *ParseSourceParams) (Downloader, bool, error) {
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
		absSource = filepath.Join(params.CWD, params.Source)
	}

	fi, err := os.Stat(absSource)
	if err != nil {
		if common.IsNotExistErr(err) {
			logger.DebugContext(ctx, "will not treat template location as a local path because the path does not exist",
				"src", absSource)
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("Stat(): %w", err)
	}

	if !fi.IsDir() {
		logger.WarnContext(ctx, "the template source won't be treated as a local path; that path exists as a regular file but a template location must be a directory",
			"src", absSource)
		return nil, false, nil
	}

	logger.InfoContext(ctx, "treating src as a local path", "src", absSource)

	return &LocalDownloader{
		SrcPath: absSource,
	}, true, nil
}

// LocalDownloader implements Downloader.
type LocalDownloader struct {
	// This path uses the OS-native file separator and is an absolute path.
	SrcPath string
}

// installedDir is only used to check for canonical-ness.
func (l *LocalDownloader) Download(ctx context.Context, cwd, templateDir, destDir string) (*DownloadMetadata, error) {
	logger := logging.FromContext(ctx).With("logger", "localTemplateSource.Download")

	templateDir = common.JoinIfRelative(cwd, templateDir)

	logger.DebugContext(ctx, "copying local template source",
		"src_path", l.SrcPath,
		"template_dir", templateDir)
	if err := common.CopyRecursive(ctx, nil, &common.CopyParams{
		SrcRoot: l.SrcPath,
		DstRoot: templateDir,
		FS:      &common.RealFS{},
	}); err != nil {
		return nil, err //nolint:wrapcheck
	}
	gitVars, err := gitTemplateVars(ctx, l.SrcPath)
	if err != nil {
		return nil, err
	}
	canonicalSource, version, locType, err := canonicalize(ctx, cwd, l.SrcPath, destDir)
	if err != nil {
		return nil, err
	}
	dlMeta := &DownloadMetadata{
		IsCanonical:     canonicalSource != "",
		CanonicalSource: canonicalSource,
		LocationType:    locType,
		HasVersion:      version != "",
		Version:         version,
		Vars:            *gitVars,
	}
	return dlMeta, nil
}

// canonicalize determines whether the given combination of src and dest
// directories qualify as a canonical source, and if so, returns the
// canonicalized version of the source. See the docs on DownloadMetadata for an
// explanation of canonical sources.
func canonicalize(ctx context.Context, cwd, source, destDir string) (canonicalSource, version string, locType LocationType, _ error) {
	logger := logging.FromContext(ctx).With("logger", "canonicalize")

	absSource := common.JoinIfRelative(cwd, source)
	absDestDir := common.JoinIfRelative(cwd, destDir)

	// See the docs on DownloadMetadata for an explanation of why we compare the git
	// workspaces to decide if source is canonical.
	sourceGitWorkspace, sourceIsGit, err := git.Workspace(ctx, absSource)
	if err != nil {
		return "", "", "", err //nolint:wrapcheck
	}
	destGitWorkspace, destIsGit, err := git.Workspace(ctx, absDestDir)
	if err != nil {
		return "", "", "", err //nolint:wrapcheck
	}
	if !sourceIsGit {
		return "", "", LocalNonGit, nil
	}
	if !destIsGit || sourceGitWorkspace != destGitWorkspace {
		logger.DebugContext(ctx, "local template source is not canonical, template dir and dest dir do not share a git workspace",
			"source_dir", absSource,
			"dest_dir", absDestDir,
			"source_git_workspace", sourceGitWorkspace,
			"dest_git_workspace", destGitWorkspace)
		return "", "", LocalGit, nil
	}

	logger.DebugContext(ctx, "local template source is canonical because template dir and dest dir are both in the same git workspace",
		"source_dir", absSource,
		"dest", absDestDir,
		"git_workspace", destGitWorkspace)
	out, err := filepath.Rel(absDestDir, absSource)
	if err != nil {
		return "", "", "", fmt.Errorf("filepath.Rel(%q,%q): %w", absDestDir, absSource, err)
	}

	version, _, err = gitCanonicalVersion(ctx, sourceGitWorkspace)
	if err != nil {
		return "", "", "", err
	}
	return filepath.ToSlash(out), version, LocalGit, nil
}
