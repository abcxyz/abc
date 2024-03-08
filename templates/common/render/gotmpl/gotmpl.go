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

package gotmpl

import (
	"regexp"
	"sort"
	"strings"
	"text/template"

	"golang.org/x/exp/maps"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/errs"
	"github.com/abcxyz/abc/templates/model"
)

var templateKeyErrRegex = regexp.MustCompile(`map has no entry for key "([^"]*)"`)

// pos may be nil if the template is not coming from the spec file and therefore
// there's no reason to print out spec file location in an error message. If
// template execution fails because of a missing input variable, the error will
// be wrapped in a UnknownVarErr.
func ParseExec(pos *model.ConfigPos, tmpl string, scope *common.Scope) (string, error) {
	// As of go1.20, if the template references a nonexistent variable, then the
	// returned error will be of type *errors.errorString; unfortunately there's
	// no distinctive error type we can use to detect this particular error.
	//
	// We only get this error because we ask for Option("missingkey=error") when
	// parsing the template. Otherwise it would silently insert "<no value>".
	parsedTmpl, err := template.New("").Funcs(scope.GoTmplFuncs()).Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", pos.Errorf(`error compiling as go-template: %w`, err)
	}
	var sb strings.Builder
	vars := scope.AllVars()
	if err := parsedTmpl.Execute(&sb, vars); err != nil {
		// If this error looks like a missing key error, then replace it with a
		// more helpful error.
		matches := templateKeyErrRegex.FindStringSubmatch(err.Error())
		if matches != nil {
			varNames := maps.Keys(vars)
			sort.Strings(varNames)
			err = &errs.UnknownVarError{
				VarName:       matches[1],
				AvailableVars: varNames,
				Wrapped:       err,
			}
		}
		return "", pos.Errorf("template.Execute() failed: %w", err)
	}
	return sb.String(), nil
}

// ParseExecAll runs ParseExec on each of the input strings (which should
// contain Go templates).
func ParseExecAll(ss []model.String, scope *common.Scope) ([]string, error) {
	out := make([]string, len(ss))
	for i, in := range ss {
		var err error
		out[i], err = ParseExec(in.Pos, in.Val, scope)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
