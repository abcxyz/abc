// Copyright 2023 The Authors (see AUTHORS file)
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

package render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/render/gotmpl"
	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/pkg/logging"
)

// Called with the contents of a file, and returns the new contents of the file
// to be written.
type walkAndModifyVisitor func([]byte) ([]byte, error)

// For each given path, recursively traverses the directory or file
// scratchDir/relPath, calling the given visitor for each file. If relPath is a
// single file, then the visitor will be called for just that one file. If
// relPath is a directory, then the visitor will be called for all files under
// that directory, recursively. A file will only be visited once per call, even
// if multiple paths include it.
//
// rawPaths is a list of path strings that will be processed (processPaths,
// processGlobs) before walking through.
func walkAndModify(ctx context.Context, sp *stepParams, rawPaths []model.String, v walkAndModifyVisitor) error {
	logger := logging.FromContext(ctx).With("logger", "walkAndModify")
	seen := map[string]struct{}{}

	paths, err := processPaths(rawPaths, sp.scope)
	if err != nil {
		return err
	}
	globbedPaths, err := processGlobs(ctx, paths, sp.scratchDir, sp.features.SkipGlobs)
	if err != nil {
		return err
	}

	for _, absPath := range globbedPaths {
		err := filepath.WalkDir(absPath.Val, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// There was some filesystem error. Give up.
				return absPath.Pos.Errorf("%w", err)
			}
			if d.IsDir() {
				return nil
			}

			if _, ok := seen[path]; ok {
				// File already processed.
				logger.DebugContext(ctx, "skipping file as already seen", "path", path)
				return nil
			}
			oldBuf, err := sp.rp.FS.ReadFile(path)
			if err != nil {
				return absPath.Pos.Errorf("Readfile(): %w", err)
			}

			relToScratchDir, err := filepath.Rel(sp.scratchDir, path)
			if err != nil {
				return absPath.Pos.Errorf("Rel(): %w", err)
			}

			// We must clone oldBuf to guarantee that the callee won't change the
			// underlying bytes. We rely on an unmodified oldBuf below in the call
			// to bytes.Equal.
			newBuf, err := v(bytes.Clone(oldBuf))
			if err != nil {
				return fmt.Errorf("when processing template file %q: %w", relToScratchDir, err)
			}

			seen[path] = struct{}{}

			if bytes.Equal(oldBuf, newBuf) {
				// If file contents are unchanged, there's no need to write.
				return nil
			}

			// The permissions in the following WriteFile call will be ignored
			// because the file already exists.
			if err := sp.rp.FS.WriteFile(path, newBuf, common.OwnerRWXPerms); err != nil {
				return absPath.Pos.Errorf("Writefile(): %w", err)
			}
			logger.DebugContext(ctx, "wrote modification", "path", path)

			return nil
		})
		if err != nil {
			return err //nolint:wrapcheck
		}
	}
	return nil
}

func templateAndCompileRegexes(regexes []model.String, scope *common.Scope) ([]*regexp.Regexp, error) {
	compiled := make([]*regexp.Regexp, len(regexes))
	var merr error
	for i, re := range regexes {
		templated, err := gotmpl.ParseExec(re.Pos, re.Val, scope)
		if err != nil {
			merr = errors.Join(merr, err)
			continue
		}

		compiled[i], err = regexp.Compile(templated)
		if err != nil {
			merr = errors.Join(merr, re.Pos.Errorf("failed compiling regex: %w", err))
			continue
		}
	}

	return compiled, merr
}

// processGlobs processes a list of relative input String paths for simple file globbing.
// Returned paths are converted from relative to absolute.
// Used after processPaths where applicable.
func processGlobs(ctx context.Context, paths []model.String, fromDir string, skipGlobs bool) ([]model.String, error) {
	logger := logging.FromContext(ctx).With("logger", "processGlobs")
	seenPaths := map[string]struct{}{}
	out := make([]model.String, 0, len(paths))

	for _, p := range paths {
		// This supports older api_versions which didn't have glob support.
		if skipGlobs {
			out = append(out, model.String{
				Val: filepath.Join(fromDir, p.Val),
				Pos: p.Pos,
			})
		} else {
			globPaths, err := filepath.Glob(filepath.Join(fromDir, p.Val))
			if err != nil {
				return nil, p.Pos.Errorf("file globbing error: %w", err)
			}
			if len(globPaths) == 0 {
				return nil, p.Pos.Errorf("glob %q did not match any files", p.Val)
			}
			logger.DebugContext(ctx, "glob path expanded:",
				"glob", p.Val,
				"matches", globPaths)
			for _, globPath := range globPaths {
				if _, ok := seenPaths[globPath]; !ok {
					out = append(out, model.String{
						Val: globPath,
						Pos: p.Pos,
					})
					seenPaths[globPath] = struct{}{}
				}
			}
		}
	}

	return out, nil
}

// processPaths processes a list of input String paths for go templating, relative paths,
// and OS-specific slashes.
func processPaths(paths []model.String, scope *common.Scope) ([]model.String, error) {
	out := make([]model.String, 0, len(paths))

	for _, p := range paths {
		tmplOutput, err := gotmpl.ParseExec(p.Pos, p.Val, scope)
		if err != nil {
			return nil, err //nolint:wrapcheck
		}

		relParsed, err := common.SafeRelPath(p.Pos, tmplOutput)
		if err != nil {
			return nil, err //nolint:wrapcheck
		}
		out = append(out, model.String{
			Val: relParsed,
			Pos: p.Pos,
		})
	}

	return out, nil
}
