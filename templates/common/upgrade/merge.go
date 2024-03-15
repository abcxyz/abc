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

	"golang.org/x/exp/maps"

	"github.com/abcxyz/abc/templates/common"
	manifestutil "github.com/abcxyz/abc/templates/model/manifest"
	manifest "github.com/abcxyz/abc/templates/model/manifest/v1alpha1"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/sets"
)

// ExportToAvoidWarnings avoids compiler warnings complaning about unused
// variables. TODO(upgrade): remove this when no longer necessary.
var ExportToAvoidWarnings = mergeAll

// A mergeAction is an action to take for a given output file. This may involve
// conflicts between upgraded template output files and files that were
// locally customized by the user.
type mergeAction string

const (
	// For the mergeActions named e.g. "editDelete", the first thing is what
	// the user did locally ("edit") and the second thing is want the template
	// wants to do ("delete").

	// Just write the contents of the file from the new template.
	writeNew mergeAction = "writeNew"

	// Just deleteAction the preexisting file in the template output directory.
	// We can't just call this "delete" because that's a Go builtin.
	deleteAction mergeAction = "delete"

	// Take no action, the current contents of the output directory are correct.
	noop mergeAction = "noop"

	// The user manually created a file, and the template also wants to create
	// that file. This is a conflict requiring the user to resolve.
	addAddConflict mergeAction = "addAddConflict"

	// The template originally outputted a file, which the user then edited. Now
	// the template wants to change the file, but we don't want to clobber the
	// user's edits, so we have to ask them to manually resolve the differences.
	editEditConflict mergeAction = "editEditConflict"

	// The template originally outputted a file, which the user then edited. Now
	// the template wants to delete the file, but we don't want to clobber the
	// user's edits, so we have to ask them to manually resolve the differences.
	editDeleteConflict mergeAction = "editDeleteConflict"

	// The template originally outputted a file, which the user then deleted.
	// Now the template wants to change the file. The user might want the newly
	// changed file despite having deleted the previous version of the file, so
	// we'll require them to manually resolve.
	deleteEditConflict mergeAction = "deleteEditConflict"
)

type mergeDecision struct {
	action           mergeAction
	humanExplanation string
}

// decideMergeParams are the inputs to decideMerge(). It contains information
// about a single output path, and whether the pre-existing file or candidate
// replacement file match certain expected hash values.
type decideMergeParams struct {
	// Is this file in the "old" manifest? If so, that means it was output by
	// the template version that was installed prior to the template version
	// that we're upgrading to right now.
	IsInOldManifest bool

	// Is this file in the "new" manifest? If so, that means it is being output
	// by the new, upgraded version of the template.
	IsInNewManifest bool

	// Only used if IsInOldManifest==true. Does the preexisting file on the
	// filesystem (before the upgrade began) match the hash value in the old
	// manifest? If the hash doesn't match, that means the user made some
	// customizations.
	OldFileMatchesOldHash hashResult // TODO make this a bool?

	// Only used if IsInNewManifest==true. Does the new file being output by
	// the new version of the template match the hash value in the old
	// manifest? If the hash matches, that means that the current template
	// outputs identical file contents to the old template.
	NewFileMatchesOldHash hashResult // TODO make this a hashresult?

	// Only used if IsInNewManifest==true and isInOldManifest==false. Does the
	// preexisting file on the filesystem (before the upgrade began) match the
	// hash value in the new manifest? If so, that means that an add/add
	// conflict can be avoided because the both parties added identical file
	// contents.
	OldFileMatchesNewHash hashResult
}

// decideMerge is the core of the algorithm that merges the template output with
// the user's existing files, which in the general case are a mix of files
// output by previous template render/upgrade operations, together with some
// local customizations. We want to integrate changes from the upgraded template
// without clobbering the user's local edits, while requiring as little manual
// conflict resolution as possible.
func decideMerge(o *decideMergeParams) (*mergeDecision, error) {
	switch {
	// Case: this file was not output by the old template version, but is output by this template version.
	case !o.IsInOldManifest && o.IsInNewManifest:
		switch o.OldFileMatchesNewHash {
		case match:
			return &mergeDecision{
				action:           noop,
				humanExplanation: "the new template adds a file which wasn't previously part of the template, and there is a locally-created file having the same contents, so taking no action",
			}, nil
		case mismatch:
			return &mergeDecision{
				action:           addAddConflict,
				humanExplanation: "the new template adds this file, but you already had a file of this name, not from this template",
			}, nil
		case absent:
			return &mergeDecision{
				action:           writeNew,
				humanExplanation: "the new template version added this file, which wasn't in the old template version",
			}, nil
		}

	// Case: this file was output by the old template version, but not by the new template version.
	case o.IsInOldManifest && !o.IsInNewManifest:
		switch o.OldFileMatchesOldHash {
		case match:
			return &mergeDecision{
				action:           deleteAction,
				humanExplanation: "this file was output by the old template but is no longer output by the new template, and there were no local edits",
			}, nil
		case mismatch:
			return &mergeDecision{
				action:           editDeleteConflict,
				humanExplanation: "this file was output by the old template but is no longer output by the new template, and there were local edits",
			}, nil
		case absent:
			return &mergeDecision{
				action:           noop,
				humanExplanation: "this file was deleted locally by the user, and the new template no longer outputs this file, so we can leave it deleted",
			}, nil
		}

	// Case: this file was output by the old template version AND the new template version.
	case o.IsInOldManifest && o.IsInNewManifest:
		if o.NewFileMatchesOldHash == match {
			return &mergeDecision{
				action:           noop,
				humanExplanation: "the new template outputs the same contents as the old template, therefore local edits (if any) can remain without needing resolution",
			}, nil
		}
		switch o.OldFileMatchesOldHash {
		case match:
			return &mergeDecision{
				action:           writeNew,
				humanExplanation: "this file was not modified by the user, and the new template has changes to this file",
			}, nil
		case mismatch:
			return &mergeDecision{
				action:           editEditConflict,
				humanExplanation: "this file was modified by the user, and the template wants to update it, so manual conflict resolution is required",
			}, nil
		case absent:
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

	return nil, fmt.Errorf("this is a bug in abc, please report it at https://github.com/abcxyz/abc/issues/new?template=bug.yaml with this text: IsInOldManifest=%t IsInNewManifest=%t OldFileMatchesOldHash=%q NewFileMatchesOldHash=%q OldFileMatchesNewHash=%q",
		o.IsInOldManifest, o.IsInNewManifest, o.OldFileMatchesOldHash, o.NewFileMatchesOldHash, o.OldFileMatchesNewHash)
}

// mergeAll incorporates the output of the upgraded template version in mergeDir
// with the preexisting template output directory in installedDir. installedDir
// in the general case is a mix of files output by previous template
// render/upgrade operations, together with some local customizations.
func mergeAll(ctx context.Context, fs common.FS, dryRun bool, installedDir, mergeDir string, oldManifest, newManifest *manifest.Manifest) error {
	oldHashes := manifestutil.HashesAsMap(oldManifest.OutputHashes)
	newHashes := manifestutil.HashesAsMap(newManifest.OutputHashes)
	filesUnion := maps.Keys(sets.UnionMapKeys(oldHashes, newHashes))
	sort.Strings(filesUnion)

	for _, relPath := range filesUnion {
		oldHash, isInOldManifest := oldHashes[relPath]
		newHash, isInNewManifest := newHashes[relPath]

		pathOld := filepath.Join(installedDir, relPath)

		// Each file is presumed missing until we see it.
		oldFileMatchesNewHash, oldFileMatchesOldHash, newFileMatchesOldHash := absent, absent, absent

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
			return fmt.Errorf("failed filesystem operation during merge: %w", err)
		}
	}
	return nil
}

const (
	suffixLocallyAdded                  = ".abcmerge_locally_added"
	suffixLocallyEdited                 = ".abcmerge_locally_edited"
	suffixFromNewTemplate               = ".abcmerge_from_new_template"
	suffixFromNewTemplateLocallyDeleted = ".abcmerge_locally_deleted_vs_new_template_version"
	suffixWantToDelete                  = ".abcmerge_template_wants_to_delete"
)

// TODO return a detailed report of what action was taken?
func actuateMergeDecision(ctx context.Context, fs common.FS, dryRun bool, decision *mergeDecision, installedDir, mergeDir, relPath string) error {
	// TODO(upgrade): support backups (eg BackupDirMaker), like in common/render/render.go.

	logger := logging.FromContext(ctx).With("logger", "actuateMergeDecision")
	logger.DebugContext(ctx, "merging one file",
		"dry_run", dryRun,
		"rel_path", relPath,
		"action", decision.action,
		"explanation", decision.humanExplanation)

	dstPath := filepath.Join(installedDir, relPath)
	srcPath := filepath.Join(mergeDir, relPath)

	switch decision.action {
	case writeNew:
		return common.CopyFile(ctx, nil, fs, srcPath, dstPath, dryRun, nil) //nolint:wrapcheck
	case noop:
		return nil
	case deleteAction:
		if dryRun {
			return nil
		}
		if err := os.Remove(dstPath); err != nil {
			return err //nolint:wrapcheck
		}
		return nil
	case deleteEditConflict:
		dstPath += suffixFromNewTemplateLocallyDeleted
		return common.CopyFile(ctx, nil, fs, srcPath, dstPath, dryRun, nil) //nolint:wrapcheck
	case editDeleteConflict:
		if dryRun {
			return nil
		}
		if err := fs.Rename(dstPath, dstPath+suffixWantToDelete); err != nil {
			return err //nolint:wrapcheck
		}
		return nil
	case editEditConflict:
		if dryRun {
			return nil
		}
		if err := fs.Rename(dstPath, dstPath+suffixLocallyEdited); err != nil {
			return err //nolint:wrapcheck
		}
		dstWithSuffix := dstPath + suffixFromNewTemplate
		return common.CopyFile(ctx, nil, fs, srcPath, dstWithSuffix, dryRun, nil) //nolint:wrapcheck
	case addAddConflict:
		if dryRun {
			return nil
		}
		if err := fs.Rename(dstPath, dstPath+suffixLocallyAdded); err != nil {
			return err //nolint:wrapcheck
		}
		return common.CopyFile(ctx, nil, fs, srcPath, dstPath+suffixFromNewTemplate, dryRun, nil) //nolint:wrapcheck
	default:
		return fmt.Errorf("internal error: unrecognized merged action %v", decision.action)
	}
}
