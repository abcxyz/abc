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

package tempdir

import (
	"context"
	"errors"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/pkg/logging"
)

// DirTracker helps manage the removal of temporary directories when
// rendering is finished.
type DirTracker struct {
	fs           common.FS
	tempDirs     []string
	keepTempDirs bool
}

// NewDirTracker constructs a TempDirRemover. Use this instead of creating
// a TempDirRemover yourself.
//
// keepTempDirs is like a no-op flag; it preserves the temp dirs for debugging
// rather than removing them.
func NewDirTracker(fs common.FS, keepTempDirs bool) *DirTracker {
	return &DirTracker{
		fs:           fs,
		keepTempDirs: keepTempDirs,
	}
}

// Track adds dir to the list of directories to remove.
func (t *DirTracker) Track(dir string) {
	if dir == "" {
		return
	}
	t.tempDirs = append(t.tempDirs, dir)
}

// MkdirTempTracked calls MkdirTemp and also tracks the resulting directory for
// later cleanup.
func (t *DirTracker) MkdirTempTracked(dir, pattern string) (string, error) {
	tempDir, err := t.fs.MkdirTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	t.Track(tempDir)
	return tempDir, nil
}

// DeferMaybeRemoveAll should be called in a defer to clean up temp dirs, like this:
//
//	defer t.DeferMaybeRemoveAll(ctx, &rErr)
func (t *DirTracker) DeferMaybeRemoveAll(ctx context.Context, outErr *error) {
	logger := logging.FromContext(ctx).With("logger", "tempDirRemover.Remove")
	if t.keepTempDirs {
		logger.WarnContext(ctx, "keeping temporary directories due to --keep-temp-dirs",
			"paths", t.tempDirs)
		return
	}

	logger.DebugContext(ctx, "removing all temporary directories (skip this with --keep-temp-dirs)")

	for _, p := range t.tempDirs {
		*outErr = errors.Join(*outErr, t.fs.RemoveAll(p))
	}
}
