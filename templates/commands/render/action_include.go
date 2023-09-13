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
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/abcxyz/abc/templates/model/spec"
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
	skipPaths, err := processPaths(inc.Skip, sp)
	if err != nil {
		return err
	}
	skip := make(map[string]struct{}, len(inc.Skip))
	for _, s := range skipPaths {
		skip[s.Val] = struct{}{}
	}

	incPaths, err := processPaths(inc.Paths, sp)
	if err != nil {
		return err
	}

	asPaths, err := processPaths(inc.As, sp)
	if err != nil {
		return err
	}

	for i, p := range incPaths {
		// During validation in spec.go, we've already enforced that either:
		//  - len(inc.As) == 0
		//  - len(inc.As) == len(inc.Paths)
		as := p.Val
		if len(inc.As) > 0 {
			as = asPaths[i].Val
		}

		// By default, we copy from the template directory. We also support
		// grabbing files from the destination directory, so we can modify files
		// that already exist in the destination.
		fromDir := sp.templateDir
		if inc.From.Val == "destination" {
			fromDir = sp.flags.Dest
		}
		absSrc := filepath.Join(fromDir, p.Val)
		absDst := filepath.Join(sp.scratchDir, as)

		skipNow := maps.Clone(skip)
		if absSrc == sp.templateDir {
			// If we're copying the template root directory, automatically skip
			// the spec.yaml file, because it's very unlikely that the user actually
			// wants the spec file in the template output.
			skipNow["spec.yaml"] = struct{}{}
		}

		if _, err := sp.fs.Stat(absSrc); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return p.Pos.Errorf("include path doesn't exist: %q", p.Val)
			}
			return fmt.Errorf("Stat(): %w", err)
		}

		params := &copyParams{
			dryRun:  false,
			dstRoot: absDst,
			rfs:     sp.fs,
			srcRoot: absSrc,
			visitor: func(relToAbsSrc string, de fs.DirEntry) (copyHint, error) {
				if _, ok := skipNow[relToAbsSrc]; ok {
					return copyHint{
						skip: true,
					}, nil
				}

				abs := filepath.Join(absSrc, relToAbsSrc)
				relToFromDir, err := filepath.Rel(fromDir, abs)
				if err != nil {
					return copyHint{}, fmt.Errorf("filepath.Rel(%s,%s)=%w", fromDir, abs, err)
				}
				if !de.IsDir() && inc.From.Val == "destination" {
					sp.includedFromDest = append(sp.includedFromDest, relToFromDir)
				}

				return copyHint{
					// Allow later includes to replace earlier includes in the
					// scratch directory. This doesn't affect whether files in
					// the final *destination* directory will be overwritten;
					// that comes later.
					overwrite: true,
				}, nil
			},
		}
		if err := copyRecursive(ctx, p.Pos, params); err != nil {
			return p.Pos.Errorf("copying failed: %w", err)
		}
	}
	return nil
}
