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

// Package spec contains commonly used function for handling spec files
package specutil

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/model/decode"
	spec "github.com/abcxyz/abc/templates/model/spec/v1beta1"
)

const (
	// The spec file is always located in the template root dir and named spec.yaml.
	SpecFileName = "spec.yaml"

	// Keys for output formatting.
	OutputDescriptionKey       = "Description"
	OutputInputNameKey         = "Input name"
	OutputInputDefaultValueKey = "Default"
	OutputInputRuleKey         = "Rule"
)

// Attrs returns a list of human-readable attributes describing a spec,
// as a list where each entry is a list of columns.
//
// Example:
//
//	{
//	  {"Description", "example description"},
//	  {"Input Name", "example name"},
//	}
func Attrs(spec *spec.Spec) [][]string {
	l := make([][]string, 0)
	l = append(l, []string{OutputDescriptionKey, spec.Desc.Val})
	return l
}

// AllInputAttrs describes all spec.Input values in the spec.
func AllInputAttrs(spec *spec.Spec) [][]string {
	l := make([][]string, 0)
	for _, v := range spec.Inputs {
		l = append(l, OneInputAttrs(v)...)
	}
	return l
}

// OneInputAttrs describes a specific spec.Input value.
func OneInputAttrs(input *spec.Input) [][]string {
	l := make([][]string, 0)
	l = append(l, []string{OutputInputNameKey, input.Name.Val}, []string{OutputDescriptionKey, input.Desc.Val})
	if input.Default != nil {
		defaultStr := input.Default.Val
		if defaultStr == "" {
			// When empty string is the default, print it differently so
			// the user can actually see what's happening.
			defaultStr = `""`
		}
		l = append(l, []string{OutputInputDefaultValueKey, defaultStr})
	}

	for idx, rule := range input.Rules {
		l = append(l, []string{fmt.Sprintf("%s %v", OutputInputRuleKey, idx), rule.Rule.Val})
		if rule.Message.Val != "" {
			l = append(l, []string{fmt.Sprintf("%s %v msg", OutputInputRuleKey, idx), rule.Message.Val})
		}
	}
	return l
}

// FormatAttrs formats the attribute list for output
//
// Example output:
//
// Description: Test Template
//
// Input name:   name1
// Description:  desc1
// Default:      .
// Rule 0:       test rule 0
// Rule 0 msg:   test rule 0 message
// Rule 1:       test rule 1
//
// Input name:   name2
// Description:  desc2.
func FormatAttrs(w io.Writer, attrList [][]string) {
	tw := tabwriter.NewWriter(w, 8, 0, 2, ' ', 0)
	for _, v := range attrList {
		if v[0] == OutputInputNameKey {
			fmt.Fprintf(tw, "\n")
		}
		fmt.Fprintf(tw, "%s:\t%s\n", v[0], v[1])
	}
	tw.Flush()
}

// Load unmarshals the spec.yaml in the given directory.
func Load(ctx context.Context, fs common.FS, templateDir, source string) (*spec.Spec, error) {
	specPath := filepath.Join(templateDir, SpecFileName)
	f, err := fs.Open(specPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("couldn't find spec.yaml in that directory, the provided template location %q might be incorrect", source)
		}
		return nil, fmt.Errorf("error opening template spec: Open(): %w", err)
	}
	defer f.Close()

	specI, err := decode.DecodeValidateUpgrade(ctx, f, SpecFileName, decode.KindTemplate)
	if err != nil {
		return nil, fmt.Errorf("error reading template spec file: %w", err)
	}

	spec, ok := specI.(*spec.Spec)
	if !ok {
		return nil, fmt.Errorf("internal error: spec file did not decode to spec.Spec")
	}

	return spec, nil
}
