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

	// True if this file was included by the "include" action from the
	// destination folder rather than the template folder (somewhat rare).
	isIncludedFromDestination bool
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
		switch {
		case o.oldFileMatchesOldHash == match || o.isIncludedFromDestination:
			return &mergeDecision{
				action:           DeleteAction,
				humanExplanation: "this file was output by the old template but is no longer output by the new template, and there were no local edits",
			}, nil
		case o.oldFileMatchesOldHash == mismatch:
			return &mergeDecision{
				action:           EditDeleteConflict,
				humanExplanation: "this file was output by the old template but is no longer output by the new template, and there were local edits",
			}, nil
		case o.oldFileMatchesOldHash == absent:
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
		switch {
		case o.oldFileMatchesOldHash == match || o.isIncludedFromDestination:
			return &mergeDecision{
				action:           WriteNew,
				humanExplanation: "this file was not modified by the user, and the new template has changes to this file",
			}, nil
		case o.oldFileMatchesOldHash == mismatch:
			return &mergeDecision{
				action:           EditEditConflict,
				humanExplanation: "this file was modified by the user, and the template wants to update it, so manual conflict resolution is required",
			}, nil
		case o.oldFileMatchesOldHash == absent:
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

		paths, err := newMergePaths(p, relPath)
		if err != nil {
			return nil, err
		}

		// Each file is presumed missing until we see it.
		oldFileMatchesNewHash, oldFileMatchesOldHash, newFileMatchesOldHash := absent, absent, absent

		if isInOldManifest {
			var err error
			oldFileMatchesOldHash, err = hashAndCompare(paths.fromOldLocal, oldHash)
			if err != nil {
				return nil, err
			}

			newFileMatchesOldHash, err = hashAndCompare(paths.fromNewTemplate, oldHash)
			if err != nil {
				return nil, err
			}
		}
		if isInNewManifest {
			oldFileMatchesNewHash, err = hashAndCompare(paths.fromOldLocal, newHash)
			if err != nil {
				return nil, err
			}
		}

		hr := &decideMergeParams{
			isInOldManifest:           isInOldManifest,
			isInNewManifest:           isInNewManifest,
			oldFileMatchesOldHash:     oldFileMatchesOldHash,
			newFileMatchesOldHash:     newFileMatchesOldHash,
			oldFileMatchesNewHash:     oldFileMatchesNewHash,
			isIncludedFromDestination: paths.fromReversed != "",
		}

		decision, err := decideMerge(hr)
		if err != nil {
			return nil, err
		}

		action, err := actuateMergeDecision(ctx, p, dryRun, decision, paths)
		if err != nil {
			return nil, fmt.Errorf("failed filesystem operation during merge: %w", err)
		}
		actionsTaken = append(actionsTaken, action)
	}
	return actionsTaken, nil
}

const (
	// These are appended to files that need manual merge conflict resolution.
	SuffixLocallyAdded = ".abcmerge_locally_added"
	SuffixFromNewTemplate               = ".abcmerge_from_new_template"
	SuffixFromNewTemplateLocallyDeleted = ".abcmerge_locally_deleted_vs_new_template_version"
	SuffixWantToDelete                  = ".abcmerge_template_wants_to_delete"

	// This is the beginning of all the above suffixes.
	ConflictSuffixBegins = ".abcmerge_"
)

// oneFileMergePaths contains the paths for all the different versions of a
// file, used by the merge logic that merges the output of the new template with
// the user's existing template output directory.
type oneFileMergePaths struct {
	// relative is the path within the scratch directory of the file we're
	// considering.
	relative string

	// The absolute path of this file in the directory where the template is
	// already installed.
	fromOldLocal string

	// Optional. In the case where this file was included-from-destination, this
	// field will be set. It is the absolute path of the file that was output by
	// the "patch" command.
	fromReversed string

	// The absolute path of this file in the merge directory (the temp directory
	// into which the new template version was rendered).
	fromNewTemplate string
}

// newMergePaths locates the various files that might be needed by the merge
// algorithm.
func newMergePaths(p *commitParams, relPath string) (*oneFileMergePaths, error) {
	fromReversed := filepath.Join(p.reversedPatchDir, relPath)
	if ok, err := common.Exists(fromReversed); err != nil {
		return nil, err //nolint:wrapcheck
	} else if !ok {
		fromReversed = ""
	}

	return &oneFileMergePaths{
		relative:        relPath,
		fromOldLocal:    filepath.Join(p.installedDir, relPath),
		fromNewTemplate: filepath.Join(p.mergeDir, relPath),
		fromReversed:    fromReversed,
	}, nil
}

// actuateMergeDecision actually moves/deletes/copies files to accomplish the
// result decided on by the merge algorithm.
func actuateMergeDecision(ctx context.Context, p *commitParams, dryRun bool, decision *mergeDecision, paths *oneFileMergePaths) (ActionTaken, error) {
	// TODO(upgrade): support backups (eg BackupDirMaker), like in common/render/render.go.

	logger := logging.FromContext(ctx).With("logger", "actuateMergeDecision")
	logger.DebugContext(ctx, "merging one file",
		"dry_run", dryRun,
		"action", decision.action,
		"rel_path", paths.relative,
		"old_path", paths.fromOldLocal,
		"new_path", paths.fromNewTemplate,
		"explanation", decision.humanExplanation)

	installedPath := filepath.Join(p.installedDir, paths.relative)

	actionTaken := ActionTaken{
		Action:      decision.action,
		Explanation: decision.humanExplanation,
		Path:        paths.relative,
	}

	switch decision.action {
	case WriteNew:
		if err := common.CopyFile(ctx, nil, p.fs, paths.fromNewTemplate, installedPath, dryRun, nil); err != nil {
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
		dstPath := installedPath + SuffixFromNewTemplateLocallyDeleted
		if err := common.CopyFile(ctx, nil, p.fs, paths.fromNewTemplate, dstPath, dryRun, nil); err != nil {
			return ActionTaken{}, err //nolint:wrapcheck
		}
		actionTaken.IncomingTemplatePath = paths.relative + SuffixFromNewTemplateLocallyDeleted
		return actionTaken, nil
	case EditDeleteConflict:
		renamedPath := installedPath + SuffixWantToDelete
		if err := common.CopyFile(ctx, nil, p.fs, paths.fromOldLocal, renamedPath, dryRun, nil); err != nil {
			return ActionTaken{}, err //nolint:wrapcheck
		}
		if err := removeOrDryRun(p.fs, dryRun, installedPath); err != nil {
			return ActionTaken{}, err
		}
		actionTaken.OursPath = paths.relative + SuffixWantToDelete
		return actionTaken, nil
	case EditEditConflict:
		incomingPath := installedPath + SuffixFromNewTemplate
		if err := common.CopyFile(ctx, nil, p.fs, paths.fromNewTemplate, incomingPath, dryRun, nil); err != nil {
			return ActionTaken{}, err //nolint:wrapcheck
		}
		actionTaken.IncomingTemplatePath = paths.relative + SuffixFromNewTemplate
		return actionTaken, nil
	case AddAddConflict:
		incomingPath := installedPath + SuffixFromNewTemplate
		if err := common.CopyFile(ctx, nil, p.fs, paths.fromNewTemplate, incomingPath, dryRun, nil); err != nil {
			return ActionTaken{}, err //nolint:wrapcheck
		}
		actionTaken.IncomingTemplatePath = paths.relative + SuffixFromNewTemplate
		return actionTaken, nil
	default:
		return ActionTaken{}, fmt.Errorf("internal error: unrecognized merged action %v", decision.action)
	}
}

func removeOrDryRun(fs common.FS, dryRun bool, path string) error {
	if dryRun {
		return nil
	}
	return fs.Remove(path) //nolint:wrapcheck
}
