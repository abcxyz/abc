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

// TODO doc
type UpgradeAllResult struct {
	// The "most severe" or "most interesting" upgrade result out of all the
	// upgrades attempted. The ascending order of severity is None ->
	// AlreadyUpToDate -> Success -> PatchReversalConflict -> MergeConflict
	//
	// For example, if we ran an upgrade on a directory containing three
	// installed templates, and the results of the upgrades were Success,
	// AlreadyUpToDate, and MergeConflict (in any order), then the overall
	// result would be MergeConflict since that's the must severe.
	Overall ResultType

	// A map from manifestPath to the result of non-erroring upgrade attempts.
	// Merge conflicts are included in this map. There are potentially multiple
	// results per manifest because we loop and upgrade repeatedly.
	//
	// Since we stop the upgrade operation when encountering an error or
	// conflict, it's guaranteed that at most 1 of the entries in this map are
	// a conflict (Type equals MergeConflict or PatchReversalConflict).
	//
	// All slices in this map will have length at least one.
	Results []*Result

	// Err is any error encountered during the upgrade operation. A merge
	// conflict during upgrade is not considered an error.
	//
	// If Err is set, then ErrManifestPath may also be set. No other fields will
	// be set.
	Err             error
	ErrManifestPath string // The optional path to the manifest whose upgrade resulted in error
}

var ErrNoManifests error = fmt.Errorf("found no template manifests to upgrade")

// UpgradeAll crawls the given directory looking for manifest files to upgrade,
// then calls Upgrade() for each one, until no more upgrades are possible. Stops
// if any errors are encountered.
//
// If no manifests could be found, then ErrNoManifests is returned.
func UpgradeAll(ctx context.Context, p *Params) *UpgradeAllResult {
	// logger := logging.FromContext(ctx).With("logger", "UpgradeAll")

	var err error
	p, err = fillDefaults(p) // includes shallow copying of input
	if err != nil {
		return &UpgradeAllResult{Err: err}
	}

	// if !filepath.IsAbs(p.Location) {
	// 	p.Location = filepath.Join(p.CWD, p.Location)
	// }

	// fi, err := p.FS.Stat(p.Location)
	// if err != nil {
	// 	 return &UpgradeAllResult{
	// 		Err: fmt.Errorf("failed to stat() input location %q: %w", p.Location, err)
	// 	 }
	// }

	// // TODO test
	// if fi.IsDir() && len(p.AlreadyResolved) > 0 {
	// 	return &UpgradeAllResult{
	// 		Err: fmt.Errorf("when using the --already-resolved flag, the upgrade command must point to
	// 	}
	// }

	manifests, err := crawlManifests(p.Location)
	if err != nil {
		return &UpgradeAllResult{Err: fmt.Errorf("while crawling manifests: %w", err)}
	}
	if len(manifests) == 0 {
		// Perhaps this isn't strictly an error, but in the case where the user
		// invokes the tool incorrectly and doesn't actually do the work they
		// intended, we want to tell them and not just pretend things are fine.
		return &UpgradeAllResult{Err: ErrNoManifests}
	}

	manifests, err = dependencyOrder(ctx, p.Location, manifests)
	if err != nil {
		return &UpgradeAllResult{Err: fmt.Errorf("while loading manifests to determine dependency order: %w", err)}
	}

	out := &UpgradeAllResult{
		Results: make([]*Result, 0, len(manifests)),
	}

	for _, m := range manifests {
		absManifestPath := filepath.Join(p.Location, m)
		if !filepath.IsAbs(absManifestPath) {
			absManifestPath = filepath.Join(p.CWD, absManifestPath)
		}
		result, err := upgrade(ctx, p, absManifestPath)
		if err != nil {
			path := filepath.Join(p.Location, m)
			if !filepath.IsAbs(path) {
				path = filepath.Join(p.CWD, path)
			}
			out.Err = fmt.Errorf("when upgrading the manifest at %s:\n%w", path, err)
			break
		}
		
		result.ManifestPath = m

		out.Results = append(out.Results, result)
	}

	out.Overall = overallResult(out.Results)

	// u := newUpgrader(p)

	// numWaves := 0
	// for {
	// 	numWaves++
	// 	anyUpgraded := u.upgradeWave(ctx)
	// 	if u.shouldAbort() {
	// 		break
	// 	}

	// 	if !anyUpgraded {
	// 		logger.DebugContext(ctx, "no further manifests to upgrade")
	// 		break
	// 	}
	// }

	out.Overall = overallResult(out.Results)

	return out
}

func overallResult(results []*Result) ResultType {
	var out ResultType
	for _, result := range results {
		if resultTypeLess(out, result.Type) {
			out = result.Type
		}
	}
	return out
}

// // upgrader tracks the state and results of an "upgrade all" operation while
// // it's in progress.
// type upgrader struct {
// 	// All fields are mutable.

// 	p                       *Params // The AlreadyResolved field is set to nil after the first upgrade
// 	out                     *UpgradeAllResult
// 	remoteTemplatesUpgraded map[string]struct{}
// }

// func newUpgrader(p *Params) *upgrader {
// 	return &upgrader{
// 		out: &UpgradeAllResult{
// 			Results: make(map[string][]*Result),
// 		},
// 		p:                       p,
// 		remoteTemplatesUpgraded: make(map[string]struct{}),
// 	}
// }

// // upgradeWave does one iteration of "find all manifests and try to upgrade each
// // one." We have to do multiple waves because templates may reference each
// // other; upgrading one template may change a different template from being
// // "up to date" to "upgradeable."
// func (u *upgrader) upgradeWave(ctx context.Context) (upgraded bool) {
// 	manifests, err := crawlManifests(u.p.Location)
// 	if err != nil {
// 		u.out.Err = err
// 		return false
// 	}
// 	anyUpgraded := false
// 	for _, manifestPath := range manifests {
// 		upgraded := u.upgradeOne(ctx, manifestPath)
// 		if u.shouldAbort() {
// 			return false
// 		}
// 		anyUpgraded = anyUpgraded || upgraded
// 	}

// 	return anyUpgraded
// }

// // upgradeOne is a wrapper around upgrade() (upgrade one template output dir to
// // the latest version). This basically integrates the standalone upgrade()
// // function with the stateful upgrader object.
// func (u *upgrader) upgradeOne(ctx context.Context, manifestPath string) (upgraded bool) {
// 	logger := logging.FromContext(ctx).With("logger", "upgradeOne")

// 	if _, ok := u.remoteTemplatesUpgraded[manifestPath]; ok {
// 		logger.DebugContext(ctx, "skipping already-upgraded manifest",
// 			"manifest", manifestPath)
// 		return false
// 	}

// 	logger.DebugContext(ctx, "attempting upgrade of one manifest",
// 		"manifest", manifestPath)
// 	result, err := upgrade(ctx, u.p, manifestPath)
// 	if err != nil {
// 		u.out.Err = err
// 		u.out.ErrManifestPath = manifestPath
// 		return false
// 	}
// 	if result.DLMeta != nil && result.DLMeta.LocationType.IsRemote() {
// 		// For templates that are upgraded from a remote source, don't
// 		// keep checking for new upgrades every wave. The only reason we
// 		// have waves in the first place is to handle the case where
// 		// one template upgrade, when writing its output, actually
// 		// creates a new version of another template. This concern
// 		// doesn't apply to templates that are sourced remotely, so we
// 		// only have to process them once.
// 		u.remoteTemplatesUpgraded[manifestPath] = struct{}{}
// 	}

// 	// Skip reporting an AlreadyUpToDate result if we just upgraded this
// 	// manifest in a previous wave. This avoids spamming the caller with
// 	// superfluous AlreadyUpToDate results while we iterate repeatedly.
// 	if len(u.out.Results[manifestPath]) == 0 || result.Type != AlreadyUpToDate {
// 		u.out.Results[manifestPath] = append(u.out.Results[manifestPath], result)
// 	}

// 	switch result.Type {
// 	case MergeConflict, PatchReversalConflict:
// 		u.out.ConflictManifestPath = manifestPath
// 		return false
// 	case Success:
// 		// When the user passes the "--already-resolved" flag, that should only
// 		// apply to the first template that we upgrade. That flag means that the
// 		// user resolved a patch reversal conflict specific to that template.
// 		u.p.AlreadyResolved = nil
// 		return true
// 	case AlreadyUpToDate:
// 		return false
// 	default:
// 		u.out.Err = fmt.Errorf("internal error: unhandled switch case for %q", result.Type)
// 		return false
// 	}
// }

// // shouldAbort return true if there was an issue upgrading a template and we
// // should stop without attempting to do any more upgrades to other templates.
// func (u *upgrader) shouldAbort() bool {
// 	return u.out.Err != nil || u.out.ConflictManifestPath != ""
// }

// crawlManifests finds all the template manifest files underneath the given
// file or directory. startFrom can be either a single manifest file or a
// directory to search recursively. Returned paths are relative to startFrom.
func crawlManifests(startFrom string) ([]string, error) {
	var manifests []string

	// absLocation := u.p.Location
	// if !filepath.IsAbs(u.p.Location) {
	// 	var err error
	// 	absLocation, err = filepath.Abs(filepath.Join(u.p.CWD, u.p.Location))
	// 	if err != nil {
	// 		u.out.Err = err
	// 		return false
	// 	}
	// }

	err := filepath.WalkDir(startFrom, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if common.IsNotExistErr(err) {
				// If the user provides a nonexistent path to upgade, then we'll
				// just return an empty list of manifests from this function and
				// let a higher level function say "no manifests were found."
				return nil
			}
			return err
		}

		baseName := filepath.Base(path)
		ext := filepath.Ext(path)
		parentDir := filepath.Base(filepath.Dir(path))
		if !strings.HasPrefix(baseName, "manifest") && ext == ".yaml" && parentDir == common.ABCInternalDir {
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

func dependencyOrder(ctx context.Context, upgradeLocation string, manifestsRel []string) ([]string, error) {
	depGraph, err := depGraph(ctx, upgradeLocation, manifestsRel)
	if err != nil {
		return nil, err
	}
	return graph.TopoSortGeneric(depGraph)
}

// upgradeLocation is the file or directory provided by the user.
// The returned map keys and values are relative paths to manifest files
// relative to upgradeLocation.
// This is basically a "self join" on manifests: TODO
// TODO short-circuit if upgradeLocation is a file (or if manifestsRel is len 1)?
func depGraph(ctx context.Context, upgradeLocation string, manifestsRel []string) (map[string][]string, error) {
	// keys are relative manifest paths, values are spec paths from
	// the template that was installed to create that manifest. Only contains
	// manifests whose location_type is local_git, since that's the only
	// location_type that can cause dependencies between templates involved in
	// an upgradeall operation.
	manifestToSourceSpec := map[string]string{}

	// TODO doc, keys are relative to upgradeLocation, vals are members of manifestsRel
	specToSourceManifest := map[string]string{}
	for _, manifestRel := range manifestsRel {
		manifestPath := filepath.Join(upgradeLocation, manifestRel)
		manifest, err := loadManifest(ctx, &common.RealFS{}, manifestPath)
		if err != nil {
			return nil, err
		}
		startFrom := filepath.Dir(filepath.Dir(manifestPath))
		for _, outputFile := range manifest.OutputFiles {
			if strings.HasSuffix(outputFile.File.Val, "/"+specutil.SpecFileName) {
				specPath := filepath.Join(startFrom, outputFile.File.Val)
				specToSourceManifest[specPath] = manifestRel
			}
		}
		if manifest.TemplateLocation.Val != "" && templatesource.LocationType(manifest.LocationType.Val) == templatesource.LocalGit {
			// If the manifest is at /foo/bar/.abc/manifest.yaml, then the
			// template was installed the /foo/bar (the parent dir of the dir
			// that contains the manifest).
			installedBySpec := filepath.Join(startFrom, manifest.TemplateLocation.Val, specutil.SpecFileName)
			manifestToSourceSpec[manifestRel] = installedBySpec
		}
	}

	manifestToManifestDeps := map[string][]string{}
	for _, manifestRel := range manifestsRel {
		manifestToManifestDeps[manifestRel] = nil
		sourceSpec, ok := manifestToSourceSpec[manifestRel]
		if !ok {
			continue
		}
		manifestThatCreatedSpec, ok := specToSourceManifest[sourceSpec]
		if !ok {
			continue
		}
		// if manifestThatCreatedSpec == manifestRel {
		// 	// TODO panic that something is horribly wrong?
		// }
		manifestToManifestDeps[manifestRel] = append(manifestToManifestDeps[manifestRel], manifestThatCreatedSpec)
	}

	return manifestToManifestDeps, nil
}

// func isUnderDir(dir, path string) {
// }
