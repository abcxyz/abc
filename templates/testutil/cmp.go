package testutil

import "strings"

// TODO doc
func TrimPrefixComparer(prefix string) func(s1, s2 string) bool {
	return func(s1, s2 string) bool {
		// We can't write our test assertions to include the temp
		// directory, since it changes every time, so dynamically rewrite the paths in the output
		// files to remove the temp directory name.
		trim := func(s string) string {
			return strings.TrimPrefix(s, prefix)
		}
		return trim(s1) == trim(s2)
	}
}
