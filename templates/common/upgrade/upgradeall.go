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
	"github.com/abcxyz/pkg/logging"
)

// type UpgradeAllParams struct {
// 	StartDir string
// }

type UpgradeAllResult struct {
	// A map from manifestPath to the result of non-erroring upgrade attempts.
	// Merge conflicts are included in this map. There are potentially multiple
	// results per manifest because we loop and upgrade repeatedly.
	//
	// Since we stop the upgrade operation when encountering an error or
	// conflict, it's guaranteed that at most 1 of the entries in this map are
	// a conflict (Type equals MergeConflict or PatchReversalConflict).
	//
	// All slices in this map will have length at least one.
	Results map[string][]*Result

	// Will be set when an upgrade didn't complete cleanly due to a merge
	// conflict between the old and new versions of the template. This points
	// to an entry in the Results map, which is guaranteed to have an entry for
	// this key.
	//
	// If this is unset, and Err is unset, then all upgrades completed cleanly
	// or no upgrades were needed.
	ConflictManifestPath string

	// Err is any error encountered during the upgrade operation. A merge
	// conflict during upgrade is not considered an error.
	//
	// If Err is set, then ErrManifestPath may also be set. No other fields will
	// be set.
	Err             error
	ErrManifestPath string // The optional path to the manifest whose upgrade resulted in error
}

// UpgradeAll crawls the given directory looking for manifest files to upgrade,
// then calls Upgrade() for each one, until no more upgrades are possible. Stops
// if any errors are encountered.
func UpgradeAll(ctx context.Context, p *Params) *UpgradeAllResult {
	logger := logging.FromContext(ctx).With("logger", "UpgradeAll")

	if !filepath.IsAbs(p.Location) {
		return &UpgradeAllResult{
			Err: fmt.Errorf("internal error: manifest path must be absolute, but got %q", p.Location),
		}
	}

	// TODO explain
	u := newUpgrader(p)

	numWaves := 0 // TODO explain why multiple waves are needed
	for {
		numWaves++
		anyUpgraded := u.upgradeWave(ctx)
		if u.shouldAbort() {
			break
		}

		if !anyUpgraded {
			logger.DebugContext(ctx, "no further manifests to upgrade")
			break
		}
	}

	if len(u.out.Results) == 0 {
		// Perhaps this isn't strictly an error, but in the case where the user
		// invokes the tool incorrectly and doesn't actually do the work they
		// intended, we want to tell them and not just pretend things are fine.
		u.out.Err = fmt.Errorf("found no template manifests to upgrade")
	}

	return u.out
}

// TODO test calling with Location as file and as directory
// TODO doc
type upgrader struct {
	// These fields are immutable.
	p *Params

	out *UpgradeAllResult
	// These fields are mutated as upgrades progress.

	remoteTemplatesUpgraded map[string]struct{}
}

func newUpgrader(p *Params) *upgrader {
	return &upgrader{
		p: p,
		out: &UpgradeAllResult{
			Results: make(map[string][]*Result),
		},
		remoteTemplatesUpgraded: make(map[string]struct{}),
	}
}

func (u *upgrader) upgradeWave(ctx context.Context) (upgraded bool) {
	// logger := logging.FromContext(ctx).With("logger", "upgradeWave")

	manifests, err := crawlManifests(u.p.Location)
	if err != nil {
		u.out.Err = err
		return false
	}
	anyUpgraded := false
	for _, manifestPath := range manifests {
		upgraded := u.upgradeOne(ctx, manifestPath)
		if u.shouldAbort() {
			return false
		}
		anyUpgraded = anyUpgraded || upgraded
	}

	return anyUpgraded
}

func (u *upgrader) upgradeOne(ctx context.Context, manifestPath string) (upgraded bool) {
	logger := logging.FromContext(ctx).With("logger", "upgradeOne")

	if _, ok := u.remoteTemplatesUpgraded[manifestPath]; ok {
		logger.DebugContext(ctx, "skipping already-upgraded manifest",
			"manifest", manifestPath)
		return false
	}

	logger.DebugContext(ctx, "attempting upgrade of one manifest",
		"manifest", manifestPath)
	result, err := upgrade(ctx, u.p, manifestPath)
	if err != nil {
		u.out.Err = err
		u.out.ErrManifestPath = manifestPath
		return false
	}
	if result.DLMeta != nil && result.DLMeta.LocationType.IsRemote() {
		// For templates that are upgraded from a remote source, don't
		// keep checking for new upgrades every wave. The only reason we
		// have waves in the first place is to handle the case where
		// one template upgrade, when writing its output, actually
		// creates a new version of another template. This concern
		// doesn't apply to templates that are sourced remotely, so we
		// only have to process them once.
		u.remoteTemplatesUpgraded[manifestPath] = struct{}{}
	}

	// Skip reporting an AlreadyUpToDate result if we just upgraded this one.
	// This avoids spamming the caller with superfluous AlreadyUpToDate results
	// while we iterate repeatedly.
	if len(u.out.Results[manifestPath]) == 0 || result.Type != AlreadyUpToDate {
		u.out.Results[manifestPath] = append(u.out.Results[manifestPath], result)
	}

	switch result.Type {
	case MergeConflict, PatchReversalConflict:
		u.out.ConflictManifestPath = manifestPath
		return false
	case Success:
		return true
	case AlreadyUpToDate:
		return false
	default:
		u.out.Err = fmt.Errorf("internal error: unhandled switch case for %q", result.Type)
		return false
	}
}

func (u *upgrader) shouldAbort() bool {
	return u.out.Err != nil || u.out.ConflictManifestPath != ""
}

func crawlManifests(startDir string) ([]string, error) {
	var manifests []string

	err := filepath.WalkDir(startDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		baseName := filepath.Base(path)
		ext := filepath.Ext(path)
		parentDir := filepath.Base(filepath.Dir(path))
		if strings.HasPrefix(baseName, "manifest") && ext == ".yaml" && parentDir == common.ABCInternalDir {
			manifests = append(manifests, path)
		}
		return nil
	})
	if err != nil {
		return nil, err //nolint:wrapcheck
	}

	return manifests, nil
}
