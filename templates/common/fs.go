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

package common

import (
	"context"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/pkg/logging"
)

const (
	// Permission bits: rwx------ .
	ownerRWXPerms = 0o700
	// Permission bits: rw------- .
	ownerRWPerms = 0o600
)

// Abstracts filesystem operations.
//
// We can't use os.DirFS or fs.StatFS because they lack some methods we need. So
// we created our own interface.
type FS interface {
	fs.StatFS

	// These methods correspond to methods in the "os" package of the same name.
	MkdirAll(string, os.FileMode) error
	MkdirTemp(string, string) (string, error)
	OpenFile(string, int, os.FileMode) (*os.File, error)
	ReadFile(string) ([]byte, error)
	RemoveAll(string) error
	WriteFile(string, []byte, os.FileMode) error
}

// This is the non-test implementation of the filesystem interface.
type RealFS struct{}

func (r *RealFS) MkdirAll(name string, perm os.FileMode) error {
	return os.MkdirAll(name, perm) //nolint:wrapcheck
}

func (r *RealFS) MkdirTemp(dir, pattern string) (string, error) {
	return os.MkdirTemp(dir, pattern) //nolint:wrapcheck
}

func (r *RealFS) Open(name string) (fs.File, error) {
	return os.Open(name) //nolint:wrapcheck
}

func (r *RealFS) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(name, flag, perm) //nolint:wrapcheck
}

func (r *RealFS) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name) //nolint:wrapcheck
}

func (r *RealFS) RemoveAll(name string) error {
	return os.RemoveAll(name) //nolint:wrapcheck
}

func (r *RealFS) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(name) //nolint:wrapcheck
}

func (r *RealFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(name, data, perm) //nolint:wrapcheck
}

// CopyParams contains most of the parameters to CopyRecursive(). There were too
// many of these, so they've been factored out into a struct to avoid having the
// function parameter list be really long.
type CopyParams struct {
	// backupDirMaker will be called when we reach the first file that actually
	// needs to be backed up. It should create a directory and return its path,
	// either relative to the cwd or absolute. Use os.MkdirTemp() in real code
	// and something hardcoded in tests.
	BackupDirMaker func(FS) (string, error)
	// // backupDir provides the path at which files will be saved before they're
	// // overwritten.
	// backupDir string
	// dryRun skips actually copy anything, just checks whether the copy would
	// be likely to succeed.
	DryRun bool
	// dstRoot is the output directory.
	DstRoot string
	// srcRoot is the file or directory from which to copy.
	SrcRoot string
	// RFS is the filesytem to use.
	RFS FS
	// visitor is an optional function that will be called for each file in the
	// source, to allow customization of the copy operation on a per-file basis.
	Visitor CopyVisitor

	// If Hasher and OutHashes are not nil, then each copied file will be hashed
	// and the raw binary hash will be saved in OutHashes. If a file is
	// "skipped" (CopyHint.Skip==true) then the hash will not be computed. In
	// dry run mode, the hash will be computed normally. OutHashes always uses
	// forward slashes as path separator, regardless of OS.
	Hasher    func() hash.Hash
	OutHashes map[string][]byte
}

// CopyVisitor is the type for callback functions that are called by
// CopyRecursive for each file and directory encountered. It gives the caller an
// opportunity to influence the behavior of the copy operation on a per-file
// basis, and also informs the of each file and directory being copied.
type CopyVisitor func(relPath string, de fs.DirEntry) (CopyHint, error)

type CopyHint struct {
	// Before overwriting a file in the destination dir, copy the preexisting
	// contents of the file into ~/.abc/$timestamp. Only used if
	// overwrite==true.
	//
	// This has no effect on directories, only files.
	BackupIfExists bool

	// Overwrite files in the destination if they already exist. The default is
	// to conservatively fail.
	//
	// This has no effect on directories, only files.
	Overwrite bool

	// Whether to skip this file or directory (don't write it to the
	// destination). For directories, this will cause all files underneath the
	// directory to be skipped.
	Skip bool
}

// CopyRecursive recursively copy folder contents with designated config params.
func CopyRecursive(ctx context.Context, pos *model.ConfigPos, p *CopyParams) (outErr error) {
	logger := logging.FromContext(ctx).With("logger", "CopyRecursive")

	backupDir := "" // will be set once the backup dir is actually created

	return fs.WalkDir(p.RFS, p.SrcRoot, func(path string, de fs.DirEntry, err error) error { //nolint:wrapcheck
		if err != nil {
			return err // There was some filesystem error. Give up.
		}
		relToSrc, err := filepath.Rel(p.SrcRoot, path)
		if err != nil {
			return pos.Errorf("filepath.Rel(%s,%s): %w", p.SrcRoot, path, err)
		}
		dst := filepath.Join(p.DstRoot, relToSrc)

		var ch CopyHint
		if p.Visitor != nil {
			ch, err = p.Visitor(relToSrc, de)
			if err != nil {
				return err
			}
		}

		if ch.Skip {
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
		if err := mkdirAllChecked(pos, p.RFS, inDir, p.DryRun); err != nil {
			return err
		}
		dstInfo, err := p.RFS.Stat(dst)
		if err == nil {
			if dstInfo.IsDir() {
				return pos.Errorf("cannot overwrite a directory with a file of the same name, %q", relToSrc)
			}
			if !ch.Overwrite {
				return pos.Errorf("destination file %s already exists and overwriting was not enabled with --force-overwrite", relToSrc)
			}
			if ch.BackupIfExists && !p.DryRun {
				if backupDir == "" {
					if backupDir, err = p.BackupDirMaker(p.RFS); err != nil {
						return fmt.Errorf("failed making backup directory: %w", err)
					}
				}
				if err := backUp(ctx, p.RFS, backupDir, p.DstRoot, relToSrc); err != nil {
					return err
				}
			}
		} else if !os.IsNotExist(err) {
			return pos.Errorf("Stat(): %w", err)
		}
		srcInfo, err := p.RFS.Stat(path)
		if err != nil {
			return fmt.Errorf("Stat(): %w", err)
		}

		// The permission bits on the output file are copied from the input file;
		// this preserves the execute bit on executable files.
		mode := srcInfo.Mode().Perm()
		var hash hash.Hash
		if p.Hasher != nil {
			hash = p.Hasher()
		}
		if err := copyFile(ctx, pos, p.RFS, path, dst, mode, p.DryRun, hash); err != nil {
			return err
		}
		if hash != nil && p.OutHashes != nil {
			p.OutHashes[filepath.ToSlash(relToSrc)] = hash.Sum(nil)
		}
		return nil
	})
}

// copyFile copies the contents of src to dst.
//
// hash is nil-able. If not nil, it will be written to with the file contents.
// The caller should call hash.Sum() to get the hash output.
func copyFile(ctx context.Context, pos *model.ConfigPos, rfs FS, src, dst string, mode fs.FileMode, dryRun bool, hash hash.Hash) (outErr error) {
	logger := logging.FromContext(ctx).With("logger", "copyFile")

	readFile, err := rfs.Open(src)
	if err != nil {
		return pos.Errorf("Open(): %w", err)
	}
	defer func() { outErr = errors.Join(outErr, readFile.Close()) }()
	var reader io.Reader = readFile

	var writer io.Writer
	if dryRun {
		writer = io.Discard
	} else {
		writeFile, err := rfs.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			return pos.Errorf("OpenFile(): %w", err)
		}
		defer func() { outErr = errors.Join(outErr, writeFile.Close()) }()
		writer = writeFile
	}

	if hash != nil {
		reader = io.TeeReader(readFile, hash)
	}

	if _, err := io.Copy(writer, reader); err != nil {
		return fmt.Errorf("Copy(): %w", err)
	}
	logger.DebugContext(ctx, "copied file", "source", src, "destination", dst)
	return nil
}

// backUp saves the file $srcRoot/$relPath into backupDir.
//
// When we overwrite a file in the destination dir, we back up the old version
// in case the user had uncommitted changes in that file that were unrelated to
// abc.
func backUp(ctx context.Context, rfs FS, backupDir, srcRoot, relPath string) error {
	backupFile := filepath.Join(backupDir, relPath)
	parent := filepath.Dir(backupFile)
	if err := os.MkdirAll(parent, ownerRWXPerms); err != nil {
		return fmt.Errorf("os.MkdirAll(%s): %w", parent, err)
	}

	fileToBackup := filepath.Join(srcRoot, relPath)

	if err := copyFile(ctx, nil, rfs, fileToBackup, backupFile, ownerRWPerms, false, nil); err != nil {
		return fmt.Errorf("failed backing up file %q at %q before overwriting: %w",
			fileToBackup, backupFile, err)
	}

	logger := logging.FromContext(ctx)
	logger.DebugContext(ctx, "completed backup", "source", fileToBackup, "destination", backupFile)

	return nil
}

// A fancy wrapper around MkdirAll with better error messages and a dry run
// mode. In dry run mode, returns an error if the MkdirAll wouldn't succeed
// (best-effort).
func mkdirAllChecked(pos *model.ConfigPos, rfs FS, path string, dryRun bool) error {
	create := false
	info, err := rfs.Stat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return pos.Errorf("Stat(): %w", err)
		}
		create = true
	} else if !info.Mode().IsDir() {
		return pos.Errorf("cannot overwrite a file with a directory of the same name, %q", path)
	}

	if dryRun || !create {
		return nil
	}

	if err := rfs.MkdirAll(path, ownerRWXPerms); err != nil {
		return pos.Errorf("MkdirAll(): %w", err)
	}

	return nil
}

// A renderFS implementation that can inject errors for testing.
type ErrorFS struct {
	FS

	MkdirAllErr  error
	OpenErr      error
	OpenFileErr  error
	ReadFileErr  error
	RemoveAllErr error
	StatErr      error
	WriteFileErr error
}

func (e *ErrorFS) MkdirAll(name string, mode fs.FileMode) error {
	if e.MkdirAllErr != nil {
		return e.MkdirAllErr
	}
	return e.FS.MkdirAll(name, mode) //nolint:wrapcheck
}

func (e *ErrorFS) Open(name string) (fs.File, error) {
	if e.OpenErr != nil {
		return nil, e.OpenErr
	}
	return e.FS.Open(name) //nolint:wrapcheck
}

func (e *ErrorFS) OpenFile(name string, flag int, mode os.FileMode) (*os.File, error) {
	if e.OpenFileErr != nil {
		return nil, e.OpenFileErr
	}
	return e.FS.OpenFile(name, flag, mode) //nolint:wrapcheck
}

func (e *ErrorFS) ReadFile(name string) ([]byte, error) {
	if e.ReadFileErr != nil {
		return nil, e.ReadFileErr
	}
	return e.FS.ReadFile(name) //nolint:wrapcheck
}

func (e *ErrorFS) RemoveAll(name string) error {
	if e.RemoveAllErr != nil {
		return e.RemoveAllErr
	}
	return e.FS.RemoveAll(name) //nolint:wrapcheck
}

func (e *ErrorFS) Stat(name string) (fs.FileInfo, error) {
	if e.StatErr != nil {
		return nil, e.StatErr
	}
	return e.FS.Stat(name) //nolint:wrapcheck
}

func (e *ErrorFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	if e.WriteFileErr != nil {
		return e.WriteFileErr
	}
	return e.FS.WriteFile(name, data, perm) //nolint:wrapcheck
}
