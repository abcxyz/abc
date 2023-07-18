package testutil

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

type ModeAndContents struct {
	Mode     os.FileMode
	Contents string
}

// LoadDirContents reads all the files recursively under "dir", returning their
// contents as a map[filename]->contents. Returns nil if dir doesn't exist.
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
	return out
}

// LoadDirWithoutMode is like LoadDirContents, but doesn't return permission
// bits.
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

// WriteAllDefaultMode wraps writeAll and sets file permissions to 0600.
func WriteAllDefaultMode(t *testing.T, root string, files map[string]string) {
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
	for path, mc := range files {
		fullPath := filepath.Join(root, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(mc.Contents), mc.Mode); err != nil {
			t.Fatal(err)
		}
		// The user's may have prevented the file from being created with the
		// desired permissions. Use chmod to really set the desired permissions.
		if err := os.Chmod(fullPath, mc.Mode); err != nil {
			t.Fatal(err)
		}
	}
}