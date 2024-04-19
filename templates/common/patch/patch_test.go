package patch

// --- a/dir1/file1.txt
// +++ b/dir2/file2.txt
// @@ -1 +1 @@
// -file1 contents
// +file2 contents

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseUnifiedDiff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    *Patch
		wantErr bool
	}{
		{
			name: "valid diff",
			input: `--- a/foo.txt
+++ b/foo.txt
@@ -1,3 +1,3 @@
-line 1
-line 2
-line 3
+line 1
+new line
+line 3`,
			want: &Patch{
				SourceName: "a/foo.txt",
				DestName:   "b/foo.txt",
				Hunks: []Hunk{
					{
						SourceLineStart: 1,
						SourceLength:    3,
						DestLineStart:   1,
						DestLength:      3,
						Actions: []Action{
							{
								AddOrRemove: Remove,
								Line:        "line 1",
							},
							{
								AddOrRemove: Remove,
								Line:        "line 2",
							},
							{
								AddOrRemove: Remove,
								Line:        "line 3",
							},
							{
								AddOrRemove: Add,
								Line:        "line 1",
							},
							{
								AddOrRemove: Add,
								Line:        "new line",
							},
							{
								AddOrRemove: Add,
								Line:        "line 3",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid diff (missing metadata)",
			input: `@@ -1,3 +1,3 @@
-line 1
-line 2
-line 3
+line 1
+new line
+line 3`,
			want:    nil,
			wantErr: true,
		},
		{
			name: "invalid diff (invalid hunk header)",
			input: `--- a/foo.txt
+++ b/foo.txt
@@ -1,3 +1,3
-line 1
-line 2
-line 3
+line 1
+new line
+line 3`,
			want:    nil,
			wantErr: true,
		},
		{
			name: "invalid diff (invalid action)",
			input: `--- a/foo.txt
+++ b/foo.txt
@@ -1,3 +1,3 @@
-line 1
-line 2
-line 3
+line 1
+new line
+line 3
+invalid action`,
			want:    nil,
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseUnifiedDiff(strings.Split(test.input, "\n"))
			if (err != nil) != test.wantErr {
				t.Errorf("ParseUnifiedDiff() error = %v, wantErr %v", err, test.wantErr)
				return
			}

			if diff := cmp.Diff(got, test.want); diff != "" {
				t.Errorf("patch was not as expected:\n%s", diff)
			}
			// if !test.wantErr && !comparePatches(got, test.want) {
			// 	t.Errorf("ParseUnifiedDiff() = %v, want %v", got, test.want)
			// }
		})
	}
}

// func comparePatches(got, want *Patch) bool {
// 	if got.SourceName != want.SourceName || got.DestName != want.DestName {
// 		return false
// 	}
// 	if len(got.Hunks) != len(want.Hunks) {
// 		return false
// 	}
// 	for i := range got.Hunks {
// 		if !compareHunks(&got.Hunks[i], &want.Hunks[i]) {
// 			return false
// 		}
// 	}
// 	return true
// }

// func compareHunks(got, want *Hunk) bool {
// 	if got.SourceLineStart != want.SourceLineStart || got.SourceLength != want.SourceLength ||
// 		got.DestLineStart != want.DestLineStart || got.DestLength != want.DestLength {
// 		return false
// 	}
// 	if len(got.Actions) != len(want.Actions) {
// 		return false
// 	}
// 	for i := range got.Actions {
// 		if got.Actions[i].AddOrRemove != want.Actions[i].AddOrRemove || got.Actions[i].Line != want.Actions[i].Line {
// 			return false
// 		}
// 	}
// 	return true
// }
