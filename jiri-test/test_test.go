// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"v.io/jiri/project"
	"v.io/jiri/tool"
	"v.io/jiri/util"
	"v.io/x/devtools/jiri-test/internal/test"
)

func TestTestProject(t *testing.T) {
	// Setup a fake JIRI_ROOT.
	root, err := project.NewFakeJiriRoot()
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer func() {
		if err := root.Cleanup(); err != nil {
			t.Fatalf("%v", err)
		}
	}()

	// Point the JIRI_ROOT and WORKSPACE environment variables to
	// the fake.
	oldRoot := os.Getenv("JIRI_ROOT")
	if err := os.Setenv("JIRI_ROOT", root.Dir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("JIRI_ROOT", oldRoot)
	oldWorkspace := os.Getenv("WORKSPACE")
	if err := os.Setenv("WORKSPACE", root.Dir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("WORKSPACE", oldWorkspace)

	// Setup a fake config.
	config := util.NewConfig(util.ProjectTestsOpt(map[string][]string{"https://test-project": []string{"ignore-this"}}))
	if err := util.SaveConfig(root.X, config); err != nil {
		t.Fatalf("%v", err)
	}

	// Check that running the tests for the test project generates
	// the expected output.
	var out bytes.Buffer
	root.X.Context = tool.NewContext(tool.ContextOpts{Stdout: &out, Stderr: &out})
	if err := runTestProject(root.X, []string{"https://test-project"}); err != nil {
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
	// Setup a fake JIRI_ROOT.
	root, err := project.NewFakeJiriRoot()
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer func() {
		if err := root.Cleanup(); err != nil {
			t.Fatalf("%v", err)
		}
	}()

	// Point the JIRI_ROOT and WORKSPACE environment variables to
	// the fake.
	oldRoot := os.Getenv("JIRI_ROOT")
	if err := os.Setenv("JIRI_ROOT", root.Dir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("JIRI_ROOT", oldRoot)
	oldWorkspace := os.Getenv("WORKSPACE")
	if err := os.Setenv("WORKSPACE", root.Dir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("WORKSPACE", oldWorkspace)

	// Check that running the test generates the expected output.
	var out bytes.Buffer
	root.X.Context = tool.NewContext(tool.ContextOpts{Stdout: &out, Stderr: &out})
	if err := runTestRun(root.X, []string{"ignore-this"}); err != nil {
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
	// Setup a fake JIRI_ROOT.
	root, err := project.NewFakeJiriRoot()
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer func() {
		if err := root.Cleanup(); err != nil {
			t.Fatalf("%v", err)
		}
	}()

	// Point the JIRI_ROOT and WORKSPACE environment variables to
	// the fake.
	oldRoot := os.Getenv("JIRI_ROOT")
	if err := os.Setenv("JIRI_ROOT", root.Dir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("JIRI_ROOT", oldRoot)
	oldWorkspace := os.Getenv("WORKSPACE")
	if err := os.Setenv("WORKSPACE", root.Dir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("WORKSPACE", oldWorkspace)

	// Check that listing existing tests generates the expected output.
	var out bytes.Buffer
	root.X.Context = tool.NewContext(tool.ContextOpts{Stdout: &out, Stderr: &out})
	if err := runTestList(root.X, []string{}); err != nil {
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
