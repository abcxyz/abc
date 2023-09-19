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
	"fmt"
	"os"
	"path/filepath"
)

type ModeAndContents struct {
	Mode     os.FileMode
	Contents string
}

// WriteAllDefaultMode wraps writeAll and sets file permissions to 0600.
func WriteAllDefaultMode(root string, files map[string]string) error {
	withMode := map[string]ModeAndContents{}
	for name, contents := range files {
		withMode[name] = ModeAndContents{
			Mode:     0o600,
			Contents: contents,
		}
	}
	return WriteAll(root, withMode)
}

// WriteAll saves the given file contents with the given permissions.
func WriteAll(root string, files map[string]ModeAndContents) error {
	for path, mc := range files {
		fullPath := filepath.Join(root, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("MkdirAll(%q): %w", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(mc.Contents), mc.Mode); err != nil {
			return fmt.Errorf("WriteFile(%q): %w", fullPath, err)
		}
		// The user's may have prevented the file from being created with the
		// desired permissions. Use chmod to really set the desired permissions.
		if err := os.Chmod(fullPath, mc.Mode); err != nil {
			return fmt.Errorf("Chmod(): %w", err)
		}
	}

	return nil
}
