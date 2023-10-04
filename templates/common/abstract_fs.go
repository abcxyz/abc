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
type AbstractFS interface {
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
	BackupDirMaker func(AbstractFS) (string, error)
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
	// rfs is the filesytem to use.
	Rfs AbstractFS
	// visitor is an optional function that will be called for each file in the
	// source, to allow customization of the copy operation on a per-file basis.
	Visitor CopyVisitor
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

	return fs.WalkDir(p.Rfs, p.SrcRoot, func(path string, de fs.DirEntry, err error) error { //nolint:wrapcheck
		if err != nil {
			return err // There was some filesystem error. Give up.
		}
		// We don't have to worry about symlinks here because we passed
		// DisableSymlinks=true to go-getter.
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
		if err := mkdirAllChecked(pos, p.Rfs, inDir, p.DryRun); err != nil {
			return err
		}
		dstInfo, err := p.Rfs.Stat(dst)
		if err == nil {
			if dstInfo.IsDir() {
				return pos.Errorf("cannot overwrite a directory with a file of the same name, %q", relToSrc)
			}
			if !ch.Overwrite {
				return pos.Errorf("destination file %s already exists and overwriting was not enabled with --force-overwrite", relToSrc)
			}
			if ch.BackupIfExists && !p.DryRun {
				if backupDir == "" {
					if backupDir, err = p.BackupDirMaker(p.Rfs); err != nil {
						return fmt.Errorf("failed making backup directory: %w", err)
					}
				}
				if err := backUp(ctx, p.Rfs, backupDir, p.DstRoot, relToSrc); err != nil {
					return err
				}
			}
		} else if !os.IsNotExist(err) {
			return pos.Errorf("Stat(): %w", err)
		}
		srcInfo, err := p.Rfs.Stat(path)
		if err != nil {
			return fmt.Errorf("Stat(): %w", err)
		}

		// The permission bits on the output file are copied from the input file;
		// this preserves the execute bit on executable files.
		mode := srcInfo.Mode().Perm()
		return copyFile(ctx, pos, p.Rfs, path, dst, mode, p.DryRun)
	})
}

func copyFile(ctx context.Context, pos *model.ConfigPos, rfs AbstractFS, src, dst string, mode fs.FileMode, dryRun bool) (outErr error) {
	logger := logging.FromContext(ctx).With("logger", "copyFile")

	readFile, err := rfs.Open(src)
	if err != nil {
		return pos.Errorf("Open(): %w", err)
	}
	defer func() { outErr = errors.Join(outErr, readFile.Close()) }()

	if dryRun {
		return nil
	}

	writeFile, err := rfs.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return pos.Errorf("OpenFile(): %w", err)
	}
	defer func() { outErr = errors.Join(outErr, writeFile.Close()) }()

	if _, err := io.Copy(writeFile, readFile); err != nil {
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
func backUp(ctx context.Context, rfs AbstractFS, backupDir, srcRoot, relPath string) error {
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
	logger.DebugContext(ctx, "completed backup", "source", fileToBackup, "destination", backupFile)

	return nil
}

// A fancy wrapper around MkdirAll with better error messages and a dry run
// mode. In dry run mode, returns an error if the MkdirAll wouldn't succeed
// (best-effort).
func mkdirAllChecked(pos *model.ConfigPos, rfs AbstractFS, path string, dryRun bool) error {
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
