// Copyright 2023 The Authors (see AUTHORS file)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package common contains the common utility functions for template commands.

package common

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// CmpFileMode is a cmp option that handles the conflict between Unix and
// Windows systems file permissions.
var CmpFileMode = cmp.Comparer(func(a, b fs.FileMode) bool {
	// Windows really only has 2 file modes: 0666 and 0444[1]. Thus we only check
	// the first bit, since we know the remaining bits will be the same.
	// Furthermore, there's no reliable way to know whether the executive bit is
	// set[2], so we ignore it.
	//
	// I tried doing fancy bitmasking stuff here, but apparently umasks on Windows
	// are undefined(?), so we've resorted to substring matching - hooray.
	//
	// [1]: https://medium.com/@MichalPristas/go-and-file-perms-on-windows-3c944d55dd44
	// [2]: https://github.com/golang/go/issues/41809
	if runtime.GOOS == "windows" {
		// A filemode of 0644 would show as "-rw-r--r--", but on Windows we only
		// care about the first bit (which is the first 3 characters in the output
		// string).
		return a.Perm().String()[1:3] == b.Perm().String()[1:3]
	}

	return a == b
})

type ModeAndContents struct {
	Mode     os.FileMode
	Contents string
}

// WriteAllDefaultMode wraps writeAll and sets file permissions to 0600.
func WriteAllDefaultMode(t *testing.T, root string, files map[string]string) {
	t.Helper()

	withMode := map[string]ModeAndContents{}
	for name, contents := range files {
		withMode[name] = ModeAndContents{
			Mode:     0o600,
			Contents: contents,
		}
	}
	WriteAll(t, root, withMode)
}

// WriteAll saves the given file contents with the given permissions.
func WriteAll(t *testing.T, root string, files map[string]ModeAndContents) {
	t.Helper()

	files = mapKeyFunc(filepath.FromSlash, files)

	for path, mc := range files {
		fullPath := filepath.Join(root, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("MkdirAll(%q): %v", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(mc.Contents), mc.Mode); err != nil {
			t.Fatalf("WriteFile(%q): %v", fullPath, err)
		}
		// The user's may have prevented the file from being created with the
		// desired permissions. Use chmod to really set the desired permissions.
		if err := os.Chmod(fullPath, mc.Mode); err != nil {
			t.Fatalf("Chmod(): %v", err)
		}
	}
}

// LoadDirContents reads all the files recursively under "dir", returning their contents as a
// map[filename]->contents. Returns nil if dir doesn't exist. Keys use slash separators, not
// native.
func LoadDirContents(t *testing.T, dir string) map[string]ModeAndContents {
	t.Helper()

	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		t.Fatal(err)
	}
	out := map[string]ModeAndContents{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("ReadFile(): %w", err)
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("Rel(): %w", err)
		}
		fi, err := d.Info()
		if err != nil {
			t.Fatal(err)
		}
		out[rel] = ModeAndContents{
			Mode:     fi.Mode(),
			Contents: string(contents),
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(): %v", err)
	}
	out = mapKeyFunc(filepath.ToSlash, out)
	return out
}

// Read all the files recursively under "dir", returning their contents as a
// map[filename]->contents but without file mode. Returns nil if dir doesn't
// exist. Keys use slash separators, not native.
func LoadDirWithoutMode(t *testing.T, dir string) map[string]string {
	t.Helper()

	withMode := LoadDirContents(t, dir)
	if withMode == nil {
		return nil
	}
	out := map[string]string{}
	for name, mc := range withMode {
		out[name] = mc.Contents
	}
	return out
}

// Return a copy of the input map where each key is transformed as f(key).
func mapKeyFunc[T any](f func(string) string, in map[string]T) map[string]T {
	out := make(map[string]T, len(in))
	for k, v := range in {
		out[f(k)] = v
	}
	return out
}
