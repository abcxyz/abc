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
	"context"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/model"
	spec "github.com/abcxyz/abc/templates/model/spec/v1beta6"
	"github.com/abcxyz/pkg/logging"
)

// defaultIgnorePatterns to be used if ignore is not provided.
var defaultIgnorePatterns = []model.String{
	{Val: ".DS_Store"},
	{Val: ".bin"},
	{Val: ".ssh"},
}

func actionInclude(ctx context.Context, inc *spec.Include, sp *stepParams) error {
	for _, path := range inc.Paths {
		if err := includePath(ctx, path, sp); err != nil {
			return err
		}
	}
	return nil
}

func copyToDst(ctx context.Context, sp *stepParams, skipPaths []model.String, pos *model.ConfigPos, absDst, absSrc, relSrc, fromVal, fromDir string) error {
	logger := logging.FromContext(ctx).With("logger", "includePath")

	if exists, err := common.ExistsFS(sp.rp.FS, absSrc); err != nil {
		return err //nolint:wrapcheck
	} else if !exists {
		return pos.Errorf("include path doesn't exist: %q", absSrc)
	}

	params := &common.CopyParams{
		DryRun:  false, // This copy targets a temp directory, so always do it.
		DstRoot: absDst,
		FS:      sp.rp.FS,
		SrcRoot: absSrc,
		Visitor: func(relToSrcRoot string, de fs.DirEntry) (common.CopyHint, error) {
			for _, skipPath := range skipPaths {
				matched := (skipPath.Val == filepath.Join(relSrc, relToSrcRoot))
				if !sp.features.SkipGlobs {
					var err error
					path := filepath.Join(relSrc, relToSrcRoot)
					matched, err = filepath.Match(skipPath.Val, path)
					if err != nil {
						return common.CopyHint{}, pos.Errorf("error matching path (%q) with skip pattern (%q): %w", path, skipPath.Val, err)
					}
				}

				if matched {
					return common.CopyHint{Skip: true}, nil
				}
			}

			abs := filepath.Join(absSrc, relToSrcRoot)
			relToFromDir, err := filepath.Rel(fromDir, abs)
			if err != nil {
				return common.CopyHint{}, fmt.Errorf("filepath.Rel(%s,%s)=%w", fromDir, absSrc, err)
			}
			matched, err := checkIgnore(sp.ignorePatterns, relToFromDir)
			if err != nil {
				return common.CopyHint{},
					fmt.Errorf("failed to match path(%q) with ignore patterns: %w", relToFromDir, err)
			}
			if matched {
				logger.DebugContext(ctx, "path ignored", "path", relToFromDir)
				return common.CopyHint{
					Skip: true,
				}, nil
			}
			if !de.IsDir() {
				if fromVal == "destination" {
					sp.includedFromDest[relToFromDir] = fromDir
				} else {
					// Edge case: suppose this sequence of events occurs:
					//  1. A given path is `include`d with from==destination
					//     (indicating an intent to modify a preexisting file in
					//     place).
					//  2. The same path is `include`d normally (not with
					//     from==destination). This represents a completely
					//     separate file that comes from the template dir and
					//     not from the dest dir.
					//  3. The second include should completely replace the
					//     first. In the metadata that tracks whether the file
					//     was included from destination, we should delete the
					//     record of this path being included from destination.
					delete(sp.includedFromDest, relToFromDir)
				}
			}

			return common.CopyHint{
				// Allow later includes to replace earlier includes in the
				// scratch directory. This doesn't affect whether files in
				// the final *destination* directory will be overwritten;
				// that comes later.
				Overwrite: true,
			}, nil
		},
	}
	if err := common.CopyRecursive(ctx, pos, params); err != nil {
		return pos.Errorf("copying failed: %w", err)
	}
	return nil
}

func isGlob(matchedPaths []model.String, originalPath, matchedPath string) bool {
	// originalPath pattern matched more than one path, pattern is a glob
	if len(matchedPaths) != 1 {
		return true
	}
	// originalPath pattern changed (expanded to matchedPath), pattern is a glob
	if originalPath != matchedPath {
		return true
	}
	return false
}

func includePath(ctx context.Context, inc *spec.IncludePath, sp *stepParams) error {
	// By default, we copy from the template directory.
	fromDirs := []string{sp.templateDir}
	if inc.From.Val == "destination" {
		// We also support including files from the destination directory, so we
		// can modify files that already exist in the destination.
		fromDirs = []string{sp.rp.DestDir}
		// TODO(upgrade): add this field in an upcoming PR
		// if sp.rp.IncludeFromDestExtraDir != "" {
		// For complicated reasons related to upgrading, we sometimes add
		// another include source directory that contains files after having
		// had their include-from-destination patch reversed as part of the
		// upgrade process.
		//	fromDirs = append(fromDirs, sp.rp.IncludeFromDestExtraDir)
		// }
	}

	anyMatches := false
	for _, fromDir := range fromDirs {
		matched, err := includeFromOneDir(ctx, inc, sp, fromDir)
		if err != nil {
			return err
		}
		anyMatches = anyMatches || matched
	}
	if !anyMatches {
		var pathStrings []string
		for _, p := range inc.Paths {
			pathStrings = append(pathStrings, p.Val)
		}
		return inc.Pos.Errorf("include paths did not match any files: %v", pathStrings)
	}
	return nil
}

// includeFromOneDir does the include action for a single source directory. This
// is needed because in some cases there's more than one source directory, and
// this function will be called multiple times for a single path in a single
// include action. The multiple source directories are effectively "overlaid" so
// that we're actually including from all of them, with later ones taking
// precedence over earlier ones, if the same file exists in all of them.
func includeFromOneDir(ctx context.Context, inc *spec.IncludePath, sp *stepParams, fromDir string) (matchedAny bool, _ error) {
	skipPaths, err := processPaths(inc.Skip, sp.scope)
	if err != nil {
		return false, err
	}
	if fromDir == sp.templateDir {
		// If we're copying the template root directory, automatically skip
		// 1. spec.yaml file, because it's very unlikely that the user actually
		// wants the spec file in the template output.
		// 2. testdata/golden directory, this is reserved for golden test usage.
		skipPaths = append(skipPaths,
			model.String{
				Val: "spec.yaml",
			},
			model.String{
				Val: filepath.Join("testdata", "golden"),
			},
		)
	}

	// During validation in spec.go, we've already enforced that either:
	// len(asPaths) is either == 0 or == len(incPaths).
	asPaths, err := processPaths(inc.As, sp.scope)
	if err != nil {
		return false, err
	}

	incPaths, err := processPaths(inc.Paths, sp.scope)
	if err != nil {
		return false, err
	}

	anyMatches := false
	for i, p := range incPaths {
		matchedPaths, err := processGlobs(ctx, []model.String{p}, fromDir, sp.features.SkipGlobs)
		if err != nil {
			return false, err
		}

		for _, absSrc := range matchedPaths {
			anyMatches = true
			relSrc, err := filepath.Rel(fromDir, absSrc.Val)
			if err != nil {
				return false, fmt.Errorf("internal error making relative path: %w", err)
			}

			// if no As val was provided, use the original file or directory name.
			relDst := relSrc
			// As val provided, check if pattern has file globbing
			if len(asPaths) > 0 {
				if isGlob(matchedPaths, filepath.Join(fromDir, p.Val), absSrc.Val) {
					// path is a glob, keep original filename and put inside directory named as the provided As val.
					relDst = filepath.Join(asPaths[i].Val, relSrc)
				} else {
					// otherwise use provided As val as new filename.
					relDst = asPaths[i].Val
				}
			}
			absDst := filepath.Join(sp.scratchDir, relDst)

			if err := copyToDst(ctx, sp, skipPaths, absSrc.Pos, absDst, absSrc.Val, relSrc, inc.From.Val, fromDir); err != nil {
				return false, err
			}
		}
	}
	return anyMatches, nil
}

// checkIgnore checks the given path against the given patterns, if given
// patterns is not provided, a default list of patterns is used.
func checkIgnore(patterns []model.String, path string) (bool, error) {
	if len(patterns) == 0 {
		patterns = defaultIgnorePatterns
	}
	for _, p := range patterns {
		var matched bool
		var err error
		if filepath.Base(p.Val) == p.Val {
			// Match file name if the pattern value is file name instead of path.
			matched, err = filepath.Match(p.Val, filepath.Base(path))
		} else if p.Val[0] == '/' {
			// Match pattern with a leading slash as it is from the same root as path.
			matched, err = filepath.Match(p.Val[1:], path)
		} else {
			// Math pattern using relative path.
			matched, err = filepath.Match(p.Val, path)
		}
		if err != nil {
			return false,
				p.Pos.Errorf("failed to match path (%q) with pattern (%q): %w", path, p.Val, err)
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}
