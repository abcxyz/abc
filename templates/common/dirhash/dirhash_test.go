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
	"path/filepath"
	"testing"

	abctestutil "github.com/abcxyz/abc/templates/testutil"
	"github.com/abcxyz/pkg/testutil"
)

func TestHashLatest(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		subdir  string
		files   map[string]string
		want    string
		wantErr string
	}{
		{
			name: "simple_success",
			files: map[string]string{
				"a.txt":    "hello",
				"b/c.yaml": "foo: bar",
			},
			want: "h1:QDmRYeMVG4rHN0RWwV7vqJxksmtiHI+JHBKeBPJUd1U=",
		},
		{
			name:    "nonexistent_dir",
			subdir:  "no_such_dir",
			files:   map[string]string{},
			wantErr: "no such file or directory",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()
			abctestutil.WriteAllDefaultMode(t, tempDir, tc.files)
			got, err := HashLatest(filepath.Join(tempDir, tc.subdir))
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}
			if got != tc.want {
				t.Errorf("HashLatest()=%q, want %q", got, tc.want)
			}
		})
	}
}

func TestVerify(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		files         map[string]string
		compareToHash string
		subdir        string
		want          bool
		wantErr       string
	}{
		{
			name: "match",
			files: map[string]string{
				"a.txt":    "hello",
				"b/c.yaml": "foo: bar",
			},
			compareToHash: "h1:QDmRYeMVG4rHN0RWwV7vqJxksmtiHI+JHBKeBPJUd1U=",
			want:          true,
		},
		{
			name: "mismatch",
			files: map[string]string{
				"a.txt":    "hello",
				"b/c.yaml": "foo: bar",
			},
			compareToHash: "h1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa=",
			want:          false,
		},
		{
			name: "filesystem_error",
			files: map[string]string{
				"a.txt":    "hello",
				"b/c.yaml": "foo: bar",
			},
			subdir:        "nonexistent",
			compareToHash: "h1:whatever",
			wantErr:       "no such file or directory",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()
			abctestutil.WriteAllDefaultMode(t, tempDir, tc.files)
			got, err := Verify(tc.compareToHash, filepath.Join(tempDir, tc.subdir))
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}
			if got != tc.want {
				t.Errorf("Verify()=%t, want %t", got, tc.want)
			}
		})
	}
}
