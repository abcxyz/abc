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

package upgrade

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"hash"
	"io"
	"os"
	"strings"

	"github.com/abcxyz/abc/templates/common"
)

// hashResult is the result from hashAndCompare().
type hashResult string

const (
	// match means "the file contents were hashed, and the value of the hash
	// matched the expected value".
	match hashResult = "hash_matched"

	// mismatch means "the file contents were hashed, and the value of the
	// hash didn't match the expected value".
	mismatch hashResult = "edited"

	// absent means "the file contents couldn't be hashed because the file
	// doesn't exist".
	absent hashResult = "deleted"
)

// hashAndCompare extracts the hash algorithm (e.g. "h1:" from wantHash, then
// hashes the given path with that algorithm.
func hashAndCompare(path, wantHash string) (hashResult, error) {
	// The hash should start with a string like "h1:" indicating the hash algorithm
	tokens := strings.SplitN(wantHash, ":", 2)
	if len(tokens) != 2 {
		return "", fmt.Errorf("malformed hash, expected it to begin with hash name followed by colon: %q", wantHash)
	}

	var hasher hash.Hash
	var wantHashUnmarshaled []byte
	switch tokens[0] {
	case "h1":
		hasher = sha256.New()
		var err error
		wantHashUnmarshaled, err = base64.StdEncoding.DecodeString(tokens[1])
		if err != nil {
			return "", fmt.Errorf("failed unmarshaling hash %q as base64: %w", tokens[1], err)
		}
	default:
		return "", fmt.Errorf("unknown hash algorithm %q", tokens[0])
	}

	inFile, err := os.Open(path)
	if err != nil {
		if common.IsStatNotExistErr(err) {
			return absent, nil
		}
		return "", fmt.Errorf("Open(%q): %w", path, err)
	}
	if _, err := io.Copy(hasher, inFile); err != nil {
		return "", fmt.Errorf("Copy(): %w", err)
	}
	gotHash := hasher.Sum(nil)

	if !bytes.Equal(gotHash, wantHashUnmarshaled) {
		return mismatch, nil
	}

	return match, nil
}
