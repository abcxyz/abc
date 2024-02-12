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

package common

import (
	"path/filepath"
	"strings"
)

// ABCInternalDir is the name for internal directories that have things like
// manifests and .gitkeep files. These may be destination directories or
// golden-test data/ directories.
const (
	ABCInternalDir    = ".abc"
	ABCInternalStdout = "stdout"
)

// IsReservedInDest returns true if the given path cannot be created in the
// destination directory because that name is reserved for internal purposes.
//
// The input path must use the local OS separators, since we process it with
// filepath. This path is relative to the destination directory.
func IsReservedInDest(relPath string) bool {
	clean := filepath.Clean(relPath)
	firstToken := strings.Split(clean, string(filepath.Separator))[0]
	return firstToken == ABCInternalDir
}

// IsReservedStdout returns true if the given path is the internal stdout path.
//
// The input path must use the local OS separators, since we process it with
// filepath. This path is relative to the destination directory.
func IsReservedStdout(relPath string) bool {
	clean := filepath.Clean(relPath)
	tokens := strings.Split(clean, string(filepath.Separator))
	if len(tokens) == 2 {
		return tokens[0] == ABCInternalDir && tokens[1] == ABCInternalStdout
	}
	return false
}
