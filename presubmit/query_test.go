// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"v.io/jiri"
	"v.io/jiri/gerrit"
	"v.io/jiri/jiritest"
	"v.io/jiri/project"
	"v.io/jiri/tool"
	"v.io/x/devtools/tooldata"
)

func TestSendCLListsToPresubmitTest(t *testing.T) {
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()

	// Create a fake configuration file.
	config := tooldata.NewConfig(
		tooldata.ProjectTestsOpt(map[string][]string{
			"release.go.core": []string{"go", "javascript"},
		}),
		tooldata.ProjectTestsOpt(map[string][]string{
			"release.js.core": []string{"javascript"},
		}),
	)
	if err := tooldata.SaveConfig(fake.X, config); err != nil {
		t.Fatalf("%v", err)
	}

	clLists := []gerrit.CLList{
		gerrit.CLList{
			gerrit.GenCL(1000, 1, "release.js.core"),
		},
		gerrit.CLList{
			gerrit.GenCLWithMoreData(2000, 1, "release.js.core", gerrit.PresubmitTestTypeNone, "vj@google.com"),
		},
		gerrit.CLList{
			gerrit.GenCLWithMoreData(2010, 1, "release.js.core", gerrit.PresubmitTestTypeAll, "foo@bar.com"),
		},
		gerrit.CLList{
			gerrit.GenMultiPartCL(1001, 1, "release.js.core", "t", 1, 2),
			gerrit.GenMultiPartCL(1002, 1, "release.go.core", "t", 2, 2),
		},
		gerrit.CLList{
			gerrit.GenMultiPartCL(1003, 1, "release.js.core", "t", 1, 3),
			gerrit.GenMultiPartCL(1004, 1, "release.go.core", "t", 2, 3),
			gerrit.GenMultiPartCLWithMoreData(1005, 1, "release.go.core", "t", 3, 3, "foo@bar.com"),
		},
		gerrit.CLList{
			gerrit.GenCL(3000, 1, "non-existent-project"),
		},
		gerrit.CLList{
			gerrit.GenMultiPartCL(1005, 1, "release.js.core", "t", 1, 2),
			gerrit.GenMultiPartCL(1006, 1, "non-existent-project", "t", 2, 2),
		},
	}

	sender := clsSender{
		clLists: clLists,
		projects: project.Projects{
			project.ProjectKey("release.go.core"): project.Project{
				Name: "release.go.core",
			},
			project.ProjectKey("release.js.core"): project.Project{
				Name: "release.js.core",
			},
		},

		// Mock out the removeOutdatedBuilds function.
		removeOutdatedFn: func(jirix *jiri.X, cls clNumberToPatchsetMap) []error { return nil },

		// Mock out the addPresubmitTestBuild function.
		// It will return error for the first clList.
		addPresubmitFn: func(jirix *jiri.X, cls gerrit.CLList, tests []string) error {
			if reflect.DeepEqual(cls, clLists[0]) {
				return fmt.Errorf("err")
			} else {
				return nil
			}
		},

		// Mock out postMessage function.
		postMessageFn: func(jirix *jiri.X, message string, refs []string, success bool) error { return nil },
	}

	var buf bytes.Buffer
	f := false
	fake.X.Context = tool.NewContext(tool.ContextOpts{
		Stdout:  &buf,
		Stderr:  &buf,
		Verbose: &f,
	})
	if err := sender.sendCLListsToPresubmitTest(fake.X); err != nil {
		t.Fatalf("want no error, got: %v", err)
	}

	// Check output and return value.
	want := `[VANADIUM PRESUBMIT] FAIL: Add http://go/vcl/1000/1
[VANADIUM PRESUBMIT] addPresubmitTestBuild failed: err
[VANADIUM PRESUBMIT] SKIP: Add http://go/vcl/2000/1 (presubmit=none)
[VANADIUM PRESUBMIT] SKIP: Add http://go/vcl/2010/1 (non-google owner)
[VANADIUM PRESUBMIT] PASS: Add http://go/vcl/1001/1, http://go/vcl/1002/1
[VANADIUM PRESUBMIT] SKIP: Add http://go/vcl/1003/1, http://go/vcl/1004/1, http://go/vcl/1005/1 (non-google owner)
[VANADIUM PRESUBMIT] project="non-existent-project" (refs/changes/xx/3000/1) not found. Skipped.
[VANADIUM PRESUBMIT] SKIP: Empty CL set
[VANADIUM PRESUBMIT] project="non-existent-project" (refs/changes/xx/1006/1) not found. Skipped.
[VANADIUM PRESUBMIT] PASS: Add http://go/vcl/1005/1
`
	if got := buf.String(); want != got {
		t.Fatalf("GOT:\n%v\nWANT:\n%v", got, want)
	}
	if got, want := sender.clsSent, 3; got != want {
		t.Fatalf("numSentCLs: got %d, want %d", got, want)
	}
}

func TestGetTestsToRun(t *testing.T) {
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()

	// Create a fake configuration file.
	config := tooldata.NewConfig(
		tooldata.ProjectTestsOpt(map[string][]string{
			"release.go.core": []string{"go", "javascript"},
		}),
		tooldata.TestGroupsOpt(map[string][]string{
			"go": []string{"vanadium-go-build", "vanadium-go-test", "vanadium-go-race"},
		}),
		tooldata.TestPartsOpt(map[string][]string{
			"vanadium-go-race": []string{"v.io/x/ref/services/device/...", "v.io/x/ref/runtime/..."},
		}),
	)
	if err := tooldata.SaveConfig(fake.X, config); err != nil {
		t.Fatalf("%v", err)
	}

	expected := []string{
		"javascript",
		"vanadium-go-build",
		"vanadium-go-race-part0",
		"vanadium-go-race-part1",
		"vanadium-go-race-part2",
		"vanadium-go-test",
	}
	sender := clsSender{}
	got, err := sender.getTestsToRun(fake.X, []string{"release.go.core"})
	if err != nil {
		t.Fatalf("want no errors, got: %v", err)
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("want %v, got %v", expected, got)
	}
}

func TestIsBuildOutdated(t *testing.T) {
	type testCase struct {
		refs     string
		cls      clNumberToPatchsetMap
		outdated bool
	}
	testCases := []testCase{
		// Builds with a single ref.
		testCase{
			refs:     "refs/changes/10/1000/2",
			cls:      clNumberToPatchsetMap{1000: 2},
			outdated: true,
		},
		testCase{
			refs:     "refs/changes/10/1000/2",
			cls:      clNumberToPatchsetMap{1000: 1},
			outdated: false,
		},

		// Builds with multiple refs.
		//
		// Overlapping cls.
		testCase{
			refs:     "refs/changes/10/1001/2",
			cls:      clNumberToPatchsetMap{1001: 3, 2000: 2},
			outdated: true,
		},
		// The other case with overlapping cl.
		testCase{
			refs:     "refs/changes/10/1000/2:refs/changes/10/2000/2",
			cls:      clNumberToPatchsetMap{1001: 2, 2000: 2},
			outdated: true,
		},
		// Both refs don't match.
		testCase{
			refs:     "refs/changes/10/1000/2:refs/changes/10/2000/2",
			cls:      clNumberToPatchsetMap{1001: 2, 2000: 2},
			outdated: true,
		},
		// Both patchsets in "cls" are smaller.
		testCase{
			refs:     "refs/changes/10/1000/2:refs/changes/10/2000/2",
			cls:      clNumberToPatchsetMap{1000: 1, 2000: 1},
			outdated: false,
		},
		// One of the patchsets in "cls" is larger than the one in "refs".
		testCase{
			refs:     "refs/changes/10/1000/2:refs/changes/10/2000/2",
			cls:      clNumberToPatchsetMap{1000: 3, 2000: 2},
			outdated: true,
		},
		// Both patchsets in "cls" are the same as the ones in "refs".
		testCase{
			refs:     "refs/changes/10/1000/2:refs/changes/10/2000/2",
			cls:      clNumberToPatchsetMap{1000: 2, 2000: 2},
			outdated: true,
		},
	}

	for i, test := range testCases {
		outdated, err := isBuildOutdated(test.refs, test.cls)
		if err != nil {
			t.Fatalf("want no errors, got: %v", err)
		}
		if expected, got := test.outdated, outdated; expected != got {
			t.Fatalf("%d: want %v, got %v", i, expected, got)
		}
	}
}

func TestParseRefString(t *testing.T) {
	type testCase struct {
		ref              string
		expectErr        bool
		expectedCL       int
		expectedPatchSet int
	}
	testCases := []testCase{
		// Normal case
		testCase{
			ref:              "ref/changes/12/3412/2",
			expectedCL:       3412,
			expectedPatchSet: 2,
		},
		// Error cases
		testCase{
			ref:       "ref/123",
			expectErr: true,
		},
		testCase{
			ref:       "ref/changes/12/a/2",
			expectErr: true,
		},
		testCase{
			ref:       "ref/changes/12/3412/a",
			expectErr: true,
		},
	}
	for _, test := range testCases {
		cl, patchset, err := parseRefString(test.ref)
		if test.expectErr {
			if err == nil {
				t.Fatalf("want errors, got: %v", err)
			}
		} else {
			if err != nil {
				t.Fatalf("want no errors, got: %v", err)
			}
			if cl != test.expectedCL {
				t.Fatalf("want %v, got %v", test.expectedCL, cl)
			}
			if patchset != test.expectedPatchSet {
				t.Fatalf("want %v, got %v", test.expectedPatchSet, patchset)
			}
		}
	}
}
