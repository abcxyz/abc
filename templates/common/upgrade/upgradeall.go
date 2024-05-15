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

	u := newUpgrader(p)

	numWaves := 0
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

	if len(u.out.Results) == 0 && u.out.Err == nil {
		// Perhaps this isn't strictly an error, but in the case where the user
		// invokes the tool incorrectly and doesn't actually do the work they
		// intended, we want to tell them and not just pretend things are fine.
		u.out.Err = fmt.Errorf("found no template manifests to upgrade")
	}

	return u.out
}

// upgrader tracks the state and results of an "upgrade all" operation while
// it's in progress.
type upgrader struct {
	// All fields are mutable.

	p                       *Params // The AlreadyResolved field is set to nil after the first upgrade
	out                     *UpgradeAllResult
	remoteTemplatesUpgraded map[string]struct{}
}

func newUpgrader(p *Params) *upgrader {
	return &upgrader{
		out: &UpgradeAllResult{
			Results: make(map[string][]*Result),
		},
		p:                       p,
		remoteTemplatesUpgraded: make(map[string]struct{}),
	}
}

// upgradeWave does one iteration of "find all manifests and try to upgrade each
// one." We have to do multiple waves because templates may reference each
// other; upgrading one template may change a different template from being
// "up to date" to "upgradeable."
func (u *upgrader) upgradeWave(ctx context.Context) (upgraded bool) {
	absLocation := u.p.Location
	if !filepath.IsAbs(u.p.Location) {
		var err error
		absLocation, err = filepath.Abs(filepath.Join(u.p.CWD, u.p.Location))
		if err != nil {
			u.out.Err = err
			return false
		}
	}

	manifests, err := crawlManifests(absLocation)
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

// upgradeOne is a wrapper around upgrade() (upgrade one template output dir to
// the latest version). This basically integrates the standalone upgrade()
// function with the stateful upgrader object.
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

	// Skip reporting an AlreadyUpToDate result if we just upgraded this
	// manifest in a previous wave. This avoids spamming the caller with
	// superfluous AlreadyUpToDate results while we iterate repeatedly.
	if len(u.out.Results[manifestPath]) == 0 || result.Type != AlreadyUpToDate {
		u.out.Results[manifestPath] = append(u.out.Results[manifestPath], result)
	}

	switch result.Type {
	case MergeConflict, PatchReversalConflict:
		u.out.ConflictManifestPath = manifestPath
		return false
	case Success:
		// When the user passes the "--already-resolved" flag, that should only
		// apply to the first template that we upgrade. That flag means that the
		// user resolved a patch reversal conflict specific to that template.
		u.p.AlreadyResolved = nil
		return true
	case AlreadyUpToDate:
		return false
	default:
		u.out.Err = fmt.Errorf("internal error: unhandled switch case for %q", result.Type)
		return false
	}
}

// shouldAbort return true if there was an issue upgrading a template and we
// should stop without attempting to do any more upgrades to other templates.
func (u *upgrader) shouldAbort() bool {
	return u.out.Err != nil || u.out.ConflictManifestPath != ""
}

// crawlManifests finds all the template manifest files underneath the given
// file or directory. startFrom can be either a single manifest file or a
// directory to search recursively.
func crawlManifests(startFrom string) ([]string, error) {
	var manifests []string

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
