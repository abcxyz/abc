// Copyright 2023 The Authors (see AUTHORS file)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package utils contains the utility function for template commands.

package utils

import (
	"regexp"
	"strings"

	"golang.org/x/exp/slices"
)

var (
	// hyphenOrSnakeCaseKeep are all the characters to keep for generating snake_case or hyphen-case strings.
	// The regex is used to match all characters not specified here so we can remove them.
	hyphenOrSnakeCaseKeep = regexp.MustCompile(`[^a-zA-Z0-9-_ ]+`)

	// snakeCaseReplace are all the characters to replace with underscores to generate snake_case strings.
	// The regex is used to match all characters here so we can replace them.
	snakeCaseReplace = regexp.MustCompile(`[- ]+`)

	// hyphenCaseReplace are all the characters to replace with hyphens to generate hyphen-case strings.
	// The regex is used to match all characters here so we can replace them.
	hyphenCaseReplace = regexp.MustCompile(`[_ ]+`)
)

// toSnakeCase converts a string to snake_case by removing all characters
// (except alphanumeric, hyphens, underscores and spaces) and replacing
// any hyphens or spaces with underscores.
func ToSnakeCase(v string) string {
	response := hyphenOrSnakeCaseKeep.ReplaceAllString(v, "")
	response = snakeCaseReplace.ReplaceAllString(response, "_")
	return response
}

// toLowerSnakeCase converts a string to snake_case by removing all characters
// (except alphanumeric, hyphens, underscores and spaces) and replacing
// any hyphens or spaces with underscores. The result is then returned
// as a lower case string.
func ToLowerSnakeCase(v string) string {
	return strings.ToLower(ToSnakeCase(v))
}

// toUpperSnakeCase converts a string to snake_case by removing all characters
// (except alphanumeric, hyphens, underscores and spaces) and replacing
// any hyphens or spaces with underscores. The result is then returned
// as a upper case string.
func ToUpperSnakeCase(v string) string {
	return strings.ToUpper(ToSnakeCase(v))
}

// toHyphenCase converts a string to hyphen-case by removing all characters
// (except alphanumeric, hyphens, underscores and spaces) and replacing
// any hyphens or spaces with underscores.
func ToHyphenCase(v string) string {
	response := hyphenOrSnakeCaseKeep.ReplaceAllString(v, "")
	response = hyphenCaseReplace.ReplaceAllString(response, "-")
	return response
}

// toLowerHyphenCase converts a string to hyphen-case by removing all characters
// (except alphanumeric, hyphens, underscores and spaces) and replacing
// any underscores or spaces with hyphens. The result is then returned
// as a lower case string.
func ToLowerHyphenCase(v string) string {
	return strings.ToLower(ToHyphenCase(v))
}

// toUpperSnakeCase converts a string to hyphen-case by removing all characters
// (except alphanumeric, hyphens, underscores and spaces) and replacing
// any underscores or spaces with hyphens. The result is then returned
// as a upper case string.
func ToUpperHyphenCase(v string) string {
	return strings.ToUpper(ToHyphenCase(v))
}

// sortStrings sorts the given list of strings. Go's built-in sorting behavior
// modifies the string in place. It would be very weird if rendering a template
// changed the order of an input further down the stack.
func SortStrings(in []string) []string {
	cp := slices.Clone(in)
	slices.Sort(cp)
	return cp
}
