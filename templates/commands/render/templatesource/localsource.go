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
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/pkg/logging"
)

var _ sourceParser = (*localSourceParser)(nil)

// localSourceParser implements sourceParser for reading a template from a local
// directory.
type localSourceParser struct{}

func (l *localSourceParser) sourceParse(ctx context.Context, cwd, src, protocol string) (Downloader, bool, error) {
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
	originalSrc := src
	if !filepath.IsAbs(src) {
		src = filepath.Join(cwd, src)
		logger.DebugContext(ctx, "source path is relative, converting to absolute path",
			"originalSrc", originalSrc,
			"src", src)
	}

	if _, err := os.Stat(src); err != nil {
		var pathErrPtr *fs.PathError

		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrInvalid) || errors.As(err, &pathErrPtr) {
			logger.DebugContext(ctx, "will not treat src as a local path because the path does not exist", "src", src)
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("Stat(): %w", err)
	}

	logger.InfoContext(ctx, "treating src as a local path", "src", src)

	return &localDownloader{
		srcPath: src, // Uses OS-native file separator
	}, true, nil
}

type localDownloader struct {
	srcPath string // uses OS-native file separator
}

func (l *localDownloader) Download(ctx context.Context, outDir string) error {
	logger := logging.FromContext(ctx).With("logger", "localTemplateSource.Download")

	logger.DebugContext(ctx, "copying local template source", "srcPath", l.srcPath, "outDir", outDir)
	return common.CopyRecursive(ctx, nil, &common.CopyParams{ //nolint:wrapcheck
		SrcRoot: l.srcPath,
		DstRoot: outDir,
		RFS:     &common.RealFS{},
	})
}
