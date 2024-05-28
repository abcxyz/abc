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
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/graph"
	"github.com/abcxyz/abc/templates/common/specutil"
	"github.com/abcxyz/abc/templates/common/templatesource"
)

// TODO(upgrade): remove this, it avoids an "unused" error from the compiler.
var (
	_ = depGraph
	_ = crawlManifests
)

// crawlManifests finds all the template manifest files underneath the given
// file or directory. startFrom can be either a single manifest file or a
// directory to search recursively. Returned paths are relative to startFrom.
func crawlManifests(startFrom string) ([]string, error) {
	var manifests []string
	err := filepath.WalkDir(startFrom, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if common.IsNotExistErr(err) {
				// If the user provides a nonexistent path to upgrade, then
				// we'll just return an empty list of manifests from this
				// function and let a higher level function say "no manifests
				// were found."
				return nil
			}
			return err
		}

		baseName := filepath.Base(path)
		ext := filepath.Ext(path)
		parentDir := filepath.Base(filepath.Dir(path))
		isManifest := strings.HasPrefix(baseName, "manifest") && ext == ".yaml" && parentDir == common.ABCInternalDir
		if !isManifest {
			return nil
		}

		relToStart, err := filepath.Rel(startFrom, path)
		if err != nil {
			return fmt.Errorf("failed determining relative path for manifest: %w", err)
		}
		manifests = append(manifests, relToStart)
		return nil
	})
	if err != nil {
		return nil, err //nolint:wrapcheck
	}

	return manifests, nil
}

// depGraph returns a depdendency graph saying which manifests were output by
// a template that itself was the output of another template. It basically
// specifies the upgrade order for templates.
//
// upgradeLocation is the file or directory provided by the user containing the
// template installations to upgrade.
//
// The returned Graph contains relative manifest paths, such that there is an
// edge from manifest1 to manifest2 if manifest2 should be upgraded before
// manifest1.
//
// This is basically a "self join" on manifests where the *source* spec.yaml
// file from one manifest is joined with the manifest that *created* that
// spec.yaml (if it exists).
func depGraph(ctx context.Context, cwd, upgradeLocation string, manifestsRel []string) (*graph.Graph[string], error) {
	// A mapping of manifestPath to the spec yaml of the template that was being
	// rendered when that manifest was created. This is just the result of
	// reading the "template_location" field from each manifest underneath the
	// location specified by the user.
	//
	// This is also filtered to be only manifests whose location_type is
	// local_git, since that's the only location_type that can cause
	// dependencies between templates involved in an upgradeall operation.
	//
	// Keys are relative paths to the manifestFile, relative to upgradeLocation.
	// Values are absolute paths to a spec file.
	manifestToSourceSpec := map[string]string{}

	// A mapping of spec file to the manifest that mentions that spec file in
	// its list of output files. This is usually empty, except when a template
	// outputs another template. So if we install template T/spec.yaml into
	// directory D, and the output to D includes a spec file D/foo/spec.yaml,
	// then this map will point from D/foo/spec.yaml to T/spec.yaml.
	//
	// You can think of this as an inversion of the output_files list in each
	// manifest, filtered to only spec.yaml files.
	//
	// Keys are an absolute path to a spec.yaml file. Values are a relative path
	// to a manifest file that mentions that spec.yaml file in its output_files
	// list.
	specToOutputManifest := map[string]string{}

	g := graph.NewGraph[string]()

	for _, manifestRel := range manifestsRel {
		// In case this manifest doesn't have any incoming or outgoing
		// dependencies (no graph edges), we manually add it to the graph so
		// it will be included in the topological sort. We can't just rely on
		// implicit creation of nodes when adding edges.
		g.AddNode(manifestRel)

		manifestPath := filepath.Join(upgradeLocation, manifestRel)
		if !filepath.IsAbs(manifestPath) {
			manifestPath = filepath.Join(cwd, manifestPath)
		}
		manifest, err := loadManifest(ctx, &common.RealFS{}, manifestPath)
		if err != nil {
			return nil, err
		}
		destDir := filepath.Dir(filepath.Dir(manifestPath))

		for _, outputFile := range manifest.OutputFiles {
			if strings.HasSuffix(outputFile.File.Val, "/"+specutil.SpecFileName) {
				specPath := filepath.Join(destDir, outputFile.File.Val)
				specToOutputManifest[specPath] = manifestRel
			}
		}
		if manifest.TemplateLocation.Val != "" && templatesource.LocationType(manifest.LocationType.Val) == templatesource.LocalGit {
			// If the manifest is at /foo/bar/.abc/manifest.yaml, then the
			// template was installed the /foo/bar (the parent dir of the dir
			// that contains the manifest).
			installedBySpec := filepath.Join(destDir, manifest.TemplateLocation.Val, specutil.SpecFileName)
			manifestToSourceSpec[manifestRel] = installedBySpec
		}
	}

	// Do the join: simplify from "manifest -> spec -> manifest" to just
	// "manifest -> manifest" by joining on the spec path.
	for _, manifestRel := range manifestsRel {
		sourceSpec, ok := manifestToSourceSpec[manifestRel]
		if !ok {
			continue
		}
		manifestThatCreatedSpec, ok := specToOutputManifest[sourceSpec]
		if !ok {
			continue
		}
		g.AddEdge(manifestRel, manifestThatCreatedSpec)
	}

	return g, nil
}
