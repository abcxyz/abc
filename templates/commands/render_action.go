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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/pkg/logging"
	"golang.org/x/exp/maps"
)

// Called with the contents of a file, and returns the new contents of the file
// to be written.
type walkAndModifyVisitor func([]byte) ([]byte, error)

// For each given path, recursively traverses the directory or file
// scratchDir/relPath, calling the given visitor for each file. If relPath is a single file, then the visitor
// will be called for just that one file. If relPath is a directory, then the
// visitor will be called for all files under that directory, recursively.
// A file will only be visited once per call, even if multiple paths include
// it.
//
// If the visitor returns modified file contents for a given file, that file
// will be overwritten with the new contents.
func walkAndModify(ctx context.Context, rfs renderFS, scratchDir string, relPaths []model.String, v walkAndModifyVisitor) error {
	logger := logging.FromContext(ctx).With("logger", "walkAndModify")
	seen := map[string]struct{}{}

	for _, relPathPos := range relPaths {
		pos := relPathPos.Pos
		relPath := relPathPos.Val
		relPath, err := safeRelPath(pos, relPath)
		if err != nil {
			return err
		}
		walkFrom := filepath.Join(scratchDir, relPath)
		if _, err := rfs.Stat(walkFrom); err != nil {
			if os.IsNotExist(err) {
				return model.ErrWithPos(pos, `path %q doesn't exist in the scratch directory, did you forget to "include" it first?"`, relPath) //nolint:wrapcheck
			}
			return model.ErrWithPos(pos, "Stat(): %w", err) //nolint:wrapcheck
		}

		err = filepath.WalkDir(walkFrom, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// There was some filesystem error. Give up.
				return model.ErrWithPos(pos, "%w", err) //nolint:wrapcheck
			}
			if d.IsDir() {
				return nil
			}

			if _, ok := seen[path]; ok {
				// File already processed.
				logger.DebugContext(ctx, "skipping file as already seen", "path", path)
				return nil
			}
			oldBuf, err := rfs.ReadFile(path)
			if err != nil {
				return model.ErrWithPos(pos, "Readfile(): %w", err) //nolint:wrapcheck
			}

			relToScratchDir, err := filepath.Rel(scratchDir, path)
			if err != nil {
				return model.ErrWithPos(pos, "Rel(): %w", err) //nolint:wrapcheck
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
			if err := rfs.WriteFile(path, newBuf, ownerRWXPerms); err != nil {
				return model.ErrWithPos(pos, "Writefile(): %w", err) //nolint:wrapcheck
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

func templateAndCompileRegexes(regexes []model.String, scope *scope) ([]*regexp.Regexp, error) {
	compiled := make([]*regexp.Regexp, len(regexes))
	var merr error
	for i, re := range regexes {
		templated, err := parseAndExecuteGoTmpl(re.Pos, re.Val, scope)
		if err != nil {
			merr = errors.Join(merr, err)
			continue
		}

		compiled[i], err = regexp.Compile(templated)
		if err != nil {
			merr = errors.Join(merr, model.ErrWithPos(re.Pos, "failed compiling regex: %w", err))
			continue
		}
	}

	return compiled, merr
}

// templateFuncs returns a function map for adding functions to go templates.
func templateFuncs() template.FuncMap {
	return map[string]any{
		"contains":          strings.Contains,
		"replace":           strings.Replace,
		"replaceAll":        strings.ReplaceAll,
		"sortStrings":       sortStrings,
		"split":             strings.Split,
		"toLower":           strings.ToLower,
		"toUpper":           strings.ToUpper,
		"trimPrefix":        strings.TrimPrefix,
		"trimSuffix":        strings.TrimSuffix,
		"trimSpace":         strings.TrimSpace,
		"toSnakeCase":       toSnakeCase,
		"toLowerSnakeCase":  toLowerSnakeCase,
		"toUpperSnakeCase":  toUpperSnakeCase,
		"toHyphenCase":      toHyphenCase,
		"toLowerHyphenCase": toLowerHyphenCase,
		"toUpperHyphenCase": toUpperHyphenCase,
	}
}

// A template parser helper to remove the boilerplate of parsing with our
// desired options.
func parseGoTmpl(tpl string) (*template.Template, error) {
	return template.New("").Funcs(templateFuncs()).Option("missingkey=error").Parse(tpl) //nolint:wrapcheck
}

var templateKeyErrRegex = regexp.MustCompile(`map has no entry for key "([^"]*)"`)

// pos may be nil if the template is not coming from the spec file and therefore
// there's no reason to print out spec file location in an error message. If
// template execution fails because of a missing input variable, the error will
// be wrapped in a unknownTemplateKeyError.
func parseAndExecuteGoTmpl(pos *model.ConfigPos, tmpl string, scope *scope) (string, error) {
	parsedTmpl, err := parseGoTmpl(tmpl)
	if err != nil {
		return "", model.ErrWithPos(pos, `error compiling as go-template: %w`, err) //nolint:wrapcheck
	}

	// As of go1.20, if the template references a nonexistent variable, then the
	// returned error will be of type *errors.errorString; unfortunately there's
	// no distinctive error type we can use to detect this particular error.
	//
	// We only get this error because we asked for Option("missingkey=error")
	// when parsing the template. Otherwise it would silently insert "<no
	// value>".
	var sb strings.Builder
	vars := scope.All()
	if err := parsedTmpl.Execute(&sb, vars); err != nil {
		// If this error looks like a missing key error, then replace it with a
		// more helpful error.
		matches := templateKeyErrRegex.FindStringSubmatch(err.Error())
		if matches != nil {
			varNames := maps.Keys(vars)
			sort.Strings(varNames)
			err = &unknownTemplateKeyError{
				key:           matches[1],
				availableKeys: varNames,
				wrapped:       err,
			}
		}
		return "", model.ErrWithPos(pos, "template.Execute() failed: %w", err) //nolint:wrapcheck
	}
	return sb.String(), nil
}

func parseAndExecuteGoTmplAll(ss []model.String, scope *scope) ([]string, error) {
	out := make([]string, len(ss))
	for i, in := range ss {
		var err error
		out[i], err = parseAndExecuteGoTmpl(in.Pos, in.Val, scope)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// unknownTemplateKeyError is an error that will be returned when a template
// references a variable that's nonexistent.
type unknownTemplateKeyError struct {
	key           string
	availableKeys []string
	wrapped       error
}

func (n *unknownTemplateKeyError) Error() string {
	return fmt.Sprintf("the template referenced a nonexistent input variable name %q; available variable names are %v",
		n.key, n.availableKeys)
}

func (n *unknownTemplateKeyError) Unwrap() error {
	return n.wrapped
}

func (n *unknownTemplateKeyError) Is(other error) bool {
	_, ok := other.(*unknownTemplateKeyError) //nolint:errorlint // errors.As() should never be used inside errors.Is()
	return ok
}

// copyParams contains most of the parameters to copyRecursive(). There were too
// many of these, so they've been factored out into a struct to avoid having the
// function parameter list be really long.
type copyParams struct {
	// backupDirMaker will be called when we reach the first file that actually
	// needs to be backed up. It should create a directory and return its path,
	// either relative to the cwd or absolute. Use os.MkdirTemp() in real code
	// and something hardcoded in tests.
	backupDirMaker func(renderFS) (string, error)
	// // backupDir provides the path at which files will be saved before they're
	// // overwritten.
	// backupDir string
	// dryRun skips actually copy anything, just checks whether the copy would
	// be likely to succeed.
	dryRun bool
	// dstRoot is the output directory.
	dstRoot string
	// srcRoot is the file or directory from which to copy.
	srcRoot string
	// rfs is the filesytem to use.
	rfs renderFS
	// visitor is an optional function that will be called for each file in the
	// source, to allow customization of the copy operation on a per-file basis.
	visitor copyVisitor
}

// copyVisitor is the type for callback functions that are called by
// copyRecursive for each file and directory encountered. It gives the caller an
// opportunity to influence the behavior of the copy operation on a per-file
// basis, and also informs the of each file and directory being copied.
type copyVisitor func(relPath string, de fs.DirEntry) (copyHint, error)

type copyHint struct {
	// Before overwriting a file in the destination dir, copy the preexisting
	// contents of the file into ~/.abc/$timestamp. Only used if
	// overwrite==true.
	//
	// This has no effect on directories, only files.
	backupIfExists bool

	// Overwrite files in the destination if they already exist. The default is
	// to conservatively fail.
	//
	// This has no effect on directories, only files.
	overwrite bool

	// Whether to skip this file or directory (don't write it to the
	// destination). For directories, this will cause all files underneath the
	// directory to be skipped.
	skip bool
}

func copyRecursive(ctx context.Context, pos *model.ConfigPos, p *copyParams) (outErr error) {
	logger := logging.FromContext(ctx).With("logger", "copyRecursive")

	backupDir := "" // will be set once the backup dir is actually created

	return fs.WalkDir(p.rfs, p.srcRoot, func(path string, de fs.DirEntry, err error) error { //nolint:wrapcheck
		if err != nil {
			return err // There was some filesystem error. Give up.
		}
		// We don't have to worry about symlinks here because we passed
		// DisableSymlinks=true to go-getter.
		relToSrc, err := filepath.Rel(p.srcRoot, path)
		if err != nil {
			return model.ErrWithPos(pos, "filepath.Rel(%s,%s): %w", p.srcRoot, path, err) //nolint:wrapcheck
		}
		dst := filepath.Join(p.dstRoot, relToSrc)

		var ch copyHint
		if p.visitor != nil {
			ch, err = p.visitor(relToSrc, de)
			if err != nil {
				return err
			}
		}

		if ch.skip {
			logger.DebugContext(ctx, "walkdir visitor skipped file or directory", "path", relToSrc)
			if de.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		if de.IsDir() {
			// We don't create directories when they're encountered by this WalkDirFunc.
			// Instead, we create output directories as needed when a file needs to be
			// placed in that directory.
			return nil
		}

		// The spec file may specify a file to copy that's deep in a directory
		// tree, (like include "some/deep/subdir/myfile.txt") without including
		// its parent directory. We can't rely on WalkDir having traversed the
		// parent directory of $path, so we must create the target directory if
		// it doesn't exist.
		inDir := filepath.Dir(dst)
		if err := mkdirAllChecked(pos, p.rfs, inDir, p.dryRun); err != nil {
			return err
		}
		dstInfo, err := p.rfs.Stat(dst)
		if err == nil {
			if dstInfo.IsDir() {
				return model.ErrWithPos(pos, "cannot overwrite a directory with a file of the same name, %q", relToSrc) //nolint:wrapcheck
			}
			if !ch.overwrite {
				return model.ErrWithPos(pos, "destination file %s already exists and overwriting was not enabled with --force-overwrite", relToSrc) //nolint:wrapcheck
			}
			if ch.backupIfExists && !p.dryRun {
				if backupDir == "" {
					if backupDir, err = p.backupDirMaker(p.rfs); err != nil {
						return fmt.Errorf("failed making backup directory: %w", err)
					}
				}
				if err := backUp(ctx, p.rfs, backupDir, p.dstRoot, relToSrc); err != nil {
					return err
				}
			}
		} else if !os.IsNotExist(err) {
			return model.ErrWithPos(pos, "Stat(): %w", err) //nolint:wrapcheck
		}
		srcInfo, err := p.rfs.Stat(path)
		if err != nil {
			return fmt.Errorf("Stat(): %w", err)
		}

		// The permission bits on the output file are copied from the input file;
		// this preserves the execute bit on executable files.
		mode := srcInfo.Mode().Perm()
		return copyFile(ctx, pos, p.rfs, path, dst, mode, p.dryRun)
	})
}

func copyFile(ctx context.Context, pos *model.ConfigPos, rfs renderFS, src, dst string, mode fs.FileMode, dryRun bool) (outErr error) {
	logger := logging.FromContext(ctx).With("logger", "copyFile")

	readFile, err := rfs.Open(src)
	if err != nil {
		return model.ErrWithPos(pos, "Open(): %w", err) //nolint:wrapcheck
	}
	defer func() { outErr = errors.Join(outErr, readFile.Close()) }()

	if dryRun {
		return nil
	}

	writeFile, err := rfs.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return model.ErrWithPos(pos, "OpenFile(): %w", err) //nolint:wrapcheck
	}
	defer func() { outErr = errors.Join(outErr, writeFile.Close()) }()

	if _, err := io.Copy(writeFile, readFile); err != nil {
		return fmt.Errorf("Copy(): %w", err)
	}
	logger.DebugContext(ctx, "copied file", "source", src, "destination", dst)
	return nil
}

// safeRelPath returns an error if the path contains a ".." traversal, and
// converts it to a relative path by removing any leading "/".
func safeRelPath(pos *model.ConfigPos, p string) (string, error) {
	if strings.Contains(p, "..") {
		return "", model.ErrWithPos(pos, `path %q must not contain ".."`, p) //nolint:wrapcheck
	}
	return strings.TrimLeft(p, string(filepath.Separator)), nil
}

// backUp saves the file $srcRoot/$relPath into backupDir.
//
// When we overwrite a file in the destination dir, we back up the old version
// in case the user had uncommitted changes in that file that were unrelated to
// abc.
func backUp(ctx context.Context, rfs renderFS, backupDir, srcRoot, relPath string) error {
	backupFile := filepath.Join(backupDir, relPath)
	parent := filepath.Dir(backupFile)
	if err := os.MkdirAll(parent, ownerRWXPerms); err != nil {
		return fmt.Errorf("os.MkdirAll(%s): %w", parent, err)
	}

	fileToBackup := filepath.Join(srcRoot, relPath)

	if err := copyFile(ctx, nil, rfs, fileToBackup, backupFile, ownerRWPerms, false); err != nil {
		return fmt.Errorf("failed backing up file %q at %q before overwriting: %w",
			fileToBackup, backupFile, err)
	}

	logger := logging.FromContext(ctx)
	logger.InfoContext(ctx, "completed backup", "source", fileToBackup, "destination", backupFile)

	return nil
}
