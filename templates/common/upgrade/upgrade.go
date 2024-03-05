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

// Package upgrade implements template upgrading: taking a directory containing
// a rendered template and updating it with the latest version of the template.
package upgrade

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"

	"github.com/benbjohnson/clock"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/input"
	"github.com/abcxyz/abc/templates/common/render"
	"github.com/abcxyz/abc/templates/common/tempdir"
	"github.com/abcxyz/abc/templates/common/templatesource"
	"github.com/abcxyz/abc/templates/model/decode"
	manifest "github.com/abcxyz/abc/templates/model/manifest/v1alpha1"
)

// Params contains all the arguments to Upgrade().
type Params struct {
	Clock clock.Clock

	// CWD is the value of os.Getwd(), or in testing, a temp directory.
	CWD string

	// The value of --debug-scratch-contents.
	DebugScratchContents bool

	// The value of --debug-step-diffs.
	DebugStepDiffs bool

	// FS abstracts filesystem operations for error injection testing.
	FS common.FS

	// The value of --git-protocol.
	GitProtocol string

	// The value of --input-file.
	InputFiles []string

	// The value of --input.
	Inputs map[string]string

	// The value of --keep-temp-dirs.
	KeepTempDirs bool

	// The path to the manifest file where the template was previously installed
	// to that will now be upgraded.
	ManifestPath string // Must be an absolute path.

	// The value of --prompt.
	Prompt   bool
	Prompter input.Prompter

	// The value of --skip-input-validation.
	SkipInputValidation bool

	// Used in tests to do prompting for inputs even though the input is not a
	// TTY.
	skipPromptTTYCheck bool

	// The output stream used to print prompts when Prompt==true.
	Stdout io.Writer

	// Empty string, except in tests. Will be used as the parent of temp dirs.
	TempDirBase string

	// For testing, allow template directories in dirty git workspaces to be
	// treated as canonical.
	AllowDirtyTestOnly bool
}

// Upgrade takes a directory containing previously rendered template output and
// updates it using the newest version of the template, which is pointed to by
// the manifest file.
func Upgrade(ctx context.Context, p *Params) (rErr error) {
	if !filepath.IsAbs(p.ManifestPath) {
		return fmt.Errorf("internal error: manifest path must be absolute, but got %q", p.ManifestPath)
	}

	manifest, err := loadManifest(ctx, p)
	if err != nil {
		return err
	}

	tempTracker := tempdir.NewDirTracker(p.FS, p.KeepTempDirs)
	defer tempTracker.DeferMaybeRemoveAll(ctx, &rErr)

	// The "merge directory" is yet another temp directory in addition to
	// the template dir and scratch dir. It holds the output of template
	// rendering before we merge it with the real template output directory.
	mergeDir, err := tempTracker.MkdirTempTracked(p.TempDirBase, tempdir.UpgradeMergeDirNamePart)
	if err != nil {
		return fmt.Errorf("failed creating temp dir: %w", err)
	}

	// For now, manifest files are always located in the .abc directory under
	// the directory where they were installed.
	installedDir := filepath.Join(filepath.Dir(p.ManifestPath), "..")

	downloader, err := templatesource.ForUpgrade(ctx, &templatesource.ForUpgradeParams{
		InstalledDir:       installedDir,
		CanonicalLocation:  manifest.TemplateLocation.Val,
		LocType:            manifest.LocationType.Val,
		GitProtocol:        p.GitProtocol,
		AllowDirtyTestOnly: p.AllowDirtyTestOnly,
	})
	if err != nil {
		return fmt.Errorf("failed creating downloader for manifest location %q of type %q with git protocol %q: %w",
			manifest.TemplateLocation.Val, manifest.LocationType.Val, p.GitProtocol, err)
	}

	// TODO(upgrade): check the dirhash of the downloaded template, and
	// short-circuit if the installed version is already the newest.

	// TODO(upgrade): handle "include from destination" as a special case
	if err := render.Render(ctx, &render.Params{
		Clock:                 p.Clock,
		Cwd:                   p.CWD,
		DebugStepDiffs:        p.DebugStepDiffs,
		DestDir:               installedDir,
		Downloader:            downloader,
		ForceManifestBaseName: filepath.Base(p.ManifestPath),
		FS:                    p.FS,
		GitProtocol:           p.GitProtocol,
		InputFiles:            p.InputFiles,
		Inputs:                inputsToMap(manifest.Inputs),
		KeepTempDirs:          p.KeepTempDirs,
		Manifest:              true,
		OutDir:                mergeDir,
		Prompt:                p.Prompt,
		Prompter:              p.Prompter,
		SkipInputValidation:   p.SkipInputValidation,
		SkipPromptTTYCheck:    p.skipPromptTTYCheck,
		SourceForMessages:     manifest.TemplateLocation.Val,
		Stdout:                p.Stdout,
		TempDirBase:           p.TempDirBase,
	}); err != nil {
		return fmt.Errorf("TODO: %w", err)
	}

	// TODO(upgrade): much of the upgrade logic is missing here:
	//   - checking file hashes
	//   - checking diffs
	//   - others
	if err := common.CopyRecursive(ctx, nil, &common.CopyParams{
		SrcRoot: mergeDir,
		DstRoot: installedDir,
		DryRun:  false,
		Visitor: func(relPath string, de fs.DirEntry) (common.CopyHint, error) {
			return common.CopyHint{
				Overwrite: true,
			}, nil
		},
		FS:     p.FS,
		Hasher: sha256.New,
	}); err != nil {
		return err //nolint:wrapcheck
	}

	return nil
}

func loadManifest(ctx context.Context, p *Params) (*manifest.Manifest, error) {
	f, err := p.FS.Open(p.ManifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open manifest file at %q: %w", p.ManifestPath, err)
	}
	defer f.Close()

	manifestI, err := decode.DecodeValidateUpgrade(ctx, f, p.ManifestPath, decode.KindManifest)
	if err != nil {
		return nil, fmt.Errorf("error reading manifest file: %w", err)
	}

	out, ok := manifestI.(*manifest.Manifest)
	if !ok {
		return nil, fmt.Errorf("internal error: manifest file did not decode to *manifest.Manifest")
	}

	return out, nil
}

func inputsToMap(inputs []*manifest.Input) map[string]string {
	out := make(map[string]string, len(inputs))
	for _, input := range inputs {
		out[input.Name.Val] = input.Value.Val
	}
	return out
}
