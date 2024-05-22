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

package testutil

import (
	"reflect"
	"slices"
	"strings"

	"github.com/google/go-cmp/cmp"
)

// TransformStructFields is an Option to use with the cmp library that modifies
// certain struct fields before doing the comparison. The provided transform is
// provided to all struct fields who field name is in the "fieldNames" list and
// whose struct type matches "structExample".
//
// Example invocation: trim the string "foo" from the beginning of
// MyStruct.MyFirstField and MyStruct.MySecondField before doing the comparison.
//
//	cmp.Diff(x, y, abctestutil.TransformStructFields(
//	                  abctestutil.TrimStringPrefixTransformer("foo"),
//	                  MyStruct{}, "MyFirstField", "MySecondField")
func TransformStructFields(transform cmp.Option, structExample interface{}, fieldNames ...string) cmp.Option {
	return cmp.FilterPath(
		func(p cmp.Path) bool {
			sf, ok := p.Last().(cmp.StructField)
			if !ok {
				return false
			}
			structType := p.Index(-2).Type()
			if structType != reflect.TypeOf(structExample) {
				return false
			}
			return ok && structType == reflect.TypeOf(structExample) &&
				slices.Contains(fieldNames, sf.Name())
		},
		transform)
}

// TrimStringPrefixTransformer is a Transformer function to use with the cmp
// library that removes a prefix from strings before comparing them. See
// the [TransformStructFields] documentation for an example.
func TrimStringPrefixTransformer(prefix string) cmp.Option {
	return cmp.Transformer("trim_prefix", func(s string) string {
		return strings.TrimPrefix(s, prefix)
	})
}
