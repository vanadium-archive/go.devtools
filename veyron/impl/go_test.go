package impl

import (
	"bytes"
	"os"
	"path/filepath"
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
	env, err := util.VeyronEnvironment(util.HostPlatform())
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
