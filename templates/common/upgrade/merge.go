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
	"os"
	"path/filepath"
	"sort"

	"github.com/abcxyz/abc/templates/common"
	manifestutil "github.com/abcxyz/abc/templates/model/manifest"
	manifest "github.com/abcxyz/abc/templates/model/manifest/v1alpha1"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/sets"
	"golang.org/x/exp/maps"
)

type mergeAction string

const (
	// For the mergeActions named e.g. "editDelete", the first thing is what
	// the user did locally ("edit") and the second thing is want the template
	// wants to do ("delete").

	// Just write the contents of the file from the new template.
	writeNew mergeAction = "writeNew"

	// Just delete the preexisting file in the template output directory.
	delete mergeAction = "delete"

	// Take no action, the current contents of the output directory are correct.
	noop mergeAction = "noop"

	// Add dot-suffixes (like .abc_merge_resolve_this) to file names indicating
	// that the user needs to merge manually.
	addAddConflict     mergeAction = "addAddConflict"
	editEditConflict   mergeAction = "editEditConflict"
	editDeleteConflict mergeAction = "editDeleteConflict"
	deleteEditConflict mergeAction = "deleteEditConflict"
)

type mergeDecision struct {
	action           mergeAction
	humanExplanation string
}

// TODO a million tests
// TODO document that if a field is irrelevant it can be left as zero
type decideMergeParams struct {
	IsInOldManifest       bool
	IsInNewManifest       bool
	OldFileMatchesOldHash HashResult // TODO make this a bool?
	NewFileMatchesOldHash HashResult // TODO make this a hashresult?
	OldFileMatchesNewHash HashResult
}

func decideMerge(o *decideMergeParams) (*mergeDecision, error) {
	switch {
	case !o.IsInOldManifest && o.IsInNewManifest:
		switch o.OldFileMatchesNewHash {
		case HashMatch:
			return &mergeDecision{
				action:           noop,
				humanExplanation: "the new template adds a file which wasn't previously part of the template, and there is a locally-created file having the same contents, so taking no action",
			}, nil
		case HashMismatch:
			return &mergeDecision{
				action:           addAddConflict,
				humanExplanation: "the new template adds this file, but you already had a file of this name, not from this template",
			}, nil
		default:
			return &mergeDecision{
				action:           writeNew,
				humanExplanation: "the new template version added this file, which wasn't in the old template version",
			}, nil
		}

	case o.IsInOldManifest && !o.IsInNewManifest:
		switch o.OldFileMatchesOldHash {
		case HashMatch:
			return &mergeDecision{
				action:           delete,
				humanExplanation: "this file was output by the old template but is no longer output by the new template, and there were no local edits",
			}, nil
		case HashMismatch:
			return &mergeDecision{
				action:           editDeleteConflict,
				humanExplanation: "this file was output by the old template but is no longer output by the new template, and there were local edits",
			}, nil
		case Absent:
			return &mergeDecision{
				action:           noop,
				humanExplanation: "this file was deleted locally by the user, and the new template no longer outputs this file, so we can leave it deleted",
			}, nil
		}

	case o.IsInOldManifest && o.IsInNewManifest:
		if o.NewFileMatchesOldHash == HashMatch {
			return &mergeDecision{
				action:           noop,
				humanExplanation: "the new template outputs the same contents as the old template, therefore local edits (if any) can remain without needing resolution",
			}, nil
		}
		switch o.OldFileMatchesOldHash {
		case HashMatch:
			return &mergeDecision{
				action:           writeNew,
				humanExplanation: "this file was not modified by the user, and the new template has changes to this file",
			}, nil
		case HashMismatch:
			return &mergeDecision{
				action:           editEditConflict,
				humanExplanation: "this file was modified by the user, and the template wants to update it, so manual conflict resolution is required",
			}, nil
		case Absent:
			// This is the case where the new template has a version of this
			// file that's different than the previous template version, but the
			// user deleted their copy. It's *probably* safe to just leave the
			// file deleted, but to be safe, let's ask the user to resolve the
			// conflict just in case the new version of the file in the updated
			// template has something really important.
			return &mergeDecision{
				action:           deleteEditConflict,
				humanExplanation: "this file was deleted by the user, and the new template has updates, so manual conflict resolution is required",
			}, nil
		}
	}

	return nil, fmt.Errorf("this is a bug in abc, please report it at https://github.com/abcxyz/abc/issues/new?template=bug.yaml: IsInOldManifest=%t IsInNewManifest=%t OldFileMatchesOldHash=%q NewFileMatchesOldHash=%q OldFileMatchesNewHash=%q",
		o.IsInOldManifest, o.IsInNewManifest, o.OldFileMatchesOldHash, o.NewFileMatchesOldHash, o.OldFileMatchesNewHash)
}

func mergeAll(ctx context.Context, fs common.FS, dryRun bool, installedDir, mergeDir string, oldManifest, newManifest *manifest.Manifest) error {
	oldHashes := manifestutil.HashesAsMap(oldManifest.OutputHashes)
	newHashes := manifestutil.HashesAsMap(newManifest.OutputHashes)
	filesUnion := maps.Keys(sets.UnionMapKeys(oldHashes, newHashes))
	sort.Strings(filesUnion)

	for _, relPath := range filesUnion {
		oldHash, isInOldManifest := oldHashes[relPath]
		newHash, isInNewManifest := newHashes[relPath]

		pathOld := filepath.Join(installedDir, relPath)

		// Files are presumed missing until we see them
		oldFileMatchesNewHash, oldFileMatchesOldHash, newFileMatchesOldHash := Absent, Absent, Absent

		var err error
		if isInOldManifest {
			oldFileMatchesOldHash, err = hashAndCompare(pathOld, oldHash)
			if err != nil {
				return err
			}
			newFileMatchesOldHash, err = hashAndCompare(filepath.Join(mergeDir, relPath), oldHash)
			if err != nil {
				return err
			}
		}
		if isInNewManifest {
			oldFileMatchesNewHash, err = hashAndCompare(pathOld, newHash)
			if err != nil {
				return err
			}
		}

		hr := &decideMergeParams{
			IsInOldManifest:       isInOldManifest,
			IsInNewManifest:       isInNewManifest,
			OldFileMatchesOldHash: oldFileMatchesOldHash,
			NewFileMatchesOldHash: newFileMatchesOldHash,
			OldFileMatchesNewHash: oldFileMatchesNewHash,
		}

		decision, err := decideMerge(hr)
		if err != nil {
			return err
		}

		if err := actuateMergeDecision(ctx, fs, dryRun, decision, installedDir, mergeDir, relPath); err != nil {
			return err
		}
	}
	return nil
}

const (
	suffixLocallyAdded                  = ".abcmerge_locally_added"
	suffixLocallyEdited                 = ".abcmerge_locally_edited"
	suffixFromNewTemplate               = ".abcmerge_from_new_template"
	suffixFromNewTemplateLocallyDeleted = ".abcmerge_conflict_locally_deleted_vs_new_template_version"
	suffixWantToDelete                  = ".abcmerge_template_wants_to_delete"
)

// TODO return a detailed report of what action was taken?
func actuateMergeDecision(ctx context.Context, fs common.FS, dryRun bool, decision *mergeDecision, installedDir, mergeDir, relPath string) error {
	// TODO(upgrade): support backups (eg BackupDirMaker), like in common/render/render.go.

	logger := logging.FromContext(ctx).With("logger", "actuateMergeDecision")
	logger.DebugContext(ctx, "merging one file",
		"dryRun", dryRun,
		"relPath", relPath,
		"action", decision.action,
		"explanation", decision.humanExplanation)

	dstPath := filepath.Join(installedDir, relPath)
	srcPath := filepath.Join(mergeDir, relPath)

	switch decision.action {
	case delete:
		if dryRun {
			return nil
		}
		return os.Remove(dstPath)
	case noop:
		return nil
	case deleteEditConflict:
		dstPath += suffixFromNewTemplateLocallyDeleted
		return common.CopyFile(ctx, nil, fs, srcPath, dstPath, dryRun, nil)
	case editDeleteConflict:
		if dryRun {
			return nil
		}
		return fs.Rename(dstPath, dstPath+suffixWantToDelete)
	case editEditConflict:
		if dryRun {
			return nil
		}
		if err := fs.Rename(dstPath, dstPath+suffixLocallyEdited); err != nil {
			return err
		}
		dstWithSuffix := dstPath + suffixFromNewTemplate
		return common.CopyFile(ctx, nil, fs, srcPath, dstWithSuffix, dryRun, nil)

	case writeNew:
		return common.CopyFile(ctx, nil, fs, srcPath, dstPath, dryRun, nil)
	case addAddConflict:
		if dryRun {
			return nil
		}
		if err := fs.Rename(dstPath, dstPath+suffixLocallyAdded); err != nil {
			return err
		}
		return common.CopyFile(ctx, nil, fs, srcPath, dstPath+suffixFromNewTemplate, dryRun, nil)
	default:
		return fmt.Errorf("internal error: unrecognized merged action %v", decision.action)
	}
}
