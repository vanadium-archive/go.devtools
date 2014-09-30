package impl

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"tools/lib/cmdline"
	"tools/lib/util"
)

// TestGoVeyronEnvironment checks that the implementation of the
// 'veyron go' command sets up the veyron environment and then
// dispatches calls to the go tool.
func TestGoVeyronEnvironment(t *testing.T) {
	testCmd := cmdline.Command{}
	var out, errOut bytes.Buffer
	testCmd.Init(nil, &out, &errOut)
	if err := os.Setenv("GOPATH", ""); err != nil {
		t.Fatalf("%v", err)
	}
	if err := runGo(&testCmd, []string{"env", "GOPATH"}); err != nil {
		t.Fatalf("%v", err)
	}
	env, err := util.VeyronEnvironment()
	if err != nil {
		t.Fatalf("%v", err)
	}
	if expected, got := env["GOPATH"], strings.TrimSpace(out.String()); expected != got {
		t.Fatalf("unexpected GOPATH: expected %v, got %v", expected, got)
	}
}

// TestGoVDLGeneration checks that the implementation of the 'veyron
// go' command generates up-to-date VDL files for select go tool
// commands before dispatching these commands to the go tool.
func TestGoVDLGeneration(t *testing.T) {
	testCmd := cmdline.Command{}
	var out, errOut bytes.Buffer
	testCmd.Init(nil, &out, &errOut)
	root, err := util.VeyronRoot()
	if err != nil {
		t.Fatalf("%v", err)
	}
	testFile := filepath.Join(root, "tools", "go", "src", "tools", "veyron", "impl", "testdata", "test.vdl.go")
	// Remove the test VDL generated file.
	if err := os.Remove(testFile); err != nil {
		t.Fatalf("Remove(%v) failed: %v", testFile, err)
	}
	// Check that the 'env' go command does not generate the test VDL file.
	if err := runGo(&testCmd, []string{"env", "GOPATH"}); err != nil {
		t.Fatalf("%v", err)
	}
	if _, err := os.Stat(testFile); err != nil {
		if !os.IsNotExist(err) {
			t.Fatalf("Stat(%v) failed: %v", testFile, err)
		}
	} else {
		t.Fatalf("file %v exists and it should not.", testFile)
	}
	// Check that the 'build' go command generates the test VDL file.
	if err := runGo(&testCmd, []string{"build", "tools/..."}); err != nil {
		t.Fatalf("%v", err)
	}
	if _, err := os.Stat(testFile); err != nil {
		t.Fatalf("Stat(%v) failed: %v", testFile, err)
	}
}

// TestProfileList checks that the implementation of the 'veyron
// profile list' command lists the profiles supported by the current
// operating system.
func TestProfileList(t *testing.T) {
	// Setup fake profile description.
	root, err := util.VeyronRoot()
	if err != nil {
		t.Fatalf("%v", err)
	}
	profile, description := "test-profile", "test profile description\n"
	dir, mode := filepath.Join(root, "scripts", "setup", runtime.GOOS, profile), os.FileMode(0700)
	if err := os.Mkdir(dir, mode); err != nil {
		t.Fatalf("Mkdir(%v, %v) failed: %v", dir, mode, err)
	}
	defer os.RemoveAll(dir)
	file, mode := filepath.Join(dir, "DESCRIPTION"), os.FileMode(0600)
	if err := ioutil.WriteFile(file, []byte(description), mode); err != nil {
		t.Fatalf("WriteFile(%v, %v) failed: %v", file, mode, err)
	}
	// Check that the profile description is listed.
	testCmd := cmdline.Command{}
	var out, errOut bytes.Buffer
	testCmd.Init(nil, &out, &errOut)
	if err := runProfileList(&testCmd, nil); err != nil {
		t.Fatalf("%v", err)
	}
	match, expected := false, fmt.Sprintf("  %s: %s", profile, strings.TrimSpace(description))
	for _, line := range strings.Split(out.String(), "\n") {
		if expected == line {
			match = true
			break
		}
	}
	if !match {
		t.Fatalf("no match for %v found in:\n%v", expected, out.String())
	}
}

// TestProfileSetup checks that the implementation of the 'veyron
// profile setup' command executes the profile setup script.
func TestProfileSetup(t *testing.T) {
	// Setup fake profile script.
	root, err := util.VeyronRoot()
	if err != nil {
		t.Fatalf("%v", err)
	}
	profile := "test-profile"
	dir, mode := filepath.Join(root, "scripts", "setup", runtime.GOOS, profile), os.FileMode(0700)
	if err := os.Mkdir(dir, mode); err != nil {
		t.Fatalf("Mkdir(%v, %v) failed: %v", dir, mode, err)
	}
	defer os.RemoveAll(dir)
	file, mode, script := filepath.Join(dir, "setup.sh"), os.FileMode(0700), "#!/bin/bash\n"
	if err := ioutil.WriteFile(file, []byte(script), mode); err != nil {
		t.Fatalf("WriteFile(%v, %v) failed: %v", file, mode, err)
	}
	// Check that the profile script is executed.
	testCmd := cmdline.Command{}
	verboseFlag = true
	var stdout, stderr bytes.Buffer
	testCmd.Init(nil, &stdout, &stderr)
	if err := runProfileSetup(&testCmd, []string{profile}); err != nil {
		t.Fatalf("%v", err)
	}
	if got, expected := stdout.String(), fmt.Sprintf(">> Setting up profile %v\n>>>> %v\n>>>> OK\n>> OK\n", profile, file); expected != got {
		t.Fatalf("unexpected output:\nexpected\n%v\ngot\n%v", expected, got)
	}
}
