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

package run

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	abctestutil "github.com/abcxyz/abc/templates/testutil"
)

func TestDiff(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		dirContents map[string]string
		color       bool
		skip        bool
		file1       string
		file1RelTo  string
		file2       string
		file2RelTo  string
		want        string
	}{
		{
			name: "both_empty",
			dirContents: map[string]string{
				"file1.txt": "",
				"file2.txt": "",
			},
			file1:      "file1.txt",
			file1RelTo: ".",
			file2:      "file2.txt",
			file2RelTo: ".",
			want:       "",
		},
		{
			name:        "both_missing",
			dirContents: map[string]string{},
			file1:       "file1.txt",
			file1RelTo:  ".",
			file2:       "file2.txt",
			file2RelTo:  ".",
			want:        "",
		},
		{
			name: "one_missing",
			dirContents: map[string]string{
				"file1.txt": "file1 contents\n",
			},
			file1:      "file1.txt",
			file1RelTo: ".",
			file2:      "file2.txt",
			file2RelTo: ".",
			want: `--- a/file1.txt
+++ b/file2.txt
@@ -1 +0,0 @@
-file1 contents
`,
		},
		{
			name: "files_same",
			dirContents: map[string]string{
				"file1.txt": "contents\n",
				"file2.txt": "contents\n",
			},
			file1:      "file1.txt",
			file1RelTo: ".",
			file2:      "file2.txt",
			file2RelTo: ".",
			want:       "",
		},
		{
			name: "files_differ",
			dirContents: map[string]string{
				"file1.txt": "file1 contents\n",
				"file2.txt": "file2 contents\n",
			},
			file1:      "file1.txt",
			file1RelTo: ".",
			file2:      "file2.txt",
			file2RelTo: ".",
			want: `--- a/file1.txt
+++ b/file2.txt
@@ -1 +1 @@
-file1 contents
+file2 contents
`,
		},
		{
			name:  "files_differ_with_color_on_machine_with_color_support",
			color: true,
			skip: func() bool {
				hasColor, err := diffColorSupported(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				return !hasColor
			}(),
			dirContents: map[string]string{
				"file1.txt": "file1 contents\n",
				"file2.txt": "file2 contents\n",
			},
			file1:      "file1.txt",
			file1RelTo: ".",
			file2:      "file2.txt",
			file2RelTo: ".",
			want:       "\x1b[1m--- a/file1.txt\x1b[0m\n\x1b[1m+++ b/file2.txt\x1b[0m\n\x1b[36m@@ -1 +1 @@\x1b[0m\n\x1b[31m-file1 contents\x1b[0m\n\x1b[32m+file2 contents\x1b[0m\n",
		},
		{
			name:  "files_differ_with_color_on_machine_without_color_support",
			color: false,
			skip: func() bool {
				hasColor, err := diffColorSupported(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				return hasColor
			}(),
			dirContents: map[string]string{
				"file1.txt": "file1 contents\n",
				"file2.txt": "file2 contents\n",
			},
			file1:      "file1.txt",
			file1RelTo: ".",
			file2:      "file2.txt",
			file2RelTo: ".",
			want:       "--- a/file1.txt\n+++ b/file2.txt\n@@ -1 +1 @@\n-file1 contents\n+file2 contents\n",
		},
		{
			name: "relative_to_different_subdirs",
			dirContents: map[string]string{
				"dir1/file1.txt": "file1 contents\n",
				"dir2/file2.txt": "file2 contents\n",
			},
			file1:      "dir1/file1.txt",
			file1RelTo: "dir1",
			file2:      "dir2/file2.txt",
			file2RelTo: "dir2",
			want: `--- a/file1.txt
+++ b/file2.txt
@@ -1 +1 @@
-file1 contents
+file2 contents
`,
		},
		{
			name: "in_subdirs_relative_to_root",
			dirContents: map[string]string{
				"dir1/file1.txt": "file1 contents\n",
				"dir2/file2.txt": "file2 contents\n",
			},
			file1:      "dir1/file1.txt",
			file1RelTo: ".",
			file2:      "dir2/file2.txt",
			file2RelTo: ".",
			want: `--- a/dir1/file1.txt
+++ b/dir2/file2.txt
@@ -1 +1 @@
-file1 contents
+file2 contents
`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if tc.skip {
				t.Skip("presence or absence of diff color support on this machine requires skipping this subtest")
			}
			tempDir := t.TempDir()
			abctestutil.WriteAll(t, tempDir, tc.dirContents)

			abs := func(rel string) string {
				return filepath.Join(tempDir, rel)
			}

			gotDiff, err := RunDiff(context.Background(), tc.color, abs(tc.file1), abs(tc.file1RelTo), abs(tc.file2), abs(tc.file2RelTo))
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(gotDiff, tc.want); diff != "" {
				t.Errorf("diff was not as expected (-got,+want): %s", diff)
			}
		})
	}
}
