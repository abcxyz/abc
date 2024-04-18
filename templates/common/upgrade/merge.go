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
	"path/filepath"
	"sort"

	"golang.org/x/exp/maps"

	"github.com/abcxyz/abc/templates/common"
	manifestutil "github.com/abcxyz/abc/templates/model/manifest"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/sets"
)

// An Action is an action to take for a given output file. This may involve
// conflicts between upgraded template output files and files that were
// locally customized by the user.
//
// For the mergeActions named e.g. "editDelete", the first thing is what
// the user did locally ("edit") and the second thing is want the template
// wants to do ("delete").
type Action string

func (a Action) IsConflict() bool {
	switch a {
	case AddAddConflict, EditEditConflict, EditDeleteConflict, DeleteEditConflict:
		return true
	case WriteNew, DeleteAction, Noop:
		return false
	}
	// This should be unreachable. The golangci "exhaustive" lint check will
	// tell us if any case isn't handled above.
	panic("unreachable")
}

const (
	// Just write the contents of the file from the new template.
	WriteNew Action = "writeNew"

	// Just DeleteAction the preexisting file in the template output directory.
	// We can't just call this "delete" because that's a Go builtin.
	DeleteAction Action = "delete"

	// Take no action, the current contents of the output directory are correct.
	Noop Action = "noop"

	// The user manually created a file, and the template also wants to create
	// that file. This is a conflict requiring the user to resolve.
	AddAddConflict Action = "addAddConflict"

	// The template originally outputted a file, which the user then edited. Now
	// the template wants to change the file, but we don't want to clobber the
	// user's edits, so we have to ask them to manually resolve the differences.
	EditEditConflict Action = "editEditConflict"

	// The template originally outputted a file, which the user then edited. Now
	// the template wants to delete the file, but we don't want to clobber the
	// user's edits, so we have to ask them to manually resolve the differences.
	EditDeleteConflict Action = "editDeleteConflict"

	// The template originally outputted a file, which the user then deleted.
	// Now the template wants to change the file. The user might want the newly
	// changed file despite having deleted the previous version of the file, so
	// we'll require them to manually resolve.
	DeleteEditConflict Action = "deleteEditConflict"
)

// A mergeDecision is the output from the conflict detector. It contains the
// action to take (e.g. "editDeleteConflict") and the reason why that decision
// was made.
type mergeDecision struct {
	action           Action
	humanExplanation string
}

// decideMergeParams are the inputs to decideMerge(). It contains information
// about a single output path, and whether the pre-existing file or candidate
// replacement file match certain expected hash values.
type decideMergeParams struct {
	// Is this file in the "old" manifest? If so, that means it was output by
	// the template version that was installed prior to the template version
	// that we're upgrading to right now.
	isInOldManifest bool

	// Is this file in the "new" manifest? If so, that means it is being output
	// by the new, upgraded version of the template.
	isInNewManifest bool

	// Only used if IsInOldManifest==true. Does the preexisting file on the
	// filesystem (before the upgrade began) match the hash value in the old
	// manifest? If the hash doesn't match, that means the user made some
	// customizations.
	oldFileMatchesOldHash hashResult

	// Only used if IsInNewManifest==true. Does the new file being output by
	// the new version of the template match the hash value in the old
	// manifest? If the hash matches, that means that the current template
	// outputs identical file contents to the old template.
	newFileMatchesOldHash hashResult

	// Only used if IsInNewManifest==true and isInOldManifest==false. Does the
	// preexisting file on the filesystem (before the upgrade began) match the
	// hash value in the new manifest? If so, that means that an add/add
	// conflict can be avoided because the both parties added identical file
	// contents.
	oldFileMatchesNewHash hashResult
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
	case !o.isInOldManifest && o.isInNewManifest:
		switch o.oldFileMatchesNewHash {
		case match:
			return &mergeDecision{
				action:           Noop,
				humanExplanation: "the new template adds a file which wasn't previously part of the template, and there is a locally-created file having the same contents, so taking no action",
			}, nil
		case mismatch:
			return &mergeDecision{
				action:           AddAddConflict,
				humanExplanation: "the new template adds this file, but you already had a file of this name, not from this template",
			}, nil
		case absent:
			return &mergeDecision{
				action:           WriteNew,
				humanExplanation: "the new template version added this file, which wasn't in the old template version",
			}, nil
		}

	// Case: this file was output by the old template version, but not by the new template version.
	case o.isInOldManifest && !o.isInNewManifest:
		switch o.oldFileMatchesOldHash {
		case match:
			return &mergeDecision{
				action:           DeleteAction,
				humanExplanation: "this file was output by the old template but is no longer output by the new template, and there were no local edits",
			}, nil
		case mismatch:
			return &mergeDecision{
				action:           EditDeleteConflict,
				humanExplanation: "this file was output by the old template but is no longer output by the new template, and there were local edits",
			}, nil
		case absent:
			return &mergeDecision{
				action:           Noop,
				humanExplanation: "this file was deleted locally by the user, and the new template no longer outputs this file, so we can leave it deleted",
			}, nil
		}

	// Case: this file was output by the old template version AND the new template version.
	case o.isInOldManifest && o.isInNewManifest:
		if o.newFileMatchesOldHash == match {
			return &mergeDecision{
				action:           Noop,
				humanExplanation: "the new template outputs the same contents as the old template, therefore local edits (if any) can remain without needing resolution",
			}, nil
		}
		switch o.oldFileMatchesOldHash {
		case match:
			return &mergeDecision{
				action:           WriteNew,
				humanExplanation: "this file was not modified by the user, and the new template has changes to this file",
			}, nil
		case mismatch:
			return &mergeDecision{
				action:           EditEditConflict,
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
				action:           DeleteEditConflict,
				humanExplanation: "this file was deleted by the user, and the new template has updates, so manual conflict resolution is required",
			}, nil
		}
	}

	return nil, fmt.Errorf("this is a bug in abc, please report it at https://github.com/abcxyz/abc/issues/new?template=bug.yaml with this text: IsInOldManifest=%t IsInNewManifest=%t OldFileMatchesOldHash=%q NewFileMatchesOldHash=%q OldFileMatchesNewHash=%q",
		o.isInOldManifest, o.isInNewManifest, o.oldFileMatchesOldHash, o.newFileMatchesOldHash, o.oldFileMatchesNewHash)
}

// mergeAll incorporates the output of the upgraded template version in mergeDir
// with the preexisting template output directory in installedDir. installedDir
// in the general case is a mix of files output by previous template
// render/upgrade operations, together with some local customizations.
func mergeAll(ctx context.Context, p *commitParams, dryRun bool) ([]ActionTaken, error) {
	oldHashes := manifestutil.HashesAsMap(p.oldManifest.OutputFiles)
	newHashes := manifestutil.HashesAsMap(p.newManifest.OutputFiles)
	filesUnion := maps.Keys(sets.UnionMapKeys(oldHashes, newHashes))
	sort.Strings(filesUnion)

	actionsTaken := make([]ActionTaken, 0, len(filesUnion))

	for _, relPath := range filesUnion {
		oldHash, isInOldManifest := oldHashes[relPath]
		newHash, isInNewManifest := newHashes[relPath]

		pathOld := filepath.Join(p.installedDir, relPath)

		// Each file is presumed missing until we see it.
		oldFileMatchesNewHash, oldFileMatchesOldHash, newFileMatchesOldHash := absent, absent, absent

		var err error
		if isInOldManifest {
			oldFileMatchesOldHash, err = hashAndCompare(pathOld, oldHash)
			if err != nil {
				return nil, err
			}
			newFileMatchesOldHash, err = hashAndCompare(filepath.Join(p.mergeDir, relPath), oldHash)
			if err != nil {
				return nil, err
			}
		}
		if isInNewManifest {
			oldFileMatchesNewHash, err = hashAndCompare(pathOld, newHash)
			if err != nil {
				return nil, err
			}
		}

		hr := &decideMergeParams{
			isInOldManifest:       isInOldManifest,
			isInNewManifest:       isInNewManifest,
			oldFileMatchesOldHash: oldFileMatchesOldHash,
			newFileMatchesOldHash: newFileMatchesOldHash,
			oldFileMatchesNewHash: oldFileMatchesNewHash,
		}

		decision, err := decideMerge(hr)
		if err != nil {
			return nil, err
		}

		action, err := actuateMergeDecision(ctx, p, dryRun, decision, relPath)
		if err != nil {
			return nil, fmt.Errorf("failed filesystem operation during merge: %w", err)
		}
		actionsTaken = append(actionsTaken, action)
	}
	return actionsTaken, nil
}

const (
	// These are appended to files that need manual merge conflict resolution.
	suffixLocallyAdded                  = ".abcmerge_locally_added"
	suffixLocallyEdited                 = ".abcmerge_locally_edited"
	suffixFromNewTemplate               = ".abcmerge_from_new_template"
	suffixFromNewTemplateLocallyDeleted = ".abcmerge_locally_deleted_vs_new_template_version"
	suffixWantToDelete                  = ".abcmerge_template_wants_to_delete"

	// This is the beginning of all the above suffixes.
	conflictSuffixBegins = ".abcmerge_"
)

func actuateMergeDecision(ctx context.Context, p *commitParams, dryRun bool, decision *mergeDecision, relPath string) (ActionTaken, error) {
	// TODO(upgrade): support backups (eg BackupDirMaker), like in common/render/render.go.

	logger := logging.FromContext(ctx).With("logger", "actuateMergeDecision")
	logger.DebugContext(ctx, "merging one file",
		"dry_run", dryRun,
		"rel_path", relPath,
		"action", decision.action,
		"explanation", decision.humanExplanation)

	installedPath := filepath.Join(p.installedDir, relPath)
	mergePath := filepath.Join(p.mergeDir, relPath)

	actionTaken := ActionTaken{
		Action:      decision.action,
		Explanation: decision.humanExplanation,
		Path:        relPath,
	}

	switch decision.action {
	case WriteNew:
		if err := common.CopyFile(ctx, nil, p.fs, mergePath, installedPath, dryRun, nil); err != nil {
			return ActionTaken{}, err //nolint:wrapcheck
		}
		return actionTaken, nil
	case Noop:
		return actionTaken, nil
	case DeleteAction:
		if err := removeOrDryRun(p.fs, dryRun, installedPath); err != nil {
			return ActionTaken{}, err
		}
		return actionTaken, nil
	case DeleteEditConflict:
		dstPath := installedPath + suffixFromNewTemplateLocallyDeleted
		if err := common.CopyFile(ctx, nil, p.fs, mergePath, dstPath, dryRun, nil); err != nil {
			return ActionTaken{}, err //nolint:wrapcheck
		}
		actionTaken.IncomingTemplatePath = relPath + suffixFromNewTemplateLocallyDeleted
		return actionTaken, nil
	case EditDeleteConflict:
		renamedPath := installedPath + suffixWantToDelete
		if err := renameOrDryRun(p.fs, dryRun, installedPath, renamedPath); err != nil {
			return ActionTaken{}, err
		}
		actionTaken.OursPath = relPath + suffixWantToDelete
		return actionTaken, nil
	case EditEditConflict:
		renamedPath := installedPath + suffixLocallyEdited
		incomingPath := installedPath + suffixFromNewTemplate
		if err := renameOrDryRun(p.fs, dryRun, installedPath, renamedPath); err != nil {
			return ActionTaken{}, err
		}
		if err := common.CopyFile(ctx, nil, p.fs, mergePath, incomingPath, dryRun, nil); err != nil {
			return ActionTaken{}, err //nolint:wrapcheck
		}
		actionTaken.OursPath = relPath + suffixLocallyEdited
		actionTaken.IncomingTemplatePath = relPath + suffixFromNewTemplate
		return actionTaken, nil
	case AddAddConflict:
		renamedPath := installedPath + suffixLocallyAdded
		if err := renameOrDryRun(p.fs, dryRun, installedPath, renamedPath); err != nil {
			return ActionTaken{}, err
		}
		incomingPath := installedPath + suffixFromNewTemplate
		if err := common.CopyFile(ctx, nil, p.fs, mergePath, incomingPath, dryRun, nil); err != nil {
			return ActionTaken{}, err //nolint:wrapcheck
		}
		actionTaken.OursPath = relPath + suffixLocallyAdded
		actionTaken.IncomingTemplatePath = relPath + suffixFromNewTemplate
		return actionTaken, nil
	default:
		return ActionTaken{}, fmt.Errorf("internal error: unrecognized merged action %v", decision.action)
	}
}

func renameOrDryRun(fs common.FS, dryRun bool, from, to string) error {
	if dryRun {
		return nil
	}
	return fs.Rename(from, to) //nolint:wrapcheck
}

func removeOrDryRun(fs common.FS, dryRun bool, path string) error {
	if dryRun {
		return nil
	}
	return fs.Remove(path) //nolint:wrapcheck
}
