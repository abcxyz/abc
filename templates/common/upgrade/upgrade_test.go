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
	"context"
	"path/filepath"
	"testing"

	"github.com/benbjohnson/clock"
	"github.com/google/go-cmp/cmp"

	"github.com/abcxyz/abc/templates/common"
	abctestutil "github.com/abcxyz/abc/templates/testutil"
	"github.com/abcxyz/pkg/testutil"
)

func TestUpgrade(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		manifestBaseName string
		gitIsDirty       bool
		// destDirContents and templateDirContents will both be placed in
		// subdirectories of a per-subtest temp directory. That temp directory
		// will be a git repo (to satisfy the "same workspace" checks).
		destDirContents     map[string]string
		templateDirContents map[string]string

		wantDestDirContentsAfter map[string]string
		wantErr                  string
	}{
		// TODO(upgrade): tests to add:
		//  dirhash unchanged
		//  file hash mismatches require conflict resolution
		//  added files/dirs
		//  removed files/dirs
		//  removed files/dirs, with local modifications
		//  remote git template
		//  manifest name is preserved
		//  dirty git workspace (gitIsDirty==true)
		//  extra inputs needed:
		//    inputs from file
		//    inputs provided as flags
		//    prompt for inputs

		{
			name: "simple_success",
			// cwd:          "installed_dir",
			// manifestPath: ".abc/manifest.yaml",
			manifestBaseName: "manifest_foo.lock.yaml",
			destDirContents: map[string]string{
				"out.txt": "hello",
				".abc/manifest_foo.lock.yaml": `
api_version: 'cli.abcxyz.dev/v1beta5'
kind: 'Manifest'
template_location: '../template_dir'
location_type: 'local_git'
template_dirhash: 1a2b3c
template_version: ""
inputs:
  - name: 'foo'
    value: 'bar'
output_hashes:
  - file: 'out.txt'
    hash: 'aaaaaabbbbbb'
`,
			},
			templateDirContents: map[string]string{
				"out.txt": "hello",
				"spec.yaml": `
api_version: 'cli.abcxyz.dev/v1beta5'
kind: 'Template'

desc: 'my template'

inputs:
  - name: 'foo'
    desc: 'an arbitrary input'

steps:
  - desc: 'include foo.txt'
    action: 'include'
    params:
      paths: ['out.txt']
  - desc: 'append hello to the file'
    action: 'append'
    params:
      paths: ['out.txt']
      with: ', world'
`,
			},
			wantDestDirContentsAfter: map[string]string{
				"out.txt": "hello, world\n",
				".abc/manifest_foo.lock.yaml": `# Generated by the "abc templates" command. Do not modify.
api_version: cli.abcxyz.dev/v1beta5
kind: Manifest
creation_time: 1970-01-01T00:00:00Z
modification_time: 1970-01-01T00:00:00Z
template_location: ../template_dir
location_type: local_git
template_version: 5597fc600ead69ad92c81a22b58c9e551cd86b9a
template_dirhash: h1:d3vExSz6l85xYUPxN2r4p2hq8OgeoOHEzGEhLH873HU=
inputs:
    - name: foo
      value: bar
output_hashes:
    - file: out.txt
      hash: h1:hT/5N2Kgbdv3IsTr6d3WbY9j3a6pf1IcPswg2nyXYCA=
`,
			},
		},

		// TODO(upgrade): many tests are needed
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempBase := t.TempDir()
			destDir := filepath.Join(tempBase, "dest_dir")
			manifestDir := filepath.Join(destDir, common.ABCInternalDir)
			templateDir := filepath.Join(tempBase, "template_dir")

			var stdoutBuf bytes.Buffer

			abctestutil.WriteAllDefaultMode(t, tempBase, abctestutil.WithGitRepoAt("", nil))
			abctestutil.WriteAllDefaultMode(t, destDir, tc.destDirContents)
			abctestutil.WriteAllDefaultMode(t, templateDir, tc.templateDirContents)

			ctx := context.Background()

			params := &Params{
				Clock:              clock.NewMock(),
				CWD:                destDir,
				FS:                 &common.RealFS{},
				ManifestPath:       filepath.Join(manifestDir, tc.manifestBaseName),
				Stdout:             &stdoutBuf,
				AllowDirtyTestOnly: true,
			}
			err := Upgrade(ctx, params)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}

			gotInstalledDirContentsAfter := abctestutil.LoadDirWithoutMode(t, destDir)
			if diff := cmp.Diff(gotInstalledDirContentsAfter, tc.wantDestDirContentsAfter); diff != "" {
				t.Errorf("installed directory contents after upgrading were not as expected (-got,+want): %s", diff)
			}
		})
	}
}
