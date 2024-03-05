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
	"os"
	"path/filepath"
	"strings"

	"github.com/abcxyz/abc/internal/version"
	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/input"
	"github.com/abcxyz/abc/templates/common/render"
	"github.com/abcxyz/abc/templates/common/tempdir"
	"github.com/abcxyz/abc/templates/common/templatesource"
	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/abc/templates/model/decode"
	"github.com/abcxyz/abc/templates/model/header"
	manifest "github.com/abcxyz/abc/templates/model/manifest/v1alpha1"
	"github.com/benbjohnson/clock"
	"gopkg.in/yaml.v3"
)

// // TODO doc
// const newManifestBaseName = "manifest.lock.yaml"

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

	oldManifest, err := loadManifest(ctx, p.FS, p.ManifestPath)
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
		CanonicalLocation:  oldManifest.TemplateLocation.Val,
		LocType:            oldManifest.LocationType.Val,
		GitProtocol:        p.GitProtocol,
		AllowDirtyTestOnly: p.AllowDirtyTestOnly,
	})
	if err != nil {
		return fmt.Errorf("failed creating downloader for manifest location %q of type %q with git protocol %q: %w",
			oldManifest.TemplateLocation.Val, oldManifest.LocationType.Val, p.GitProtocol, err)
	}

	// TODO(upgrade): check the dirhash of the downloaded template, and
	// short-circuit if the installed version is already the newest.

	// TODO(upgrade): handle "include from destination" as a special case
	if err := render.Render(ctx, &render.Params{
		Clock:               p.Clock,
		Cwd:                 p.CWD,
		DebugStepDiffs:      p.DebugStepDiffs,
		DestDir:             installedDir,
		Downloader:          downloader,
		FS:                  p.FS,
		GitProtocol:         p.GitProtocol,
		InputFiles:          p.InputFiles,
		Inputs:              inputsToMap(oldManifest.Inputs),
		KeepTempDirs:        p.KeepTempDirs,
		Manifest:            true,
		OutDir:              mergeDir,
		Prompt:              p.Prompt,
		Prompter:            p.Prompter,
		SkipInputValidation: p.SkipInputValidation,
		SkipPromptTTYCheck:  p.skipPromptTTYCheck,
		SourceForMessages:   oldManifest.TemplateLocation.Val,
		Stdout:              p.Stdout,
		TempDirBase:         p.TempDirBase,
	}); err != nil {
		return fmt.Errorf("failed rendering template as part of upgrade operation: %w", err)
	}

	// TODO(upgrade): much of the upgrade logic is missing here:
	//   - checking file hashes
	//   - checking diffs
	//   - others

	return mergeTentatively(ctx, p.FS, installedDir, mergeDir, p.ManifestPath, oldManifest)
}

func mergeTentatively(ctx context.Context, fs common.FS, installedDir, mergeDir, oldManifestPath string, oldManifest *manifest.Manifest) error {
	// TODO(upgrade): support backups (eg BackupDirMaker), like in common/render/render.go.

	for _, dryRun := range []bool{true, false} {
		if err := commit(ctx, dryRun, fs, installedDir, mergeDir, oldManifestPath, oldManifest); err != nil {
			return err
		}
	}

	return nil
}

func commit(ctx context.Context, dryRun bool, f common.FS, installedDir, mergeDir, oldManifestPath string, oldManifest *manifest.Manifest) error {
	if err := common.CopyRecursive(ctx, nil, &common.CopyParams{
		DryRun:  dryRun,
		DstRoot: installedDir,
		SrcRoot: mergeDir,
		Visitor: func(relPath string, de fs.DirEntry) (common.CopyHint, error) {
			if de.IsDir() && relPath == common.ABCInternalDir {
				// The metadata needs special merging, so skip it for now.
				return common.CopyHint{
					Skip: true,
				}, nil
			}
			return common.CopyHint{
				Overwrite: true,
			}, nil
		},
		FS:     f,
		Hasher: sha256.New,
	}); err != nil {
		return err //nolint:wrapcheck
	}

	abcDir := filepath.Join(mergeDir, common.ABCInternalDir)
	newManifestBaseName, err := findManifest(abcDir)
	if err != nil {
		return err
	}
	newManifest, err := loadManifest(ctx, f, filepath.Join(abcDir, newManifestBaseName))
	if err != nil {
		return err
	}

	mergedManifest := mergeManifest(oldManifest, newManifest)

	buf, err := yaml.Marshal(mergedManifest)
	if err != nil {
		return fmt.Errorf("failed marshaling Manifest when writing: %w", err)
	}
	buf = append(common.DoNotModifyHeader, buf...)

	if dryRun {
		return nil
	}

	return os.WriteFile(oldManifestPath, buf, common.OwnerRWPerms)
}

func mergeManifest(old, newManifest *manifest.Manifest) *manifest.WithHeader {
	// Most fields come from the new manifest, except for the creation time
	// which comes from the old manifest.
	forMarshaling := manifest.ForMarshaling(*newManifest)
	forMarshaling.CreationTime = old.CreationTime

	return &manifest.WithHeader{
		Header: &header.Fields{
			NewStyleAPIVersion: model.String{Val: decode.LatestSupportedAPIVersion(version.IsReleaseBuild())},
			Kind:               model.String{Val: decode.KindManifest},
		},
		Wrapped: &forMarshaling,
	}
}

func loadManifest(ctx context.Context, fs common.FS, path string) (*manifest.Manifest, error) {
	f, err := fs.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open manifest file at %q: %w", path, err)
	}
	defer f.Close()

	manifestI, err := decode.DecodeValidateUpgrade(ctx, f, path, decode.KindManifest)
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

// Finds a manifest file in the given directory by globbing. If there's not
// exactly one match, that's an error. The returned string is just the basename,
// with no directory.
func findManifest(dir string) (string, error) {
	joined := filepath.Join(dir, "manifest*.yaml")
	matches, err := filepath.Glob(joined)
	if err != nil {
		return "", fmt.Errorf("filepath.Glob(%q): %w", joined, err)
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("no manifest was found in %q", dir)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("multiple manifests were found in %q: %s",
			dir, strings.Join(matches, ", "))
	}

	return filepath.Base(matches[0]), nil
}
