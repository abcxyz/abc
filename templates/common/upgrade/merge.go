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

type mergeAction int

const (
	actionWriteNew mergeAction = iota
	actionUserMustResolveMerge
	actionUserEditedButWantToDelete
	actionUserDeletedButWantToUpdate
	actionDelete
	actionNoop
)

type mergeDecision struct {
	action           mergeAction
	humanExplanation string
}

// decideMergeParams is the input to decideMerge. It provides all the info
// that's needed to decide which mergeAction to take for a given file.
type decideMergeParams struct {
	// Is this file
	IsInOldManifest bool
	IsInNewManifest bool

	// Since the last time the template was upgraded or rendered (before the
	// current upgrade operation), did the user make edit or delete the file?
	// Or does it still match the hash that it had when abc touched it last?
	LocalEdit hashResult

	// Whether the hash recorded in the old/pre-upgrade manifest matches the
	// computed hash of the new template output file. In the cases where there's
	// no such file because the template doesn't output this file anymore, then
	// this should be left as the zero value.
	OldManifestHashMatchedNewFile bool
}

// Truth table for merge decisions:
//
// | isInOld | isInNew | userLocalEdits | oldHashMatchedNewFile | Action                                                                            |
// | --------|---------|----------------|-----------------------|-----------------------------------------------------------------------------------|
// | False   | True    | *              | *                     | Case 1: write new, the new template version added this file |
// | True    | False   | Unchanged      | *                     | Case 2: delete the file, the user never touched it and the new template no longer outputs it |
// | True    | False   | Edited         | *                     | Case 3: user must resolve their edit vs template's deletion |
// | True    | False   | Deleted        | *                     | Case 4: no-op: user deleted locally and template no longer outputs the file |
// | True    | True    | Unchanged      | False                 | Case 5: write new, user has no edits, and the template has a new version |
// | True    | True    | Edited         | False                 | Case 6: user must resolve their edits vs template's new file version |
// | True    | True    | Deleted        | False                 | Case 7: user must resolve their deletion vs template's new file version |
// | True    | True    | *              | True                  | Case 8: no-op: user's local edits can remain, new template didn't change this file |
//
// Maintainers: please keep the above table and the code in sync!
func decideMerge(o *decideMergeParams) (*mergeDecision, error) {
	switch {
	case !o.IsInOldManifest && o.IsInNewManifest:
		return &mergeDecision{
			action:           actionWriteNew,
			humanExplanation: "the new template version added this file, which wasn't in the old template version",
		}, nil

	case o.IsInOldManifest && !o.IsInNewManifest:
		switch o.LocalEdit {
		case HashMatch:
			return &mergeDecision{
				action:           actionDelete,
				humanExplanation: "this file was output by the old template but is no longer output by the new template, and there were no local edits",
			}, nil
		case HashMismatch:
			return &mergeDecision{
				action:           actionUserEditedButWantToDelete,
				humanExplanation: "this file was output by the old template but is no longer output by the new template, and there were local edits",
			}, nil
		case Deleted:
			return &mergeDecision{
				action:           actionNoop,
				humanExplanation: "this file was deleted locally by the user, and the new template no longer outputs this file, so we can leave it deleted",
			}, nil
		}

	case o.IsInOldManifest && o.IsInNewManifest:
		if o.OldManifestHashMatchedNewFile {
			return &mergeDecision{
				action:           actionNoop,
				humanExplanation: "the new template outputs the same contents as the old template, therefore local edits (if any) can remain without needing resolution",
			}, nil
		}
		switch o.LocalEdit {
		case HashMatch:
			return &mergeDecision{
				action:           actionWriteNew,
				humanExplanation: "this file was not modified by the user, and the new template has changes to this file",
			}, nil
		case HashMismatch:
			return &mergeDecision{
				action:           actionUserMustResolveMerge,
				humanExplanation: "this file was modified by the user, and the template wants to update it, so manual conflict resolution is required",
			}, nil
		case Deleted:
			// This is the case where the new template has a version of this
			// file that's different than the previous template version, but the
			// user deleted their copy. It's *probably* safe to just leave the
			// file deleted, but to be safe, let's ask the user to resolve the
			// conflict just in case the new version of the file in the updated
			// template has something really important.
			return &mergeDecision{
				action:           actionUserDeletedButWantToUpdate,
				humanExplanation: "this file was deleted by the user, and the new template has updates, so manual conflict resolution is required",
			}, nil
		}
	}

	return nil, fmt.Errorf("this is a bug in abc, please report it at https://github.com/abcxyz/abc/issues/new?template=bug.yaml: IsInOldManifest=%t IsInNewManifest=%t LocalEdit=%q OldManifestHashMatchedNewFile=%t",
		o.IsInOldManifest, o.IsInNewManifest, o.LocalEdit, o.OldManifestHashMatchedNewFile)
}

func mergeAll(ctx context.Context, fs common.FS, dryRun bool, installedDir, mergeDir string, oldManifest, newManifest *manifest.Manifest) error {
	oldHashes := manifestutil.HashesAsMap(oldManifest.OutputHashes)
	newHashes := manifestutil.HashesAsMap(newManifest.OutputHashes)
	filesUnion := maps.Keys(sets.UnionMapKeys(oldHashes, newHashes))
	sort.Strings(filesUnion)

	for _, relPath := range filesUnion {
		oldHash, isInOldManifest := oldHashes[relPath]
		_, isInNewManifest := newHashes[relPath]

		var localEdit HashResult
		var hashMatchedNewFile bool
		if isInOldManifest {
			var err error
			localEdit, err = hashAndCompare(filepath.Join(installedDir, relPath), oldHash)
			if err != nil {
				return err
			}

			if isInNewManifest {
				var err error
				editType, err := hashAndCompare(filepath.Join(mergeDir, relPath), oldHash)
				if err != nil {
					return err
				}
				if editType == HashMatch {
					hashMatchedNewFile = true
				}
			}
		}

		hr := &decideMergeParams{
			IsInOldManifest:               isInOldManifest,
			IsInNewManifest:               isInNewManifest,
			LocalEdit:                     localEdit,
			OldManifestHashMatchedNewFile: hashMatchedNewFile,
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
	case actionDelete:
		if dryRun {
			return nil
		}
		return os.Remove(dstPath)
	case actionNoop:
		return nil
	case actionUserDeletedButWantToUpdate:
		dstPath += suffixFromNewTemplateLocallyDeleted
		return common.CopyFile(ctx, nil, fs, srcPath, dstPath, dryRun, nil)
	case actionUserEditedButWantToDelete:
		if dryRun {
			return nil
		}
		return fs.Rename(dstPath, dstPath+suffixWantToDelete)
	case actionUserMustResolveMerge:
		if dryRun {
			return nil
		}
		if err := fs.Rename(dstPath, dstPath+suffixLocallyEdited); err != nil {
			return err
		}
		dstWithSuffix := dstPath + suffixFromNewTemplate
		return common.CopyFile(ctx, nil, fs, srcPath, dstWithSuffix, dryRun, nil)

	case actionWriteNew:
		return common.CopyFile(ctx, nil, fs, srcPath, dstPath, dryRun, nil)
	default:
		return fmt.Errorf("internal error: unrecognized merged action %v", decision.action)
	}
}
