package testutil

import (
	"reflect"
	"slices"
	"strings"

	"github.com/google/go-cmp/cmp"
)

// TODO doc, example invocation
func TrimPrefixComparer(prefix string) func(s1, s2 string) bool {
	return func(s1, s2 string) bool {
		// We can't write our test assertions to include the temp directory,
		// since it changes every time, so dynamically rewrite the paths in the
		// output files to remove the temp directory name.
		trim := func(s string) string {
			return strings.TrimPrefix(s, prefix)
		}
		return trim(s1) == trim(s2)
	}
}

// TODO doc, example
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

func TrimStringPrefixTransformer(prefix string) cmp.Option {
	return cmp.Transformer("trim_prefix", func(s string) string {
		return strings.TrimPrefix(s, prefix)
	})
}
