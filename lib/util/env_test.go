package util

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

// TestVeyronRootSymlink checks that VeyronRoot interprets the value
// of the VEYRON_ROOT environment variable as a path, evaluates any
// symlinks the path might contain, and returns the result.
func TestVeyronRootSymlink(t *testing.T) {
	// Create a temporary directory.
	dir, prefix := "", ""
	tmpDir, err := ioutil.TempDir(dir, prefix)
	if err != nil {
		t.Fatalf("TempDir(%v, %v) failed: %v", dir, prefix, err)
	}
	defer os.RemoveAll(tmpDir)

	// Make sure tmpDir is not a symlink itself.
	tmpDir, err = filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%v) failed: %v", tmpDir, err)
	}

	// Create a directory and a symlink to it.
	root, perm := filepath.Join(tmpDir, "root"), os.FileMode(0700)
	if err := os.Mkdir(root, perm); err != nil {
		t.Fatalf("%v", err)
	}
	symRoot := filepath.Join(tmpDir, "sym_root")
	if err := os.Symlink(root, symRoot); err != nil {
		t.Fatalf("%v", err)
	}

	// Set the VEYRON_ROOT to the symlink created above and check
	// that VeyronRoot() evaluates the symlink.
	if err := os.Setenv("VEYRON_ROOT", symRoot); err != nil {
		t.Fatalf("%v", err)
	}
	got, err := VeyronRoot()
	if err != nil {
		t.Fatalf("%v", err)
	}
	if want := root; got != want {
		t.Fatalf("unexpected output: got %v, want %v", got, want)
	}
}
