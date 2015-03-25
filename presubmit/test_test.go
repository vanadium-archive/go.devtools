// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"reflect"
	"testing"
)

func TestParseCLs(t *testing.T) {
	type testCase struct {
		refs        string
		projects    string
		expectErr   bool
		expectedCLs []cl
	}
	testCases := []testCase{
		// Single ref and project.
		testCase{
			refs:      "refs/changes/10/1000/1",
			projects:  "release.go.core",
			expectErr: false,
			expectedCLs: []cl{
				cl{
					clNumber: 1000,
					patchset: 1,
					ref:      "refs/changes/10/1000/1",
					project:  "release.go.core",
				},
			},
		},

		// Multiple refs and projects.
		testCase{
			refs:      "refs/changes/10/1000/1:refs/changes/20/1020/1",
			projects:  "release.go.core:release.js.core",
			expectErr: false,
			expectedCLs: []cl{
				cl{
					clNumber: 1000,
					patchset: 1,
					ref:      "refs/changes/10/1000/1",
					project:  "release.go.core",
				},
				cl{
					clNumber: 1020,
					patchset: 1,
					ref:      "refs/changes/20/1020/1",
					project:  "release.js.core",
				},
			},
		},

		// len(refs) != len(project)
		testCase{
			refs:      "refs/changes/10/1000/1:refs/changes/20/1020/1",
			projects:  "release.go.core",
			expectErr: true,
		},
	}

	for _, test := range testCases {
		reviewTargetRefsFlag = test.refs
		projectsFlag = test.projects
		gotCLs, err := parseCLs()
		if test.expectErr && err == nil {
			t.Fatalf("want errors, got no errors")

		}
		if !test.expectErr && err != nil {
			t.Fatalf("want no errors, got: %v", err)
		}
		if err == nil {
			if !reflect.DeepEqual(test.expectedCLs, gotCLs) {
				t.Fatalf("want %#v, got %#v", test.expectedCLs, gotCLs)
			}
		}
	}
}
