package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"tools/lib/cmdline"
	"tools/lib/util"
)

func createBuildDir(t *testing.T, rootDir, name string, buildNames []string) {
	stableDir, perm := filepath.Join(rootDir, ".manifest", "v1", "builds", name), os.FileMode(0700)
	if err := os.MkdirAll(stableDir, perm); err != nil {
		t.Fatalf("MkdirAll(%v, %v) failed: %v", stableDir, perm, err)
	}
	for i, buildName := range buildNames {
		path := filepath.Join(stableDir, buildName)
		_, err := os.Create(path)
		if err != nil {
			t.Fatalf("%v", err)
		}
		if i == 0 {
			symlinkPath := filepath.Join(rootDir, ".manifest", "v1", name)
			if err := os.Symlink(path, symlinkPath); err != nil {
				t.Fatalf("Symlink(%v, %v) failed: %v", path, symlinkPath, err)
			}
		}
	}
}

func generateOutput(builds []build) string {
	output := ""
	for _, b := range builds {
		output += fmt.Sprintf("%q builds:\n", b.name)
		for _, name := range b.builds {
			output += fmt.Sprintf("  %v\n", name)
		}
	}
	return output
}

type build struct {
	name   string
	builds []string
}

func TestList(t *testing.T) {
	// Setup a fake VEYRON_ROOT.
	dir, prefix := "", ""
	tmpDir, err := ioutil.TempDir(dir, prefix)
	if err != nil {
		t.Fatalf("TempDir(%v, %v) failed: %v", dir, prefix, err)
	}
	defer os.RemoveAll(tmpDir)
	oldRoot, err := util.VeyronRoot()
	if err := os.Setenv("VEYRON_ROOT", tmpDir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("VEYRON_ROOT", oldRoot)

	// Create a test suite.
	builds := []build{
		build{
			name:   "beta",
			builds: []string{"beta-1", "beta-2", "beta-3"},
		},
		build{
			name:   "stable",
			builds: []string{"stable-1", "stable-2", "stable-3"},
		},
	}
	for _, b := range builds {
		createBuildDir(t, tmpDir, b.name, b.builds)
	}

	// Check that running "buildbot list" with no arguments
	// returns the expected output.
	var stdout bytes.Buffer
	command := cmdline.Command{}
	command.Init(nil, &stdout, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := runBuildList(&command, nil); err != nil {
		t.Fatalf("%v", err)
	}
	got, want := stdout.String(), generateOutput(builds)
	if got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v\n", got, want)
	}

	// Check that running "buildbot list" with one argument
	// returns the expected output.
	stdout.Reset()
	if err := runBuildList(&command, []string{"stable"}); err != nil {
		t.Fatalf("%v", err)
	}
	got, want = stdout.String(), generateOutput(builds[1:])
	if got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v\n", got, want)
	}

	// Check that running "buildbot list" with multiple arguments
	// returns the expected output.
	stdout.Reset()
	if err := runBuildList(&command, []string{"beta", "stable"}); err != nil {
		t.Fatalf("%v", err)
	}
	got, want = stdout.String(), generateOutput(builds)
	if got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v\n", got, want)
	}
}
