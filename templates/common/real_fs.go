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
	"io/fs"
	"os"
)

// RealFS is the non-test implementation of the filesystem interface.
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

func (r *RealFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(name) //nolint:wrapcheck
}
