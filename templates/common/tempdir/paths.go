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

package tempdir

const (
	// These will be used as part of the names of the temporary directories to
	// make them identifiable.

	// The directory that contains a diff for each template rendering step to
	// help with template debugging. Must be enabled by command line flag.
	DebugStepDiffsDirNamePart = "debug-step-diffs-"

	// The temp directory where the "golden-test verify" command writes the "got"
	// output before comparing to the "wanted" output.
	GoldenTestRenderNamePart = "golden-test-"

	// The temp directory where templates perform their actions and "include"
	// into, before it is committed to the user-visible destination directory.
	ScratchDirNamePart = "scratch-"

	// The temp directory that contains the downloaded template.
	TemplateDirNamePart = "template-copy-"

	// The temp directory where the upgrade operation renders the upgraded
	// version of the template, before it is merged with the user-visible
	// destination directory.
	UpgradeMergeDirNamePart = "upgrade-merge-"

	// The temp directory where, during the upgrade process, the output of the
	// patch command is written to. This contains the result of applying every
	// patch in the manifest YAML file to the corresponding
	// included-from-destination file.
	ReversedPatchDirNamePart = "reversed-patch-"
)
