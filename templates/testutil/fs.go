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

// Package testutil contains util functions to facilitate tests.
package testutil

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	MinimalGitHeadSHA      = "5597fc600ead69ad92c81a22b58c9e551cd86b9a"
	MinimalGitHeadShortSHA = MinimalGitHeadSHA[:7]
)

type ModeAndContents struct {
	Mode     os.FileMode
	Contents string
}

// WriteAll wraps writeAll and sets file permissions to 0600.
func WriteAll(tb testing.TB, root string, files map[string]string) {
	tb.Helper()

	withMode := map[string]ModeAndContents{}
	for name, contents := range files {
		withMode[name] = ModeAndContents{
			Mode:     0o600,
			Contents: contents,
		}
	}
	WriteAllMode(tb, root, withMode)
}

// WriteAllMode saves the given file contents with the given permissions.
func WriteAllMode(tb testing.TB, root string, files map[string]ModeAndContents) {
	tb.Helper()

	for path, mc := range files {
		fullPath := filepath.Join(root, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			tb.Fatalf("MkdirAll(%q): %v", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(mc.Contents), mc.Mode); err != nil {
			tb.Fatalf("WriteFile(%q): %v", fullPath, err)
		}
		// The user's may have prevented the file from being created with the
		// desired permissions. Use chmod to really set the desired permissions.
		if err := os.Chmod(fullPath, mc.Mode); err != nil {
			tb.Fatalf("Chmod(): %v", err)
		}
	}
}

// LoadDirMode reads all the files recursively under "dir", returning their contents as a
// map[filename]->contents. Returns nil if dir doesn't exist. Keys use slash separators, not
// native.
func LoadDirMode(tb testing.TB, dir string, opts ...LoadDirOpt) map[string]ModeAndContents {
	tb.Helper()

	opt := combineLoadDirOpts(opts...)

	// We can't use common.Exists() here; that would cause an import cycle.
	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrInvalid) {
			return nil
		}
		tb.Fatal(err)
	}
	out := map[string]ModeAndContents{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			tb.Fatal(err)
		}
		if skippable(tb, relPath, opt.skipGlobs) {
			if d.IsDir() {
				return fs.SkipDir // skips traversing all children of this dir
			}
			return nil
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
			tb.Fatal(err)
		}
		out[rel] = ModeAndContents{
			Mode:     fi.Mode(),
			Contents: string(contents),
		}
		return nil
	})
	if err != nil {
		tb.Fatalf("WalkDir(): %v", err)
	}
	return out
}

func skippable(tb testing.TB, relPath string, globs []string) bool {
	tb.Helper()

	for _, glob := range globs {
		ok, err := filepath.Match(glob, relPath)
		if err != nil {
			tb.Fatalf("invalid glob: %s", glob)
		}
		if ok {
			return true
		}
	}
	return false
}

// LoadDir reads all the files recursively under "dir", returning their contents as a
// map[filename]->contents but without file mode. Returns nil if dir doesn't
// exist. Keys use slash separators, not native.
func LoadDir(tb testing.TB, dir string, opts ...LoadDirOpt) map[string]string {
	tb.Helper()

	withMode := LoadDirMode(tb, dir, opts...)
	if withMode == nil {
		return nil
	}
	out := map[string]string{}
	for name, mc := range withMode {
		out[name] = mc.Contents
	}
	return out
}

// LoadDirOpt is a "functional option" for the LoadDir* functions.
type LoadDirOpt struct {
	skipGlobs []string
}

func combineLoadDirOpts(opts ...LoadDirOpt) LoadDirOpt {
	var out LoadDirOpt
	for _, opt := range opts {
		out.skipGlobs = append(out.skipGlobs, opt.skipGlobs...)
	}
	return out
}

// SkipGlob is a LoadDirOpt that skips all directory entries matching the given
// glob (and their children, in the case of directories).
func SkipGlob(glob string) LoadDirOpt {
	return LoadDirOpt{
		skipGlobs: []string{glob},
	}
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
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}
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

func TestMustGlob(tb testing.TB, glob string) (string, bool) {
	tb.Helper()

	matches, err := filepath.Glob(glob)
	if err != nil {
		tb.Fatalf("couldn't find template directory: %v", err)
	}
	switch len(matches) {
	case 0:
		return "", false
	case 1:
		return matches[0], true
	}
	tb.Fatalf("got %d matches for glob %q, wanted 1: %s", len(matches), glob, matches)
	panic("unreachable") // silence compiler warning for "missing return"
}

// Overwrite writes the given contents to dir/baseName, failing the test on
// error.
func Overwrite(tb testing.TB, dir, baseName, contents string) {
	tb.Helper()
	filename := filepath.Join(dir, baseName)
	if err := os.WriteFile(filename, []byte(contents), 0o600); err != nil {
		tb.Fatal(err)
	}
}

// Prepends adds contents the beginning of the file dir/baseName, failing the
// test on error.
func Prepend(tb testing.TB, dir, baseName, contents string) {
	tb.Helper()

	filename := filepath.Join(dir, baseName)
	buf, err := os.ReadFile(filename)
	if err != nil {
		tb.Fatal(err)
	}
	Overwrite(tb, dir, baseName, contents+string(buf))
}

func Remove(tb testing.TB, dir, baseName string) {
	tb.Helper()

	filename := filepath.Join(dir, baseName)
	if err := os.Remove(filename); err != nil {
		tb.Fatal(err)
	}
}
