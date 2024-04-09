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

package render

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/benbjohnson/clock"
	"gopkg.in/yaml.v3"

	"github.com/abcxyz/abc/internal/version"
	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/dirhash"
	"github.com/abcxyz/abc/templates/common/templatesource"
	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/abc/templates/model/decode"
	"github.com/abcxyz/abc/templates/model/header"
	manifest "github.com/abcxyz/abc/templates/model/manifest/v1alpha1"
)

// writeManifestParams are all the argument to writeManifest, wrapped in a
// struct because there are so many.
type writeManifestParams struct {
	// Fakeable time for testing.
	clock clock.Clock

	// CWD is used to determine whether the template location is canonical or
	// not, in the case where a template is installed from a local directory.
	cwd string

	// destDir is the template render output directory, where the manifest will be
	// written under the .abc directory.
	destDir string

	// Information from the downloader. Includes info about the canonical
	// template location.
	dlMeta *templatesource.DownloadMetadata

	// dryRun creates the manifest in memory but doesn't write it to a file.
	dryRun bool

	// A fakeable filesystem for testing errors.
	fs common.FS

	// The set of values that were used as the template inputs; combined from
	// --input, --input-file, prompts, and defaults.
	inputs map[string]string

	// The SHA256 hash of each file created by the template rendering process
	// in the destination directory.
	outputHashes map[string][]byte

	// The temp directory where the template was downloaded.
	templateDir string
}

// writeManifest creates a manifest struct, marshals it as YAML, and writes it
// to destDir/.abc/ .
func writeManifest(p *writeManifestParams) (rErr error) {
	m, err := buildManifest(p)
	if err != nil {
		return err
	}

	buf, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("failed marshaling Manifest when writing: %w", err)
	}

	baseName := manifestBaseName(p)
	manifestDir := filepath.Join(p.destDir, common.ABCInternalDir)
	manifestPath := filepath.Join(manifestDir, baseName)

	if p.dryRun {
		if _, err := p.fs.Stat(manifestPath); err != nil {
			if common.IsStatNotExistErr(err) {
				// This is good. We don't want to overwrite an existing manifest file,
				// so that fact that it doesn't already exist is good news.
				return nil
			}
			return fmt.Errorf("Stat(): %w", err)
		}
		return fmt.Errorf("dry run failed, the output manifest file %q already exists", manifestPath)
	}

	if err := p.fs.MkdirAll(manifestDir, common.OwnerRWXPerms); err != nil {
		return fmt.Errorf("failed creating %s directory to contain manifest: %w", manifestDir, err)
	}

	// Why O_EXCL? Because we don't want to overwrite an existing file.
	fh, err := p.fs.OpenFile(manifestPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, common.OwnerRWPerms)
	if err != nil {
		return fmt.Errorf("OpenFile(%q): %w", manifestPath, err)
	}
	defer func() {
		rErr = errors.Join(rErr, fh.Close())
	}()

	buf = append(
		[]byte("# Generated by the \"abc templates\" command. Do not modify.\n"),
		buf...)
	if _, err := fh.Write(buf); err != nil {
		return fmt.Errorf("Write(%q): %w", manifestPath, err)
	}

	return nil
}

func manifestBaseName(p *writeManifestParams) string {
	namePart := "nolocation"
	if p.dlMeta.IsCanonical {
		namePart = url.PathEscape(p.dlMeta.CanonicalSource)
	}

	// We include the creation time in the filename to disambiguate between
	// multiple installations of the same template that target the same
	// destination directory.
	timeStr := p.clock.Now().UTC().Format(time.RFC3339Nano)

	return strings.Join(
		[]string{"manifest", namePart, timeStr},
		"_") + ".lock.yaml"
}

// buildManifest constructs the manifest struct for the given parameters.
// canonicalSource is optional, it will be empty in the case where the template
// location is non-canonical (i.e. installing from ~/mytemplate).
func buildManifest(p *writeManifestParams) (*manifest.WithHeader, error) {
	templateDirhash, err := dirhash.HashLatest(p.templateDir)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}

	inputList := make([]*manifest.Input, 0, len(p.inputs))
	for name, val := range p.inputs {
		inputList = append(inputList, &manifest.Input{
			Name:  model.String{Val: name},
			Value: model.String{Val: val},
		})
	}

	outputList := make([]*manifest.OutputFile, 0, len(p.outputHashes))
	for file, hash := range p.outputHashes {
		// For consistency with dirhash, we'll encode our hashes as
		// base64 with an "h1:" prefix indicating SHA256.
		hashStr := "h1:" + base64.StdEncoding.EncodeToString(hash)
		outputList = append(outputList, &manifest.OutputFile{
			File: model.String{Val: file},
			Hash: model.String{Val: hashStr},
		})
	}

	// Alphabetize the lists of inputs and outputs just to be deterministic and
	// civilized.
	sort.Slice(inputList, func(l, r int) bool {
		return inputList[l].Name.Val < inputList[r].Name.Val
	})
	sort.Slice(outputList, func(l, r int) bool {
		return outputList[l].File.Val < outputList[r].File.Val
	})

	now := p.clock.Now().UTC()
	apiVersion := decode.LatestSupportedAPIVersion(version.IsReleaseBuild())

	return &manifest.WithHeader{
		Header: &header.Fields{
			NewStyleAPIVersion: model.String{Val: apiVersion},
			Kind:               model.String{Val: decode.KindManifest},
		},
		Wrapped: &manifest.ForMarshaling{
			TemplateLocation: model.String{Val: p.dlMeta.CanonicalSource}, // may be empty string if location isn't canonical
			LocationType:     model.String{Val: p.dlMeta.LocationType},
			TemplateDirhash:  model.String{Val: templateDirhash},
			TemplateVersion:  model.String{Val: p.dlMeta.Version},
			CreationTime:     now,
			ModificationTime: now,
			Inputs:           inputList,
			OutputFiles:      outputList,
		},
	}, nil
}
