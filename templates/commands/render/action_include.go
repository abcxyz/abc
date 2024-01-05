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

	"golang.org/x/exp/maps"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/model"
	spec "github.com/abcxyz/abc/templates/model/spec/v1beta3"
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

func includePath(ctx context.Context, inc *spec.IncludePath, sp *stepParams) error {
	logger := logging.FromContext(ctx).With("logger", "includePath")

	// By default, we copy from the template directory. We also support
	// grabbing files from the destination directory, so we can modify files
	// that already exist in the destination.
	fromDir := sp.templateDir
	if inc.From.Val == "destination" {
		fromDir = sp.flags.Dest
	}

	skipPaths, err := processPaths(inc.Skip, sp.scope)
	if err != nil {
		return err
	}
	skip := make(map[string]struct{}, len(inc.Skip))
	for _, s := range skipPaths {
		skip[s.Val] = struct{}{}
	}

	incPaths, err := processPaths(inc.Paths, sp.scope)
	if err != nil {
		return err
	}

	asPaths, err := processPaths(inc.As, sp.scope)
	if err != nil {
		return err
	}

	for i, p := range incPaths {
		// During validation in spec.go, we've already enforced that either:
		// len(asPaths) is either == 0 or == len(incPaths).
		as := p.Val
		if len(asPaths) > 0 {
			as = asPaths[i].Val
		}

		absSrc := filepath.Join(fromDir, p.Val)
		absDst := filepath.Join(sp.scratchDir, as)

		skipNow := maps.Clone(skip)
		if absSrc == sp.templateDir {
			// If we're copying the template root directory, automatically skip
			// 1. spec.yaml file, because it's very unlikely that the user actually
			// wants the spec file in the template output.
			// 2. testdata/golden directory, this is reserved for golden test usage.
			skipNow["spec.yaml"] = struct{}{}
			skipNow[filepath.Join("testdata", "golden")] = struct{}{}
		}

		if _, err := sp.fs.Stat(absSrc); err != nil {
			if common.IsStatNotExistErr(err) {
				return p.Pos.Errorf("include path doesn't exist: %q", p.Val)
			}
			return fmt.Errorf("Stat(): %w", err)
		}

		params := &common.CopyParams{
			DryRun:  false,
			DstRoot: absDst,
			RFS:     sp.fs,
			SrcRoot: absSrc,
			Visitor: func(relToAbsSrc string, de fs.DirEntry) (common.CopyHint, error) {
				if _, ok := skipNow[relToAbsSrc]; ok {
					return common.CopyHint{
						Skip: true,
					}, nil
				}

				abs := filepath.Join(absSrc, relToAbsSrc)
				relToFromDir, err := filepath.Rel(fromDir, abs)
				if err != nil {
					return common.CopyHint{}, fmt.Errorf("filepath.Rel(%s,%s)=%w", fromDir, abs, err)
				}
				matched, err := checkIgnore(sp.ignorePatterns, relToFromDir, fromDir)
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

				if !de.IsDir() && inc.From.Val == "destination" {
					sp.includedFromDest = append(sp.includedFromDest, relToFromDir)
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
		if err := common.CopyRecursive(ctx, p.Pos, params); err != nil {
			return p.Pos.Errorf("copying failed: %w", err)
		}
	}
	return nil
}

// checkIgnore checks the given path against the given patterns, if given
// patterns is not provided, a default list of patterns is used.
func checkIgnore(patterns []model.String, path, fromDir string) (bool, error) {
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
			// Match absolute path if the pattern start with a leading slash.
			matched, err = filepath.Match(filepath.Join(fromDir, filepath.FromSlash(p.Val)), filepath.Join(fromDir, path))
		} else {
			// Math pattern using relative path.
			matched, err = filepath.Match(filepath.FromSlash(p.Val), path)
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
