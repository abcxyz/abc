package testutil

import "path/filepath"

// KeysToPlatformPaths converts the keys from slash-separated paths to the local
// OS file separator. The maps are modified in place.
func KeysToPlatformPaths[T any](maps ...map[string]T) {
	for _, m := range maps {
		for k, v := range m {
			if newKey := filepath.FromSlash(k); newKey != k {
				m[newKey] = v
				delete(m, k)
			}
		}
	}
}

// ToPlatformPaths converts each input from slash-separated paths to the local
// OS file separator. The slices are modified in place.
func ToPlatformPaths(slices ...[]string) {
	for _, s := range slices {
		for i := range s {
			s[i] = filepath.FromSlash(s[i])
		}
	}
}
