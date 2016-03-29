// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"v.io/jiri/jiritest"
	"v.io/jiri/tool"
	"v.io/x/devtools/jiri-test/internal/test"
	"v.io/x/devtools/tooldata"
)

func TestTestProject(t *testing.T) {
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()

	// Point the WORKSPACE environment variable to the fake.
	oldWorkspace := os.Getenv("WORKSPACE")
	if err := os.Setenv("WORKSPACE", fake.X.Root); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("WORKSPACE", oldWorkspace)

	// Setup a fake config.
	config := tooldata.NewConfig(tooldata.ProjectTestsOpt(map[string][]string{"https://test-project": []string{"ignore-this"}}))
	if err := tooldata.SaveConfig(fake.X, config); err != nil {
		t.Fatalf("%v", err)
	}

	// Check that running the tests for the test project generates
	// the expected output.
	var out bytes.Buffer
	fake.X.Context = tool.NewContext(tool.ContextOpts{Stdout: &out, Stderr: &out})
	if err := runTestProject(fake.X, []string{"https://test-project"}); err != nil {
		t.Fatalf("%v", err)
	}
	got, want := out.String(), `##### Running test "ignore-this" #####
##### PASSED #####
SUMMARY:
ignore-this PASSED
`
	if got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestTestRun(t *testing.T) {
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()

	// Point the WORKSPACE environment variable to the fake.
	oldWorkspace := os.Getenv("WORKSPACE")
	if err := os.Setenv("WORKSPACE", fake.X.Root); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("WORKSPACE", oldWorkspace)

	// Check that running the test generates the expected output.
	var out bytes.Buffer
	fake.X.Context = tool.NewContext(tool.ContextOpts{Stdout: &out, Stderr: &out})
	if err := runTestRun(fake.X, []string{"ignore-this"}); err != nil {
		t.Fatalf("%v", err)
	}
	got, want := out.String(), `##### Running test "ignore-this" #####
##### PASSED #####
SUMMARY:
ignore-this PASSED
`
	if got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestTestList(t *testing.T) {
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()

	// Point the WORKSPACE environment variable to the fake.
	oldWorkspace := os.Getenv("WORKSPACE")
	if err := os.Setenv("WORKSPACE", fake.X.Root); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("WORKSPACE", oldWorkspace)

	// Check that listing existing tests generates the expected output.
	var out bytes.Buffer
	fake.X.Context = tool.NewContext(tool.ContextOpts{Stdout: &out, Stderr: &out})
	if err := runTestList(fake.X, []string{}); err != nil {
		t.Fatalf("%v", err)
	}
	testList, err := test.ListTests()
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got, want := strings.TrimSpace(out.String()), strings.Join(testList, "\n"); got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}
