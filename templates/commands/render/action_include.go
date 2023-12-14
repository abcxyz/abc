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
	spec "github.com/abcxyz/abc/templates/model/spec/v1beta1"
	"golang.org/x/exp/maps"
)

func actionInclude(ctx context.Context, inc *spec.Include, sp *stepParams) error {
	for _, path := range inc.Paths {
		if err := includePath(ctx, path, sp); err != nil {
			return err
		}
	}
	return nil
}

func includePath(ctx context.Context, inc *spec.IncludePath, sp *stepParams) error {
	// By default, we copy from the template directory. We also support
	// grabbing files from the destination directory, so we can modify files
	// that already exist in the destination.
	fromDir := sp.templateDir
	if inc.From.Val == "destination" {
		fromDir = sp.flags.Dest
	}

	unglobbedSkipPaths, err := processPaths(inc.Skip, sp.scope)
	if err != nil {
		return err
	}
	skipPaths, err := processGlobs(ctx, unglobbedSkipPaths, fromDir)
	if err != nil {
		return err
	}
	skip := make(map[string]struct{}, len(inc.Skip))
	for _, s := range skipPaths {
		relSkipPath, err := filepath.Rel(fromDir, s.Val)
		if err != nil {
			return err
		}
		skip[relSkipPath] = struct{}{}
	}

	skipNow := maps.Clone(skip)
	if fromDir == sp.templateDir {
		// If we're copying the template root directory, automatically skip
		// 1. spec.yaml file, because it's very unlikely that the user actually
		// wants the spec file in the template output.
		// 2. testdata/golden directory, this is reserved for golden test usage.
		skipNow["spec.yaml"] = struct{}{}
		skipNow[filepath.Join("testdata", "golden")] = struct{}{}
	}

	asPaths, err := processPaths(inc.As, sp.scope)
	if err != nil {
		return err
	}

	incPaths, err := processPaths(inc.Paths, sp.scope)
	if err != nil {
		return err
	}

	for i, p := range incPaths {
		matchedPaths, err := processGlobs(ctx, []model.String{p}, fromDir)
		if err != nil {
			return err
		}

		for _, matchedPath := range matchedPaths {
			// During validation in spec.go, we've already enforced that either:
			// len(asPaths) is either == 0 or == len(incPaths).
			relMatchedPath, err := filepath.Rel(fromDir, matchedPath.Val)
			if err != nil {
				return err
			}

			as := relMatchedPath
			if len(asPaths) > 0 { // As provided
				if len(matchedPaths) != 1 || matchedPath.Val != filepath.Join(fromDir, p.Val) {
					// path is a glob, keep original filename and put inside directory named by the provided As val.
					as = filepath.Join(asPaths[i].Val, relMatchedPath)
				} else {
					// otherwise use provided As val as new filename.
					as = asPaths[i].Val
				}
			}
			absDst := filepath.Join(sp.scratchDir, as)

			params := &common.CopyParams{
				DryRun:  false,
				DstRoot: absDst,
				RFS:     sp.fs,
				SrcRoot: matchedPath.Val,
				Visitor: func(relToSrcRoot string, de fs.DirEntry) (common.CopyHint, error) {
					if _, ok := skipNow[filepath.Join(relMatchedPath, relToSrcRoot)]; ok {
						return common.CopyHint{
							Skip: true,
						}, nil
					}

					abs := filepath.Join(matchedPath.Val, relToSrcRoot)
					relToFromDir, err := filepath.Rel(fromDir, abs)
					if err != nil {
						return common.CopyHint{}, fmt.Errorf("filepath.Rel(%s,%s)=%w", fromDir, matchedPath.Val, err)
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
	}
	return nil
}
