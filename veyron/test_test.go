package main

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"tools/lib/cmdline"
	"tools/lib/runutil"
	"tools/lib/util"
)

func createTestScript(t *testing.T, ctx *util.Context) {
	testScript, err := util.TestScriptFile("veyron-test")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := ctx.Run().Function(runutil.MkdirAll(filepath.Dir(testScript), os.FileMode(0755))); err != nil {
		t.Fatalf("%v", err)
	}
	data := "#!/bin/bash\n"
	if err := ioutil.WriteFile(testScript, []byte(data), os.FileMode(0755)); err != nil {
		t.Fatalf("WriteFile(%v) failed: %v", testScript, err)
	}
}

func TestTestProject(t *testing.T) {
	// Setup an instance of veyron universe.
	ctx := util.DefaultContext()
	dir, prefix := "", ""
	rootDir, err := ioutil.TempDir(dir, prefix)
	if err != nil {
		t.Fatalf("TempDir(%v, %v) failed: %v", dir, prefix, err)
	}
	defer os.RemoveAll(rootDir)
	oldRoot := os.Getenv("VEYRON_ROOT")
	if err := os.Setenv("VEYRON_ROOT", rootDir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("VEYRON_ROOT", oldRoot)

	config := util.CommonConfig{
		ProjectTests: map[string][]string{
			"https://test-project": []string{"veyron-test"},
		},
	}
	createConfig(t, ctx, &config)
	createTestScript(t, ctx)

	// Check that running the tests for the test project generates
	// the expected output.
	var out bytes.Buffer
	command := cmdline.Command{}
	command.Init(nil, &out, &out)
	if err := runTestProject(&command, []string{"https://test-project"}); err != nil {
		t.Fatalf("%v", err)
	}
	got, want := out.String(), `##### Running test "veyron-test" #####
##### PASSED #####
SUMMARY:
veyron-test PASSED
`
	if got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestTestRun(t *testing.T) {
	// Setup an instance of veyron universe.
	ctx := util.DefaultContext()
	dir, prefix := "", ""
	rootDir, err := ioutil.TempDir(dir, prefix)
	if err != nil {
		t.Fatalf("TempDir(%v, %v) failed: %v", dir, prefix, err)
	}
	defer os.RemoveAll(rootDir)
	oldRoot := os.Getenv("VEYRON_ROOT")
	if err := os.Setenv("VEYRON_ROOT", rootDir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("VEYRON_ROOT", oldRoot)

	createTestScript(t, ctx)

	// Check that running the test generates the expected output.
	var out bytes.Buffer
	command := cmdline.Command{}
	command.Init(nil, &out, &out)
	if err := runTestRun(&command, []string{"veyron-test"}); err != nil {
		t.Fatalf("%v", err)
	}
	got, want := out.String(), `##### Running test "veyron-test" #####
##### PASSED #####
SUMMARY:
veyron-test PASSED
`
	if got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestTestList(t *testing.T) {
	// Setup an instance of veyron universe.
	ctx := util.DefaultContext()
	dir, prefix := "", ""
	rootDir, err := ioutil.TempDir(dir, prefix)
	if err != nil {
		t.Fatalf("TempDir(%v, %v) failed: %v", dir, prefix, err)
	}
	defer os.RemoveAll(rootDir)
	oldRoot := os.Getenv("VEYRON_ROOT")
	if err := os.Setenv("VEYRON_ROOT", rootDir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("VEYRON_ROOT", oldRoot)

	createTestScript(t, ctx)

	// Check that listing existing tests generates the expected
	// output.
	var out bytes.Buffer
	command := cmdline.Command{}
	command.Init(nil, &out, &out)
	if err := runTestList(&command, []string{}); err != nil {
		t.Fatalf("%v", err)
	}
	if got, want := out.String(), "veyron-test\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}
