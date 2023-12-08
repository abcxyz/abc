// Copyright 2023 The Authors (see AUTHORS file)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package common contains the common utility functions for template commands.

package templatesource

import (
	"context"
	"fmt"
	"reflect"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/pkg/logging"
)

const (
	// These will be used as part of the names of the temporary directories to
	// make them identifiable.
	templateDirNamePart = "template-copy-"
)

// A Downloader is returned by a sourceParser. It offers the ability to
// download a template, and provides some metadata.
type Downloader interface {
	// Download downloads this template into the given directory.
	Download(ctx context.Context, outDir string) error

	// CanonicalSource() returns the canonical source location for this
	// template, if it exists.
	//
	// A "canonical" location is one that's the same for everybody. When
	// installing a template source like
	// "~/my_downloaded_templates/foo_template", that location is not canonical,
	// because not every has that directory downloaded on their machine. On the
	// other hand, a template location like
	// github.com/abcxyz/gcp-org-terraform-template *is* canonical because
	// everyone everywhere can access it by that name.
	//
	// Canonical template locations are preferred because they make automatic
	// template upgrades easier. Given a destination directory that is the
	// output of a template, we can easily upgrade it if we know the canonical
	// location of the template that created it. We just go look for new git
	// tags at the canonical location.
	//
	// A local template directory is not a canonical location except for one
	// special case: when the template source directory and the destination
	// directory are within the same repo. This supports the case where a single
	// git repo contains templates that are rendered into that repo. Since the
	// relative path between the template directory and the destination
	// directory are the same for everyone who clones the repo, that means the
	// relative path counts as a canonical source.
	//
	// CanonicalSource should only be called after Download() has returned
	// successfully. This lets us account for redirects encountered while
	// downloading.
	//
	// "dest" is the value of --dest. cwd is the current working directory.
	CanonicalSource(ctx context.Context, cwd, dest string) (string, bool, error)
}

// Downloads the template and returns:
//   - the ParsedSource giving metadata about the template
//   - the name of the temp directory where the template contents were saved.
//
// If error is returned, then the returned directory name may or may not exist,
// and may or may not be empty.
func Download(ctx context.Context, fs common.FS, tempDirBase, source, gitProtocol string) (Downloader, string, error) {
	logger := logging.FromContext(ctx).With("logger", "downloadTemplate")

	templateDir, err := fs.MkdirTemp(tempDirBase, templateDirNamePart)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create temporary directory to use as template directory: %w", err)
	}
	logger.DebugContext(ctx, "created temporary template directory",
		"path", templateDir)

	downloader, err := ParseSource(ctx, &ParseSourceParams{
		Source:      source,
		GitProtocol: gitProtocol,
	})
	if err != nil {
		return nil, templateDir, err
	}
	logger.DebugContext(ctx, "template location parse successful as", "type", reflect.TypeOf(downloader).String())

	if err := downloader.Download(ctx, templateDir); err != nil {
		return nil, templateDir, err //nolint:wrapcheck
	}
	logger.DebugContext(ctx, "copied source template temporary directory",
		"source", source,
		"destination", templateDir)

	return downloader, templateDir, nil
}
