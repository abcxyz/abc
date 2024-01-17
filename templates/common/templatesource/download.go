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
	"strings"

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
	Download(ctx context.Context, cwd, destDir string) (*DownloadMetadata, error)
}

type DownloadMetadata struct {
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
	// IsCanonical is true if and only if CanonicalSource and LocationType are
	// non-empty.
	IsCanonical     bool
	CanonicalSource string
	LocationType    string

	// Depending on where the template was taken from, there might be a version
	// string associated with it (e.g. a git tag or a git SHA).
	//
	// HasVersion is true if and only if Version is non-empty.
	HasVersion bool
	Version    string

	// Values for template variables like _git_tag and _git_sha.
	Vars DownloaderVars
}

// Values for template variables like _git_tag and _git_sha.
type DownloaderVars struct {
	GitTag      string
	GitSHA      string
	GitShortSHA string
}

// The parameters to Download, wrapped in a struct because there are so many.
type DownloadParams struct {
	FS common.FS

	// The directory under which to create temp directories.
	TempDirBase string

	// The template source location, e.g. github.com/abcxyz/abc/t/rest_server.
	Source string

	// The value of --git-protocol.
	GitProtocol string

	// The value of os.Getwd().
	CWD string
}

// Downloads the template and returns:
//   - the ParsedSource giving metadata about the template
//   - the name of the temp directory where the template contents were saved.
//
// If error is returned, then the returned directory name may or may not exist,
// and may or may not be empty.
func Download(ctx context.Context, p *DownloadParams) (*DownloadMetadata, string, error) {
	logger := logging.FromContext(ctx).With("logger", "Download")

	templateDir, err := p.FS.MkdirTemp(p.TempDirBase, templateDirNamePart)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create temporary directory to use as template directory: %w", err)
	}
	logger.DebugContext(ctx, "created temporary template directory",
		"path", templateDir)

	downloader, err := ParseSource(ctx, &ParseSourceParams{
		Source:      p.Source,
		GitProtocol: p.GitProtocol,
	})
	if err != nil {
		return nil, templateDir, err
	}
	logger.DebugContext(ctx, "template location parse successful as", "type", reflect.TypeOf(downloader).String())

	dlMeta, err := downloader.Download(ctx, p.CWD, templateDir)
	if err != nil {
		return nil, templateDir, err //nolint:wrapcheck
	}
	logger.DebugContext(ctx, "downloaded source template temporary directory",
		"source", p.Source,
		"destination", templateDir)

	if dlMeta.IsCanonical && strings.ContainsAny(dlMeta.CanonicalSource, `\`+"\n") {
		return nil, "", fmt.Errorf("the template location contains a disallowed character; no backslashes or newlines are allowed in the canonicalized source %q",
			dlMeta.CanonicalSource)
	}

	return dlMeta, templateDir, nil
}
