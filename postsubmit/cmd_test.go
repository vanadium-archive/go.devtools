// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"reflect"
	"testing"

	"v.io/jiri/project"
	"v.io/jiri/tool"
	"v.io/jiri/util"
)

func TestJenkinsTestsToStart(t *testing.T) {
	ctx := tool.NewDefaultContext()
	root, err := project.NewFakeJiriRoot(ctx)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer func() {
		if err := root.Cleanup(ctx); err != nil {
			t.Fatalf("%v", err)
		}
	}()

	// Point the V23_ROOT environment variable to the fake.
	oldRoot, err := project.JiriRoot()
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := os.Setenv("V23_ROOT", root.Dir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("V23_ROOT", oldRoot)

	// Create a fake configuration file.
	config := util.NewConfig(
		util.ProjectTestsOpt(map[string][]string{
			"release.go.core": []string{"go", "javascript"},
			"release.js.core": []string{"javascript"},
		}),
		util.TestGroupsOpt(map[string][]string{
			"go":         []string{"vanadium-go-build", "vanadium-go-test", "vanadium-go-race"},
			"javascript": []string{"vanadium-js-integration", "vanadium-js-unit"},
		}),
	)
	if err := util.SaveConfig(ctx, config); err != nil {
		t.Fatalf("%v", err)
	}

	testCases := []struct {
		projects            []string
		expectedJenkinsTest []string
	}{
		{
			projects: []string{"release.go.core"},
			expectedJenkinsTest: []string{
				"vanadium-go-build",
				"vanadium-go-race",
				"vanadium-go-test",
				"vanadium-js-integration",
				"vanadium-js-unit",
			},
		},
		{
			projects: []string{"release.js.core"},
			expectedJenkinsTest: []string{
				"vanadium-js-integration",
				"vanadium-js-unit",
			},
		},
	}

	for _, test := range testCases {
		got, err := jenkinsTestsToStart(ctx, test.projects)
		if err != nil {
			t.Fatalf("want no errors, got: %v", err)
		}
		if !reflect.DeepEqual(test.expectedJenkinsTest, got) {
			t.Fatalf("want %v, got %v", test.expectedJenkinsTest, got)
		}
	}
}
