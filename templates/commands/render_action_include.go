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

package commands

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/abcxyz/abc/templates/model"
	"golang.org/x/exp/maps"
)

func actionInclude(ctx context.Context, inc *model.Include, sp *stepParams) error {
	stripPrefixStr, err := parseAndExecuteGoTmpl(inc.StripPrefix.Pos, inc.StripPrefix.Val, sp.scope)
	if err != nil {
		return err
	}
	addPrefixStr, err := parseAndExecuteGoTmpl(inc.AddPrefix.Pos, inc.AddPrefix.Val, sp.scope)
	if err != nil {
		return err
	}

	skip := make(map[string]struct{}, len(inc.Skip))
	for _, s := range inc.Skip {
		skipRelPath, err := parseAndExecuteGoTmpl(s.Pos, s.Val, sp.scope)
		if err != nil {
			return err
		}
		skip[skipRelPath] = struct{}{}
	}

	for i, p := range inc.Paths {
		// Paths may contain template expressions, so render them first.
		walkRelPath, err := parseAndExecuteGoTmpl(p.Pos, p.Val, sp.scope)
		if err != nil {
			return err
		}

		walkRelPath, err = safeRelPath(inc.Pos, walkRelPath)
		if err != nil {
			return err
		}

		// During validation in spec.go, we've already enforced that either:
		//  - len(inc.As) == 0
		//  - len(inc.As) == len(inc.Paths)
		var as string
		if len(inc.As) > 0 {
			as, err = parseAndExecuteGoTmpl(inc.As[i].Pos, inc.As[i].Val, sp.scope)
			if err != nil {
				return err
			}
		}

		relDst, err := dest(p.Pos, walkRelPath, as, stripPrefixStr, addPrefixStr)
		if err != nil {
			return err
		}

		// By default, we copy from the template directory. We also support
		// grabbing files from the destination directory, so we can modify files
		// that already exist in the destination.
		fromDir := sp.templateDir
		if inc.From.Val == "destination" {
			fromDir = sp.flags.Dest
		}
		absSrc := filepath.Join(fromDir, walkRelPath)
		absDst := filepath.Join(sp.scratchDir, relDst)

		skipNow := maps.Clone(skip)
		if absSrc == sp.templateDir {
			// If we're copying the template root directory, automatically skip
			// the spec.yaml file, because it's very unlikely that the user actually
			// wants the spec file in the template output.
			skipNow["spec.yaml"] = struct{}{}
		}

		if _, err := sp.fs.Stat(absSrc); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return model.ErrWithPos(p.Pos, "include path doesn't exist: %q", walkRelPath) //nolint:wrapcheck
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
			return model.ErrWithPos(p.Pos, "copying failed: %w", err) //nolint:wrapcheck
		}
	}
	return nil
}

// The caller must have already executed the go-templates for all inputs.
func dest(pathPos *model.ConfigPos, relPath, as, stripPrefix, addPrefix string) (string, error) {
	// inc.As is mutually exclusive with inc.StripPrefix and inc.AddPrefix. This
	// exclusivity is enforced earlier, during validation.
	if as != "" {
		var err error
		as, err = safeRelPath(pathPos, as)
		if err != nil {
			return "", err
		}
		return as, nil
	}

	if stripPrefix != "" {
		before := relPath
		relPath = strings.TrimPrefix(relPath, stripPrefix)
		if relPath == before {
			return "", model.ErrWithPos(pathPos, "the strip_prefix %q wasn't a prefix of the actual path %q", //nolint:wrapcheck
				stripPrefix, relPath)
		}
	}

	if addPrefix != "" {
		relPath = addPrefix + relPath
	}

	return safeRelPath(pathPos, relPath)
}
