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
	"encoding/hex"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// A minimal but actually valid .git directory that allows running real git
// commands. This was created by doing:
//   - git init myrepo && cd myrepo
//   - git commit --allow-empty -m 'Initial commit'
//   - dump the contents of .git, ignoring all files other than those
//     below, because they're not strictly necessary.
var (
	minimalGitRepoFiles = map[string]string{
		".git/refs/heads/main": "5597fc600ead69ad92c81a22b58c9e551cd86b9a",
		".git/objects/4b/825dc642cb6eb9a060e54bf8d69288fbee4904": string(mustHexDecode("0178292b4d4a305500600a00022c0001")),
		".git/objects/55/97fc600ead69ad92c81a22b58c9e551cd86b9a": string(mustHexDecode("78019d914d739b3010867bd6afd03d9354603e67d24cc0c802078bd8981873032104359806833ffaeb6bec53333df59dd1eceaddd9d5332bd6364dd54355d7bff51de750c90c59cd99a6c82cd37866a648435c55b2c2c83553368c22e35c319102d2a12fdb0e3ae991c3153ff2ba86cfdd2dbe8ab615357f626df302251dc9aaa99b86021f918110b8bad7077bfe1fade2973854023e8eb231f1280c4317861ea1d63a5ae19b0f6024512f42b56b8d5a5e0f1163b64e3674c764dc33b92c179534a43145ccf1247a2b977fdf318018fbce7c88f1bea352a11dd06c57bba22c4a69714a97796bbbbf4fca776d9b4ec874b3db063f07c50fd1d4cf2717711e3ccce28e27db1ac06563485e22375d1318ec3370d969c4a1fb341ee1eeb2f7b4ddaea3f36264218bd1fcc20be017627cc316a1f941cefa8cc9c99a56d3f9e72e0e8a120b7d535fb2617e5e09e437b3556e3d5cf038550038b536d2c12dfc217a1f2ece2c1fde6c8d90d275ec846312906515549363f7a0f6d60f705f36a6cebf560dbc7dd557690def7f0afe007d0bb2a9")),
		".git/HEAD": "ref: refs/heads/main",
	}
	// This is the SHA of the only commit in the repo above.
	MinimalGitHeadSHA = "5597fc600ead69ad92c81a22b58c9e551cd86b9a"
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
		if IsStatNotExistErr(err) {
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

// WithGitRepoAt adds "files" to the given map containing a minimal git repo.
// The prefix will be added to the beginning of each filename (e.g. "subdir/").
// Returns the input map for ease of call chaining.
//
// Any keys in the input map will not be overwritten. This allows tests to
// override certain files, say, .git/refs/main.
//
// This is intended to be used with maps that will eventually be passed to
// WriteAllDefaultMode().
func WithGitRepoAt(prefix string, m map[string]string) map[string]string {
	out := maps.Clone(m) // to be safe, don't mutate the input map.
	if out == nil {
		out = make(map[string]string, len(minimalGitRepoFiles))
	}
	for k, v := range minimalGitRepoFiles {
		newKey := prefix + k
		if _, ok := out[newKey]; ok {
			continue // don't overwrite existing entries
		}
		out[newKey] = v
	}
	return out
}

func mustHexDecode(s string) []byte {
	out, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return out
}
