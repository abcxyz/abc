package render

import (
	"os"
)

type realFS struct{}

func (r *realFS) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
