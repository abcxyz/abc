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

// Package upgrade implements template upgrading: taking a directory containing
// a rendered template and updating it with the latest version of the template.
package upgrade

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/benbjohnson/clock"
	"gopkg.in/yaml.v3"

	"github.com/abcxyz/abc/internal/version"
	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/dirhash"
	"github.com/abcxyz/abc/templates/common/input"
	"github.com/abcxyz/abc/templates/common/render"
	"github.com/abcxyz/abc/templates/common/run"
	"github.com/abcxyz/abc/templates/common/tempdir"
	"github.com/abcxyz/abc/templates/common/templatesource"
	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/abc/templates/model/decode"
	"github.com/abcxyz/abc/templates/model/header"
	manifest "github.com/abcxyz/abc/templates/model/manifest/v1alpha1"
	"github.com/abcxyz/pkg/logging"
)

// TODO(upgrade):
//   - add "abort if conflict" feature
//   - validate that manifest paths don't contain traversals
//   - add "check for update and exit" feature
//   - add "upgrade all template installations within directory"
//   - maybe switch file hashes to SHA1, so then we can opportunistically get
//     the common base from the installedDir repo or template repo (which might
//     be the same repo), so then we can do a 3-way merge (`git merge-file`)
//     instead of a 2-way merge.
//   - support --merge-strategy=ours|theirs to resolve conflicts
//   - support --merge-strategy=ai to try to get an LLM to semantically resolve the diff
//   - try an automatic 2-way merge: diff the two files, then apply the diff?
//   - interactive conflict resolution
//   - patch .rej files look the same on all platforms (mac and Linux differ)

const rejectedPatchSuffix = ".patch.rej"

// Params contains all the arguments to Upgrade().
type Params struct {
	Clock clock.Clock

	// CWD is the value of os.Getwd(), or in testing, a temp directory.
	CWD string

	// The value of --debug-scratch-contents.
	DebugScratchContents bool

	// The value of --debug-step-diffs.
	DebugStepDiffs bool

	// FS abstracts filesystem operations for error injection testing.
	FS common.FS

	// The value of --git-protocol.
	GitProtocol string

	// The value of --input-file.
	InputFiles []string

	// The value of --input.
	Inputs map[string]string

	// The value of --keep-temp-dirs.
	KeepTempDirs bool

	// The path to the manifest file where the template was previously installed
	// to that will now be upgraded. This is also overwritten with the new
	// manifest after a successful upgrade.
	ManifestPath string // Must be an absolute path.

	// The value of --prompt.
	Prompt   bool
	Prompter input.Prompter

	// Relative paths where patch reversal has already happened. This is a flag
	// supplied by the user. This will be set if there were merge conflicts
	// during patch reversal that were manually resolved by the user.
	// TODO(upgrade): add this as a CLI flag and plumb it through to here.
	ReversalAlreadyDone []string

	// The value of --skip-input-validation.
	SkipInputValidation bool

	// Used in tests to do prompting for inputs even though the input is not a
	// TTY.
	SkipPromptTTYCheck bool

	// The output stream used to print prompts when Prompt==true.
	Stdout io.Writer

	// Empty string, except in tests. Will be used as the parent of temp dirs.
	TempDirBase string

	// An optional version to update to. In the case of a remote git template,
	// it defaults to "latest", which means "the vX.Y.Z that is largest by
	// semver ordering." In the case of a template on the local filesystem, it's
	// ignored because we only have the one version that's on the filesystem.
	Version string
}

type ResultType string

const (
	// This is the value of Result.Type when the upgrade was vacuously
	// successful because the template is already on the latest version.
	AlreadyUpToDate ResultType = "already_up_to_date"

	// This is the value of Result.Type when there was an upgrade and it was
	// successful, and no user intervention is needed.
	Success ResultType = "success"

	// This is the value of Result.Type when abc tried to apply the reversal
	// patches from the manifest to included-from-destination files, and the
	// patches could not be applied cleanly. User intervention is needed, and
	// the ReversalConflicts field should be used.
	PatchReversalConflict ResultType = "patch_reversal_conflict"

	// The new version of the template conflicted with local modifications and
	// manual resolution is required. The Conflicts field should be used.
	MergeConflict ResultType = "merge_conflict"
)

// Result is returned from Upgrade if there's no error. It may indicate that
// there were merge conflicts requiring manual resolution.
type Result struct {
	// If no upgrade was done because this installation of the template is
	// already on the latest version, then this will be true and all other
	// fields in this struct will have zero values.
	Type ResultType

	// The paths to files where abc tried to apply the reversal
	// patches from the manifest to included-from-destination files, and the
	// patches could not be applied cleanly. Manual resolution is needed.
	//
	// This field should only be used when Type==PatchReversalConflict.
	ReversalConflicts []*ReversalConflict

	// Conflicts is the set of files that require manual intervention by the
	// user to resolve a merge conflict. For example, there may be a file that
	// was edited by the user and that edit conflicts with changes to that same
	// file by the upgraded version of the template.
	//
	// This field should only be used when Type==MergeConflict.
	Conflicts []ActionTaken

	// NonConflicts is the set of template output files that do NOT require any
	// action by the user. Callers are free to ignore this.
	//
	// This is mutually exclusive with "Conflicts". Each file is in at most one
	// of the two lists.
	//
	// This field should only be used when Type is Success or MergeConflict.
	NonConflicts []ActionTaken
}

// ReversalConflict happens when abc tried to apply the reversal
// patches from the manifest to included-from-destination files, and the patches
// could not be applied cleanly.
type ReversalConflict struct {
	// The relative path to the file that needs manual resolution.
	Path string
}

// ActionTaken represents an output of the merge operation. Every file that's
// part of the template output will have an ActionTaken that explains what the
// merge algorithm decided to do for this file (e.g. Noop, EditEditConflict,
// or other), and why.
//
// If there was a merge conflict, then files may have been renamed. See OursPath
// and IncomingTemplatePath.
type ActionTaken struct {
	Action Action

	// Explanation is a human-readable reason why the given Action was chosen
	// for this path.
	Explanation string

	// This is the Path to the single file that this ActionTaken is about. It is
	// always set.
	//
	// This is a relative path, starting from the directory where the template
	// is installed.
	Path string

	// OursPath is only set for certain types of merge conflict. This is the
	// path that the local file was renamed to that needs manual merge
	// resolution.
	//
	// This is a relative path, starting from the directory where the template
	// is installed.
	OursPath string

	// IncomingTemplatePath is only set for certain types of merge conflict.
	// This is the path to the incoming template file that needs merge
	// resolution.
	//
	// This is a relative path, starting from the directory where the template
	// is installed.
	IncomingTemplatePath string
}

// Upgrade takes a directory containing previously rendered template output and
// updates it using the newest version of the template, which is pointed to by
// the manifest file.
//
// Returns true if the upgrade occurred, or false if the upgrade was skipped
// because we're already on the latest version of the template.
func Upgrade(ctx context.Context, p *Params) (_ *Result, rErr error) {
	// For now, manifest files are always located in the .abc directory under
	// the directory where they were installed.
	installedDir := filepath.Join(filepath.Dir(p.ManifestPath), "..")

	if err := detectUnmergedConflicts(installedDir); err != nil {
		return nil, err
	}

	if !filepath.IsAbs(p.ManifestPath) {
		return nil, fmt.Errorf("internal error: manifest path must be absolute, but got %q", p.ManifestPath)
	}

	oldManifest, err := loadManifest(ctx, p.FS, p.ManifestPath)
	if err != nil {
		return nil, err
	}

	if oldManifest.TemplateLocation.Val == "" {
		// TODO(upgrade): add a flag to manually specify template location, to
		// be used if the template location changes or if it was installed from
		// a non-canonical location.
		return nil, fmt.Errorf("this template can't be upgraded because its manifest doesn't contain a template_location. This happens when the template is installed from a non-canonical location, such as a local temp dir, instead of from a permanent location like a remote github repo")
	}

	tempTracker := tempdir.NewDirTracker(p.FS, p.KeepTempDirs)
	defer tempTracker.DeferMaybeRemoveAll(ctx, &rErr)

	downloader, err := templatesource.ForUpgrade(ctx, &templatesource.ForUpgradeParams{
		InstalledDir:      installedDir,
		CanonicalLocation: oldManifest.TemplateLocation.Val,
		LocType:           oldManifest.LocationType.Val,
		GitProtocol:       p.GitProtocol,
		Version:           p.Version,
	})
	if err != nil {
		return nil, fmt.Errorf("failed creating downloader for manifest location %q of type %q with git protocol %q: %w",
			oldManifest.TemplateLocation.Val, oldManifest.LocationType.Val, p.GitProtocol, err)
	}

	templateDir, err := tempTracker.MkdirTempTracked(p.TempDirBase, tempdir.TemplateDirNamePart)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}

	dlMeta, err := downloader.Download(ctx, p.CWD, templateDir, installedDir)
	if err != nil {
		return nil, fmt.Errorf("failed downloading template: %w", err)
	}

	hashMatch, err := dirhash.Verify(oldManifest.TemplateDirhash.Val, templateDir)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	if hashMatch {
		// No need to upgrade. We already have the latest template version.
		return &Result{Type: AlreadyUpToDate}, nil
	}

	// The "merge directory" is yet another temp directory in addition to
	// the template dir and scratch dir. It holds the output of template
	// rendering before we merge it with the real template output directory.
	mergeDir, err := tempTracker.MkdirTempTracked(p.TempDirBase, tempdir.UpgradeMergeDirNamePart)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}

	reversedDir, err := tempTracker.MkdirTempTracked(p.TempDirBase, tempdir.ReversedPatchDirNamePart)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}

	reversalConflicts, err := reversePatches(ctx, &reversePatchesParams{
		fs:           p.FS,
		preReversed:  p.ReversalAlreadyDone,
		installedDir: installedDir,
		reversedDir:  reversedDir,
		oldManifest:  oldManifest,
	})
	if err != nil {
		return nil, err
	}
	if len(reversalConflicts) > 0 {
		return &Result{ReversalConflicts: reversalConflicts}, nil
	}

	renderResult, err := render.RenderAlreadyDownloaded(ctx, dlMeta, templateDir, &render.Params{
		Clock:                   p.Clock,
		Cwd:                     p.CWD,
		DebugStepDiffs:          p.DebugStepDiffs,
		DestDir:                 installedDir,
		Downloader:              downloader,
		FS:                      p.FS,
		GitProtocol:             p.GitProtocol,
		InputFiles:              p.InputFiles,
		IncludeFromDestExtraDir: reversedDir,
		Inputs:                  inputsToMap(oldManifest.Inputs),
		KeepTempDirs:            p.KeepTempDirs,
		Manifest:                true,
		OutDir:                  mergeDir,
		Prompt:                  p.Prompt,
		Prompter:                p.Prompter,
		SkipInputValidation:     p.SkipInputValidation,
		SkipPromptTTYCheck:      p.SkipPromptTTYCheck,
		SourceForMessages:       oldManifest.TemplateLocation.Val,
		Stdout:                  p.Stdout,
		TempDirBase:             p.TempDirBase,
	})
	if err != nil {
		return nil, fmt.Errorf("failed rendering template as part of upgrade operation: %w", err)
	}

	newManifest, err := loadManifest(ctx, p.FS, filepath.Join(mergeDir, renderResult.ManifestPath))
	if err != nil {
		return nil, err
	}

	commitParams := &commitParams{
		fs:               p.FS,
		installedDir:     installedDir,
		mergeDir:         mergeDir,
		oldManifestPath:  p.ManifestPath,
		oldManifest:      oldManifest,
		newManifest:      newManifest,
		reversedPatchDir: reversedDir,
	}
	actionsTaken, err := mergeTentatively(ctx, commitParams)
	if err != nil {
		return nil, err
	}

	conflicts, nonConflicts := partitionConflicts(actionsTaken)

	resultType := MergeConflict
	if len(conflicts) == 0 {
		resultType = Success
	}
	return &Result{
		Type:         resultType,
		Conflicts:    conflicts,
		NonConflicts: nonConflicts,
	}, nil
}

// mergeTentatively does a dry-run commit followed by a real commit.
//
// We do a dry run first to try to detect any problems before we start mutating
// the output directory. We'd like to avoid leaving a mess in the output
// directory if the operation fails.
func mergeTentatively(ctx context.Context, p *commitParams) ([]ActionTaken, error) {
	var actionsTaken []ActionTaken
	for _, dryRun := range []bool{true, false} {
		var err error
		actionsTaken, err = commit(ctx, p, dryRun)
		if err != nil {
			return nil, err
		}
	}

	sort.Slice(actionsTaken, func(i, j int) bool {
		return actionsTaken[i].Path < actionsTaken[j].Path
	})

	return actionsTaken, nil
}

// commitParams contains the inputs to commit().
type commitParams struct {
	fs common.FS

	// The directory into which the old template version was originally
	// rendered.
	installedDir string

	// The temp directory into which the new/upgraded template version was
	// rendered.
	mergeDir string

	// The directory that contains files that have already had their
	// reverse-include-from-destination patches automatically applied.
	reversedPatchDir string

	// The path to the manifest describing the original template installation
	// that we're upgrading from. This is also overwritten with the new
	// manifest.
	oldManifestPath string

	// The parsed contents of the old manifest that we're upgrading from.
	oldManifest *manifest.Manifest

	// The new contents of the manifest, loaded from mergeDir.
	newManifest *manifest.Manifest
}

// commit merges the contents of the merge directory into the installed
// directory and writes the new manifest.
func commit(ctx context.Context, p *commitParams, dryRun bool) ([]ActionTaken, error) {
	actionsTaken, err := mergeAll(ctx, p, dryRun)
	if err != nil {
		return nil, err
	}

	mergedManifest := mergeManifest(p.oldManifest, p.newManifest)

	buf, err := yaml.Marshal(mergedManifest)
	if err != nil {
		return nil, fmt.Errorf("failed marshaling Manifest when writing: %w", err)
	}
	buf = append(common.DoNotModifyHeader, buf...)

	if dryRun {
		return nil, nil
	}

	if err := os.WriteFile(p.oldManifestPath, buf, common.OwnerRWPerms); err != nil {
		return nil, fmt.Errorf("WriteFile(%q): %w", p.oldManifestPath, err)
	}

	return actionsTaken, nil
}

// mergeManifest creates a new manifest for writing to the filesystem. It takes
// mostly the fields from the new manifest, with a little bit from the old
// manifest.
//
// A subtle point here: we always use the file hash values from the new
// manifest, even if the merge algorithm decided not to use the new file. This
// is so the *next* time we upgrade this template, we'll realize that there have
// been local customizations.
func mergeManifest(old, newManifest *manifest.Manifest) *manifest.WithHeader {
	// Most fields come from the new manifest, except for the creation time
	// which comes from the old manifest.
	forMarshaling := manifest.ForMarshaling(*newManifest)
	forMarshaling.CreationTime = old.CreationTime

	return &manifest.WithHeader{
		Header: &header.Fields{
			NewStyleAPIVersion: model.String{Val: decode.LatestSupportedAPIVersion(version.IsReleaseBuild())},
			Kind:               model.String{Val: decode.KindManifest},
		},
		Wrapped: &forMarshaling,
	}
}

// loadManifest reads and unmarshals the manifest at the given path.
func loadManifest(ctx context.Context, fs common.FS, path string) (*manifest.Manifest, error) {
	f, err := fs.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open manifest file at %q: %w", path, err)
	}
	defer f.Close()

	manifestI, err := decode.DecodeValidateUpgrade(ctx, f, path, decode.KindManifest)
	if err != nil {
		return nil, fmt.Errorf("error reading manifest file: %w", err)
	}

	out, ok := manifestI.(*manifest.Manifest)
	if !ok {
		return nil, fmt.Errorf("internal error: manifest file did not decode to *manifest.Manifest")
	}

	return out, nil
}

// inputsToMap takes the list of input values (e.g. "service_account" was "my-service-account")
// and converts to a map for easier lookup.
func inputsToMap(inputs []*manifest.Input) map[string]string {
	out := make(map[string]string, len(inputs))
	for _, input := range inputs {
		out[input.Name.Val] = input.Value.Val
	}
	return out
}

// detectUnmergedConflicts looks for any filename patterns (like *.abcmerge_* or
// *.patch.rej) files in the given directory which would indicate that a
// previous upgrade operation had some unresolved merge conflicts.
func detectUnmergedConflicts(installedDir string) error {
	var unmergedFiles []string
	if err := filepath.WalkDir(installedDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(installedDir, path)
		if err != nil {
			return fmt.Errorf("filepath.Rel: %w", err)
		}
		if relPath == common.ABCInternalDir && d.IsDir() {
			return fs.SkipDir
		}
		if strings.Contains(path, conflictSuffixBegins) || strings.HasSuffix(path, rejectedPatchSuffix) {
			unmergedFiles = append(unmergedFiles, path)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed crawling directory %q looking for unmerged files: %w", installedDir, err)
	}

	if len(unmergedFiles) > 0 {
		return fmt.Errorf("aborting the upgrade because it looks like there's already an upgrade in progress. These files need to be resolved: %v", unmergedFiles)
	}
	return nil
}

// partitionConflicts splits up the incoming list into those actions/files which
// had merge conflicts, and those that didn't.
func partitionConflicts(actionsTaken []ActionTaken) (conflicts, nonConflicts []ActionTaken) {
	for _, a := range actionsTaken {
		if a.Action.IsConflict() {
			conflicts = append(conflicts, a)
			continue
		}
		nonConflicts = append(nonConflicts, a)
	}
	return conflicts, nonConflicts
}

// reversePatchParams contains the parameters to the reversePatches function.
type reversePatchesParams struct {
	fs common.FS

	// The set of files whose patches have already been applied by the user, so
	// they should be skipped by the user. This comes from a command line flag.
	preReversed []string

	// installedDir is the directory where the template output is going.
	installedDir string

	// reversedDir is where reversePatches will store the patches files after
	// the patches from the manifest have been successfully applied.
	reversedDir string

	oldManifest *manifest.Manifest
}

// reversePatches applies each patch stored in the given manifest to its
// associated file from the manifest. These patches were created by the previous
// render operation (before the current upgrade operation). There's one patch
// per file that was included-from-destination, and the patch has the effect of
// undoing the changes that were made by that template version.
func reversePatches(ctx context.Context, p *reversePatchesParams) ([]*ReversalConflict, error) {
	var out []*ReversalConflict
	for _, f := range p.oldManifest.OutputFiles {
		if f.Patch == nil || len(f.Patch.Val) == 0 {
			continue
		}

		outPath := filepath.Join(p.reversedDir, f.File.Val)
		if slices.Contains(p.preReversed, f.File.Val) {
			// In the case where a previous run of abc raised a merge conflict,
			// and the user resolved it, and provided the command-line flag
			// indicating that they already resolved it, then we skip applying
			// the patch from the manifest. Because it has already been applied
			// by the user. So we just copy the already-patched file into the
			// directory for patched files.
			if err := common.Copy(ctx, p.fs, filepath.Join(p.installedDir, f.File.Val), outPath); err != nil {
				return nil, err //nolint:wrapcheck
			}
			continue
		}
		conflict, err := reverseOnePatch(ctx, p.installedDir, outPath, f)
		if err != nil {
			return nil, err
		}
		if conflict != nil {
			out = append(out, conflict)
		}
	}
	return out, nil
}

// reverseOnePatch is a helper for reversePatches that applies a single patch
// to a single file.
func reverseOnePatch(ctx context.Context, installedDir, outPath string, f *manifest.OutputFile) (*ReversalConflict, error) {
	logger := logging.FromContext(ctx).With("logger", "reverseOnePatch")

	if err := os.MkdirAll(filepath.Dir(outPath), common.OwnerRWXPerms); err != nil {
		return nil, fmt.Errorf("failed creating output directory for patch reversal: %w", err)
	}
	installedPath := filepath.Join(installedDir, f.File.Val)
	rejectPath := installedPath + rejectedPatchSuffix

	var stdout, stderr bytes.Buffer
	opts := []*run.Option{
		run.AllowNonzeroExit(),
		run.WithStdinStr(f.Patch.Val),
		run.WithStdout(&stdout),
		run.WithStderr(&stderr),
	}
	// Alternative considered: use the --merge flag to the patch command to put
	// merge conflicts inline in the target file. Why don't we? Two reasons:
	//  - the --merge flag doesn't exist on mac
	//  - the --merge flag is mutually exclusive with the --fuzz flag
	exitCode, err := run.Run(ctx, opts,
		"patch",
		"--unified",    // the diff was originally generated with "diff -u"
		"--strip", "1", // the diff has prefixes like "a/" in "a/file.txt" that need to be removed
		"--output", outPath, // write the patched file to the reversedDir
		"--fuzz", "999", // try super hard to patch even if surrounding context has changed and the patch doesn't apply cleanly. Number was chosen arbitrarily.
		"--reject-file", rejectPath, // Patch hunks that fail to apply will be saved here
		installedPath,
	)
	if err != nil {
		return nil, fmt.Errorf("error running patch command on included-from-destination file %q: %w", f.File.Val, err)
	}
	// TODO(upgrade): support backups, maybe with patch -b
	switch exitCode {
	case 0:
		if stdout.Len() > 0 {
			logger.DebugContext(ctx, "exec of patch to reverse include-from-destination succeeded",
				"stdout", stdout.String())
		}
		return nil, nil
	case 1:
		logger.WarnContext(ctx, "reversal patch didn't apply cleanly",
			"stdout", stdout.String(),
			"stderr", stderr.String(),
			"installed_path", installedPath,
			"reject_path", rejectPath,
		)
		return &ReversalConflict{
			Path: f.File.Val, // TODO(upgrade): should this be absolute instead?
		}, nil

	default:
		return nil, fmt.Errorf("when reversing a patch from the manifest for included-from-destination files, the patch command failed unexpectedly: %s", stderr.String())
	}
}
