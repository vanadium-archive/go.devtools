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

type testScriptInfo struct {
	shortName      string
	name           string
	path           string
	content        string
	expectedResult string
}

var testScripts = []testScriptInfo{
	testScriptInfo{
		shortName:      "binaryd",
		name:           "veyron/go/src/veyron.io/veyron/veyron/services/mgmt/binary/binaryd",
		path:           "veyron/go/src/veyron.io/veyron/veyron/services/mgmt/binary/binaryd/test.sh",
		content:        "#!/bin/bash\necho binaryd",
		expectedResult: "PASS",
	},
	testScriptInfo{
		shortName:      "principal",
		name:           "veyron/go/src/veyron.io/veyron/veyron/tools/principal",
		path:           "veyron/go/src/veyron.io/veyron/veyron/tools/principal/test.sh",
		content:        "#!/bin/bash\nexit 1",
		expectedResult: "FAIL",
	},
}

func TestIntegrationTestList(t *testing.T) {
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

	// Create some fake integration test scripts.
	for _, testScript := range testScripts {
		createTestScript(testScript, tmpDir, t)
	}

	// Check "integration-test list"
	var stdout bytes.Buffer
	command := cmdline.Command{}
	command.Init(nil, &stdout, nil)
	if err := runIntegrationTestList(&command, nil); err != nil {
		t.Fatalf("%v", err)
	}
	want := fmt.Sprintf("%s (%s)\n%s (%s)\n",
		testScripts[0].shortName, testScripts[0].name,
		testScripts[1].shortName, testScripts[1].name)
	if got := stdout.String(); want != got {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v\n", got, want)
	}
}

func TestIntegrationTestRun(t *testing.T) {
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

	// Create some fake integration test scripts.
	for _, testScript := range testScripts {
		createTestScript(testScript, tmpDir, t)
	}

	// Check "integration-test run <test_names>".
	// Test when test_names has one single name.
	for _, testScript := range testScripts {
		var stdout, stderr bytes.Buffer
		command := cmdline.Command{}
		command.Init(nil, &stdout, &stderr)
		if err := runIntegrationTestRun(&command, []string{testScript.shortName}); err != nil {
			t.Fatalf("%v", err)
		}
		expectedResult := fmt.Sprintf("%s: %s\n", testScript.expectedResult, testScript.name)
		if got := stdout.String(); got != expectedResult {
			t.Fatalf("unexpected result:\ngot\n%v\nwant\n%v\n", got, expectedResult)
		}
	}

	// Test when test_names has two names.
	var stdout, stderr bytes.Buffer
	command := cmdline.Command{}
	command.Init(nil, &stdout, &stderr)
	if err := runIntegrationTestRun(&command, []string{testScripts[0].shortName, testScripts[1].shortName}); err != nil {
		t.Fatalf("%v", err)
	}
	expectedResult := fmt.Sprintf("%s: %s\n%s: %s\n",
		testScripts[0].expectedResult, testScripts[0].name,
		testScripts[1].expectedResult, testScripts[1].name)
	if got := stdout.String(); got != expectedResult {
		t.Fatalf("unexpected result:\ngot\n%v\nwant\n%v\n", got, expectedResult)
	}

	// Test when test_names has invalid name.
	stdout.Reset()
	stderr.Reset()
	if err := runIntegrationTestRun(&command, []string{"blah"}); err != nil {
		t.Fatalf("%v", err)
	}
	expectedStderr := "test name \"blah\" not found. Skipped.\n"
	if got := stderr.String(); got != expectedStderr {
		t.Fatalf("unexpected stderr:\ngot\n%v\nwant\n%v\n", got, expectedStderr)
	}
}

func createTestScript(testScript testScriptInfo, veyronRoot string, t *testing.T) {
	scriptPath := filepath.Join(veyronRoot, testScript.path)
	dir := filepath.Dir(scriptPath)
	dirMode := os.FileMode(0700)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		t.Fatalf("MkdirAll(%q, %v) failed: %v", dir, dirMode, err)
	}
	fileMode := os.FileMode(0755)
	if err := ioutil.WriteFile(scriptPath, []byte(testScript.content), fileMode); err != nil {
		t.Fatalf("WriteFile(%q, %q, %v) failed: %v", scriptPath, testScript.content, fileMode, err)
	}
}
