package commands

import (
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
)

func TestParseFlags(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		args    []string
		want    flagValues
		wantErr string
	}{
		{
			name: "all flags present",
			args: []string{
				"--template", "helloworld@v1",
				"--spec", "my_spec.yaml",
				"--dest", "my_dir",
				"--git-protocol", "https",
				"--input", "x=y",
				"--log-level", "info",
				"--force-overwrite",
				"--keep-temp-dirs",
			},
			want: flagValues{
				template:       "helloworld@v1",
				spec:           "my_spec.yaml",
				dest:           "my_dir",
				gitProtocol:    "https",
				inputs:         map[string]string{"x": "y"},
				logLevel:       "info",
				forceOverwrite: true,
				keepTempDirs:   true,
			},
		},
		{
			name: "minimal flags present",
			args: []string{
				"-t", "helloworld@v1",
			},
			want: flagValues{
				template:       "helloworld@v1",
				spec:           "./spec.yaml",
				dest:           ".",
				gitProtocol:    "https",
				inputs:         nil,
				logLevel:       "warning",
				forceOverwrite: false,
				keepTempDirs:   false,
			},
		},
		{
			name:    "a required flag is missing",
			args:    []string{},
			wantErr: "-template is required",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := &Render{}
			err := c.parseFlags(tc.args)
			if tc.wantErr != "" {
				if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
					t.Fatal(diff)
				}
				return
			}

			if diff := cmp.Diff(c.fv, tc.want, cmp.AllowUnexported(flagValues{})); diff != "" {
				t.Errorf("got %#v, want %#v, diff (-got, +want): %v", c.fv, tc.want, diff)
			}
		})
	}
}

func TestDestOK(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		flags   *flagValues
		in      fakeFS
		wantErr string
	}{
		{
			name: "dest is a directory that exists, should succeed",
			flags: &flagValues{
				dest: "my/dir",
			},
			in: fakeFS{
				paths: map[string]string{
					"my/dir/foo.txt": "",
				},
			},
		},
		{
			name: "the dest directory is actually a file, should fail",
			flags: &flagValues{
				dest: "my/file",
			},
			in: fakeFS{
				paths: map[string]string{
					"my/file": "",
				},
			},
			wantErr: "is not a directory",
		},
		{
			name: "the dest directory doesn't exist, should fail",
			flags: &flagValues{
				dest: "my/dir",
			},
			in: fakeFS{
				paths: map[string]string{},
			},
			wantErr: "doesn't exist",
		},
		{
			name: "stat returns error",
			flags: &flagValues{
				dest: "my/git/dir",
			},
			in: fakeFS{
				injectErr: fmt.Errorf("yikes"),
			},
			wantErr: "yikes",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := destOK(tc.in, tc.flags)
			if diff := testutil.DiffErrString(got, tc.wantErr); diff != "" {
				t.Error(diff)
			}
		})
	}
}

// A fake implementation of the "fs" filesystem interface.
type fakeFS struct {
	// Keys are filenames like dir1/dir2/hello.txt. Values are file contents.
	// A entry in this map implies the existence of its parent dirs; e.g. the existence of "dir1/file.txt" implies
	// that dir1 exists as a directory.
	paths     map[string]string
	injectErr error
}

func (f fakeFS) Stat(name string) (os.FileInfo, error) {
	var out os.FileInfo // The value isn't used, don't bother setting it

	if f.injectErr != nil {
		return out, f.injectErr
	}

	_, ok := f.paths[name]
	if ok {
		// The path exists and its a file.
		return &fakeFileInfo{
			name:  path.Base(name),
			isDir: false,
		}, nil
	}

	for p := range f.paths {
		// Look for any files in "paths" that have the given name as parent/grandparent/etc directory.
		// E.g. if there's a file dir1/dir2/file.txt then Stat("dir1") will return true.
		for { // traverse up towards the root
			var child string
			p, child = path.Split(p)
			if p == "" {
				break
			}
			p = strings.TrimSuffix(p, "/")
			fmt.Printf("p=%s child=%s\n", p, child)
			if p == name {
				return &fakeFileInfo{
					name:  child,
					isDir: true,
				}, nil
			}
		}
	}

	return out, os.ErrNotExist
}

type fakeFileInfo struct {
	os.FileInfo

	name  string
	isDir bool
}

func (f *fakeFileInfo) IsDir() bool {
	return f.isDir
}

func (f *fakeFileInfo) Name() string {
	return f.name
}
