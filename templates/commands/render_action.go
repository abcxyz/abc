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
	"io/fs"
	"os"
	"path/filepath"

	"github.com/abcxyz/abc/templates/model"
)

// Called with the contents of a file, and returns the new contents of the file
// to be written.
type walkAndModifyVisitor func([]byte) ([]byte, error)

// Recursively traverses the directory or file scratchDir/relPath, calling the
// given visitor for each file. If the visitor returns modified file contents
// for a given file, that file will be overwritten with the new contents.
func walkAndModify(pos *model.ConfigPos, rfs renderFS, scratchDir, relPath string, v walkAndModifyVisitor) error {
	if err := safeRelPath(pos, relPath); err != nil {
		return err
	}
	walkFrom := filepath.Join(scratchDir, relPath)
	if _, err := rfs.Stat(walkFrom); err != nil {
		if os.IsNotExist(err) {
			return model.ErrWithPos(pos, `path %q doesn't exist in the scratch directory, did you forget to "include" it first?"`, relPath) //nolint:wrapcheck
		}
		return model.ErrWithPos(pos, "Stat(): %w", err) //nolint:wrapcheck
	}

	return filepath.WalkDir(walkFrom, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// There was some filesystem error. Give up.
			return model.ErrWithPos(pos, "%w", err) //nolint:wrapcheck
		}
		if d.IsDir() {
			return nil
		}

		oldBuf, err := rfs.ReadFile(path)
		if err != nil {
			return model.ErrWithPos(pos, "Readfile(): %w", err) //nolint:wrapcheck
		}

		// We must clone oldBuf to guarantee that the callee won't change the
		// underlying bytes. We rely on an unmodified oldBuf below in the call
		// to bytes.Equal.
		newBuf, err := v(bytes.Clone(oldBuf))
		if err != nil {
			return err
		}

		if bytes.Equal(oldBuf, newBuf) {
			// If file contents are unchanged, there's no need to write.
			return nil
		}

		// The permissions in the following WriteFile call will be ignored
		// because the file already exists.
		if err := rfs.WriteFile(path, newBuf, ownerRWXPerms); err != nil {
			return model.ErrWithPos(pos, "Writefile(): %w", err) //nolint:wrapcheck
		}

		return nil
	})
}
