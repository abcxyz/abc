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

package upgrade

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/input"
	"github.com/abcxyz/abc/templates/common/render"
	"github.com/abcxyz/abc/templates/common/tempdir"
	"github.com/abcxyz/abc/templates/common/templatesource"
	"github.com/abcxyz/abc/templates/model/decode"
	manifest "github.com/abcxyz/abc/templates/model/manifest/v1alpha1"
	"github.com/benbjohnson/clock"
)

// Params contains all the arguments to Upgrade().
type Params struct {
	Clock        clock.Clock
	CWD          string
	FS           common.FS
	GitProtocol  string
	InputFiles   []string
	Inputs       map[string]string
	KeepTempDirs bool
	ManifestPath string // Must be an absolute path.
	Prompt       bool
	Prompter     input.Prompter
	Stdout       io.Writer
	TempDirBase  string
}

// Upgrade TODO
func Upgrade(ctx context.Context, p *Params) (rErr error) {
	if !filepath.IsAbs(p.ManifestPath) {
		return fmt.Errorf("internal error: manifest path must be absolute, but got %q", p.ManifestPath)
	}

	manifest, err := loadManifest(ctx, p)
	if err != nil {
		return err
	}

	// The "merge directory" is yet another temp directory in addition to
	// the template dir and scratch dir. It holds the output of template
	// rendering before we merge it with the real template output directory.
	tempTracker := tempdir.NewDirTracker(p.FS, p.KeepTempDirs)
	defer tempTracker.DeferMaybeRemoveAll(ctx, &rErr)
	mergeDir, err := tempTracker.MkdirTempTracked(p.TempDirBase, tempdir.UpgradeMergeDirNamePart)
	if err != nil {
		return fmt.Errorf("failed creating temp dir: %w", err)
	}

	// For now, manifest files are always located in the .abc directory under
	// the directory where they were installed.
	installedDir := filepath.Join(filepath.Dir(p.ManifestPath), "..")

	downloader, err := templatesource.ForUpgrade(ctx, installedDir, manifest.TemplateLocation.Val, manifest.LocationType.Val, p.GitProtocol)
	if err != nil {
		return fmt.Errorf("failed creating downloader for manifest location %q of type %q with git protocol %q: %w",
			manifest.TemplateLocation.Val, manifest.LocationType.Val, p.GitProtocol, err)
	}

	// TODO: handle "include from destination"
	if err := render.Render(ctx, &render.Params{
		Clock:                 p.Clock,
		Cwd:                   p.CWD,
		DestDir:               mergeDir,
		DestDirUltimate:       installedDir,
		Downloader:            downloader,
		ForceManifestBaseName: filepath.Base(p.ManifestPath),
		FS:                    p.FS,
		Inputs:                inputsToMap(manifest.Inputs),
		InputFiles:            p.InputFiles,
		GitProtocol:           p.GitProtocol,
		Manifest:              true,
		Prompt:                p.Prompt,
		Prompter:              p.Prompter,
		TempDirBase:           p.TempDirBase,

		// TODO: debug flags?
	}); err != nil {
		return fmt.Errorf("TODO: %w", err)
	}

	// TODO: more sophisticated: check hashes, etc
	common.CopyRecursive(ctx, nil, &common.CopyParams{
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
	})

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
