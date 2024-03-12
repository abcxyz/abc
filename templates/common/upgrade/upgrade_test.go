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
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/render"
	"github.com/abcxyz/abc/templates/common/templatesource"
	"github.com/abcxyz/abc/templates/model"
	manifest "github.com/abcxyz/abc/templates/model/manifest/v1alpha1"
	abctestutil "github.com/abcxyz/abc/templates/testutil"
	mdl "github.com/abcxyz/abc/templates/testutil/model"
	"github.com/abcxyz/pkg/testutil"
)

func TestUpgrade(t *testing.T) {
	t.Parallel()

	// We don't use UTC time here because we want to make sure local time
	// gets converted to UTC time before saving.
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("time.LoadLocation(): %v", err)
	}
	beforeUpgradeTime := time.Date(2024, 3, 1, 4, 5, 6, 7, loc)
	afterUpgradeTime := beforeUpgradeTime.Add(time.Hour)

	outTxtSpec := `
api_version: 'cli.abcxyz.dev/v1beta6'
kind: 'Template'

desc: 'my template'

steps:
  - desc: 'include out.txt'
    action: 'include'
    params:
      paths: ['out.txt']
`

	outTxtOnlyManifest := &manifest.Manifest{
		CreationTime:     beforeUpgradeTime.UTC(),
		ModificationTime: beforeUpgradeTime.UTC(),
		TemplateLocation: mdl.S("../template_dir"),
		TemplateVersion:  mdl.S(abctestutil.MinimalGitHeadSHA),
		LocationType:     mdl.S("local_git"),
		Inputs:           []*manifest.Input{},
		OutputHashes: []*manifest.OutputHash{
			{
				File: mdl.S("out.txt"),
			},
		},
	}

	cases := []struct {
		name string

		// We're doing an `abc templates render` followed by an `abc
		// templates upgrade`. Then, the files in
		// templateChangesForUpgrade are added to the template before executing
		// the upgrade operation.

		// origTemplateDirContents is used as the template for the initial
		// render operation.
		origTemplateDirContents map[string]string

		// Only one of templateUnionForUpgrade or templateReplacementForUpgrade
		// may be set.

		// templateUnionForUpgrade is a set of files that will be dropped into
		// the template directory, thus creating the new upgraded template
		// version. Is is convenient to use this instead of
		// templateReplacementForUpgrade in the case where you just want to add
		// files without removing any.
		templateUnionForUpgrade map[string]string
		// templateReplacementForUpgrade will be used as the full template
		// contents when upgrade (unlike templateUnionForUpgrade, which is a
		// delta).
		templateReplacementForUpgrade map[string]string

		wantDestContentsBeforeUpgrade map[string]string // excludes manifest contents
		wantManifestBeforeUpgrade     *manifest.Manifest
		wantDestContentsAfterUpgrade  map[string]string // excludes manifest contents
		wantManifestAfterUpgrade      *manifest.Manifest
		wantOK                        bool
		wantErr                       string
	}{
		// TODO(upgrade): tests to add:
		//  All merge cases
		//  remote git template
		//  manifest name is preserved
		//  extra inputs needed:
		//    inputs from file
		//    inputs provided as flags
		//    prompt for inputs

		{
			name: "new_template_has_updated_file_without_local_edits",
			origTemplateDirContents: map[string]string{
				"out.txt":   "hello\n",
				"spec.yaml": outTxtSpec,
			},
			wantOK: true,
			wantDestContentsBeforeUpgrade: map[string]string{
				"out.txt": "hello\n",
			},
			wantManifestBeforeUpgrade: outTxtOnlyManifest,
			templateUnionForUpgrade: map[string]string{
				"spec.yaml": outTxtSpec + `
  - desc: 'append ", world"" to the file'
    action: 'append'
    params:
      paths: ['out.txt']
      with: 'world'`,
			},
			wantDestContentsAfterUpgrade: map[string]string{
				"out.txt": "hello\nworld\n",
			},
			wantManifestAfterUpgrade: &manifest.Manifest{
				CreationTime:     beforeUpgradeTime.UTC(),
				ModificationTime: afterUpgradeTime.UTC(),
				TemplateLocation: mdl.S("../template_dir"),
				TemplateVersion:  mdl.S(abctestutil.MinimalGitHeadSHA),
				LocationType:     mdl.S("local_git"),
				Inputs:           []*manifest.Input{},
				OutputHashes: []*manifest.OutputHash{
					{
						File: mdl.S("out.txt"),
					},
				},
			},
		},
		{
			name:   "short_circuit_if_already_latest_version",
			wantOK: false,
			origTemplateDirContents: map[string]string{
				"out.txt":   "hello\n",
				"spec.yaml": outTxtSpec,
			},
			templateUnionForUpgrade: map[string]string{},
			wantDestContentsBeforeUpgrade: map[string]string{
				"out.txt": "hello\n",
			},
			wantManifestBeforeUpgrade: outTxtOnlyManifest,
			wantDestContentsAfterUpgrade: map[string]string{
				"out.txt": "hello\n",
			},
			wantManifestAfterUpgrade: outTxtOnlyManifest,
		},
		{
			name:   "new_template_has_file_not_in_old_template",
			wantOK: true,
			origTemplateDirContents: map[string]string{
				"out.txt":   "hello\n",
				"spec.yaml": outTxtSpec,
			},
			wantDestContentsBeforeUpgrade: map[string]string{
				"out.txt": "hello\n",
			},
			wantManifestBeforeUpgrade: outTxtOnlyManifest,
			templateUnionForUpgrade: map[string]string{
				"another_file.txt": "I'm another file\n",
				"spec.yaml": outTxtSpec + `
  - desc: 'include another_file.txt'
    action: 'include'
    params:
      paths: ['another_file.txt']`,
			},
			wantDestContentsAfterUpgrade: map[string]string{
				"out.txt":          "hello\n",
				"another_file.txt": "I'm another file\n",
			},
			wantManifestAfterUpgrade: &manifest.Manifest{
				CreationTime:     beforeUpgradeTime.UTC(),
				ModificationTime: afterUpgradeTime.UTC(),
				TemplateLocation: mdl.S("../template_dir"),
				TemplateVersion:  mdl.S(abctestutil.MinimalGitHeadSHA),
				LocationType:     mdl.S("local_git"),
				Inputs:           []*manifest.Input{},
				OutputHashes: []*manifest.OutputHash{
					{
						File: mdl.S("another_file.txt"),
					},
					{
						File: mdl.S("out.txt"),
					},
				},
			},
		},

		// 		{
		// 			name: "old_template_has_file_not_in_new_template_with_no_local_edits",
		// 			wantOK: true,
		// 			origTemplateDirContents: map[string]string{
		// 				"out.txt":   "hello\n",
		// 				"another_file.txt":   "I'm another file\n",
		// 				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta6'
		// kind: 'Template'

		// desc: 'my template'

		// steps:
		//   - desc: 'include files'
		//     action: 'include'
		//     params:
		//       paths: ['.']`,
		// 			},
		// 			wantDestContentsBeforeUpgrade: map[string]string{
		// 				"out.txt": "hello\n",
		// 			},
		// 			wantManifestBeforeUpgrade: outTxtOnlyManifest,
		// 			templateChangesForUpgrade: map[string]string{
		// 				"another_file.txt": "I'm another file\n",
		// 				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta6'
		// kind: 'Template'

		// desc: 'my template'

		// steps:
		//   - desc: 'include out.txt'
		//     action: 'include'
		//     params:
		//       paths: ['out.txt']`,
		// 			},
		// 		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempBase := t.TempDir()
			destDir := filepath.Join(tempBase, "dest_dir")
			manifestDir := filepath.Join(destDir, common.ABCInternalDir)
			templateDir := filepath.Join(tempBase, "template_dir")

			// Make tempBase into a valid git repo.
			abctestutil.WriteAllDefaultMode(t, tempBase, abctestutil.WithGitRepoAt("", nil))

			ctx := context.Background()

			abctestutil.WriteAllDefaultMode(t, templateDir, tc.origTemplateDirContents)
			clk := clock.NewMock()
			clk.Set(beforeUpgradeTime)
			renderAndVerify(t, ctx, clk, tempBase, templateDir, destDir, tc.wantDestContentsBeforeUpgrade)

			assertManifest(ctx, t, "before upgrade", tc.wantManifestBeforeUpgrade, manifestDir)

			clk.Set(afterUpgradeTime) // simulate time passing between initial installation and upgrade

			manifestBaseName, err := findManifest(manifestDir)
			if err != nil {
				t.Fatal(err)
			}

			params := &Params{
				Clock:              clk,
				CWD:                destDir,
				FS:                 &common.RealFS{},
				ManifestPath:       filepath.Join(manifestDir, manifestBaseName),
				Stdout:             os.Stdout,
				AllowDirtyTestOnly: true,
			}

			// Create the new template version that we'll upgrade to, in
			// templateDir.
			if len(tc.templateUnionForUpgrade) > 0 && len(tc.templateReplacementForUpgrade) > 0 {
				t.Fatal("test config bug: only one of templateUnionForUpgrade or templateReplacementForUpgrade should be set")
			}
			if len(tc.templateUnionForUpgrade) > 0 {
				abctestutil.WriteAllDefaultMode(t, templateDir, tc.templateUnionForUpgrade)
			}
			if len(tc.templateReplacementForUpgrade) > 0 {
				
				if err := os.RemoveAll(templateDir); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(templateDir, common.OwnerRWXPerms); err != nil {
					t.Fatal(err)
				}
				abctestutil.WriteAllDefaultMode(t, templateDir, tc.templateReplacementForUpgrade)
			}

			ok, err := Upgrade(ctx, params)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}

			if ok != tc.wantOK {
				t.Errorf("got ok=%t, want %t", ok, tc.wantOK)
			}

			assertManifest(ctx, t, "after upgrade", tc.wantManifestAfterUpgrade, manifestDir)

			gotInstalledDirContentsAfter := abctestutil.LoadDirWithoutMode(t, destDir, abctestutil.SkipGlob(".abc/manifest*"))
			if diff := cmp.Diff(gotInstalledDirContentsAfter, tc.wantDestContentsAfterUpgrade); diff != "" {
				t.Errorf("installed directory contents after upgrading were not as expected (-got,+want): %s", diff)
			}
		})
	}
}

func assertManifest(ctx context.Context, tb testing.TB, whereAreWe string, want *manifest.Manifest, abcDir string) {
	baseName, err := findManifest(abcDir)
	if err != nil {
		tb.Fatal(err)
	}

	got, err := loadManifest(ctx, &common.RealFS{}, filepath.Join(abcDir, baseName))
	if err != nil {
		tb.Fatal(err)
	}

	opts := []cmp.Option{
		// Don't force test authors to assert the line and column numbers
		cmpopts.IgnoreTypes(&model.ConfigPos{}, model.ConfigPos{}),

		// Don't force test author to compute hashes when writing test/updating test cases.
		cmpopts.IgnoreFields(manifest.Manifest{}, "TemplateDirhash"),
		cmpopts.IgnoreFields(manifest.OutputHash{}, "Hash"),
	}
	if diff := cmp.Diff(got, want, opts...); diff != "" {
		tb.Errorf("at %q, manifest was not as expected (-got,+want): %s", whereAreWe, diff)
	}

	// We omitted these fields from the Diff(), but make sure they look sane.
	const minHashLen = 10 // arbitrarily picked, anything shorter isn't a sane hash
	if len(got.TemplateDirhash.Val) < minHashLen {
		tb.Errorf("dirhash %q is too short", got.TemplateDirhash.Val)
	}
	for _, oh := range got.OutputHashes {
		if len(oh.Hash.Val) < minHashLen {
			tb.Errorf("output hash %q for file %q is too short", oh.Hash.Val, oh.File.Val)
		}
	}
}

func renderAndVerify(tb testing.TB, ctx context.Context, clk clock.Clock, tempBase, templateDir, destDir string, wantContents map[string]string) {
	tb.Helper()

	downloader, err := templatesource.ParseSource(ctx, &templatesource.ParseSourceParams{
		AllowDirtyTestOnly: true,
		CWD:                tempBase,
		Source:             templateDir,
	})
	if err != nil {
		tb.Fatal(err)
	}

	if err := render.Render(ctx, &render.Params{
		Clock:       clk,
		Cwd:         tempBase,
		DestDir:     destDir,
		Downloader:  downloader,
		FS:          &common.RealFS{},
		Manifest:    true,
		OutDir:      destDir,
		TempDirBase: tempBase,
	}); err != nil {
		tb.Fatal(err)
	}

	got := abctestutil.LoadDirWithoutMode(tb, destDir, abctestutil.SkipGlob(".abc/manifest*"))
	if diff := cmp.Diff(got, wantContents); diff != "" {
		tb.Fatalf("installed directory contents before upgrading were not as expected, there's something wrong with test setup (-got,+want): %s", diff)
	}
}
