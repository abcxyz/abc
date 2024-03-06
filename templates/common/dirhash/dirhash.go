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

package dirhash

import (
	"fmt"
	"strings"

	"golang.org/x/mod/sumdb/dirhash"
)

var latestHash = dirhash.Hash1

// HashLatest computes a dirhash of the given directory using the latest/best
// hash algorithm.
func HashLatest(dir string) (string, error) {
	out, err := dirhash.HashDir(dir, "", latestHash)
	if err != nil {
		return "", fmt.Errorf("dirhash.HashDir: %w", err)
	}

	return out, nil
}

// Verify returns whether the dirhash of the given directory matches the given
// hash value. It detects which hash algorithm to use based on a prefix of
// wantHash, e.g. "h1:0a1b2d3c..."
func Verify(wantHash string, dir string) (bool, error) {
	// The hash should start with a string like "h1:" indicating the hash algorithm
	tokens := strings.SplitN(wantHash, ":", 2)
	if len(tokens) != 2 {
		return false, fmt.Errorf("malformed hash, expected it to begin with hash name followed by colon: %q", wantHash)
	}

	var hash dirhash.Hash
	switch tokens[0] {
	// We could theoretically add other hash algorithms in the future if needed.
	case "h1":
		hash = dirhash.Hash1
	default:
		return false, fmt.Errorf("unknown hash algorithm %q", tokens[0])
	}

	gotHash, err := dirhash.HashDir(dir, "", hash)
	if err != nil {
		return false, fmt.Errorf("dirhash.HashDir: %w", err)
	}

	return gotHash == wantHash, nil
}
