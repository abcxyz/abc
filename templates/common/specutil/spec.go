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
	"fmt"
	"io"
	"text/tabwriter"

	spec "github.com/abcxyz/abc/templates/model/spec/v1beta1"
)

const (
	// The spec file is always located in the template root dir and named spec.yaml.
	SpacFileName = "spec.yaml"
)

// ParseSpecToList parses a spec.Spec into a list with
// the keys and attribute pairs.
//
// Example:
// ["Description", "example description", "Input Name", "example name"].
func ParseSpecToList(spec *spec.Spec) []string {
	l := make([]string, 0)
	l = append(l, "Description", spec.Desc.Val)
	for _, v := range spec.Inputs {
		l = append(l, parseSpecInputVar(v)...)
	}

	return l
}

// parseSpecInputVar parses spec.Input  into a
// list with key and attribute pairs.
func parseSpecInputVar(input *spec.Input) []string {
	l := make([]string, 0)
	l = append(l, "Input name", input.Name.Val, "Description", input.Desc.Val)
	if input.Default != nil {
		defaultStr := input.Default.Val
		if defaultStr == "" {
			// When empty string is the default, print it differently so
			// the user can actually see what's happening.
			defaultStr = `""`
		}
		l = append(l, "Default", defaultStr)
	}

	for idx, rule := range input.Rules {
		l = append(l, fmt.Sprintf("Rule %v", idx), rule.Rule.Val)
		if rule.Message.Val != "" {
			l = append(l, fmt.Sprintf("Rule %v msg", idx), rule.Message.Val)
		}
	}
	return l
}

// FormatAttrList formats the attribute list for output
//
// Example output:
//
// Description:  A template for the ages

// Input name:   name1
// Description:  desc1
// Default:      .
// Rule 0:       test rule 0
// Rule 0 msg:   test rule 0 message
// Rule 1:       test rule 1

// Input name:   name2
// Description:  desc2.
func FormatAttrList(w io.Writer, attrList []string) {
	tw := tabwriter.NewWriter(w, 8, 0, 2, ' ', 0)
	for i := 0; i < len(attrList); i += 2 {
		// print an empty line between inputs
		if attrList[i] == "Input name" {
			fmt.Fprintf(tw, "\n")
		}
		fmt.Fprintf(tw, "%s:\t%s\n", attrList[i], attrList[i+1])
	}
	tw.Flush()
}
