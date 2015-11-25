// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"reflect"
	"testing"

	"v.io/jiri/gerrit"
	"v.io/jiri/jiritest"
)

func TestGenStartPresubmitBuildLink(t *testing.T) {
	strRefs := "refs/changes/12/1012/1:refs/changes/13/1013/1"
	strProjects := "release.go.core:release.go.tools"
	strTests := "vanadium-go-test vanadium-go-race"

	got := genStartPresubmitBuildLink(strRefs, strProjects, strTests)
	want := "https://veyron.corp.google.com/jenkins/job/vanadium-presubmit-test/buildWithParameters?REFS=refs%2Fchanges%2F12%2F1012%2F1%3Arefs%2Fchanges%2F13%2F1013%2F1&PROJECTS=release.go.core%3Arelease.go.tools&TESTS=vanadium-go-test+vanadium-go-race"
	if want != got {
		t.Fatalf("\nwant:\n%s\n\ngot:\n%s\n", want, got)
	}
}

func TestGetSubmittableCLs(t *testing.T) {
	jirix, cleanup := jiritest.NewX(t)
	defer cleanup()

	cls := clList{
		// cls[0]:
		// CL without AutoSubmit label.
		gerrit.Change{},

		// cls[1]:
		// CL with AutoSubmit label and Verified approved, but Code-Review
		// is not approved.
		gerrit.Change{
			Labels: map[string]map[string]interface{}{
				"Code-Review": map[string]interface{}{
					"rejected": struct{}{},
				},
				"Verified": map[string]interface{}{
					"approved": struct{}{},
				},
			},
			AutoSubmit: true,
		},

		// cls[2]:
		// CL with AutoSubmit label and Code-Review approved, but Verified
		// is not approved.
		gerrit.Change{
			Labels: map[string]map[string]interface{}{
				"Code-Review": map[string]interface{}{
					"approved": struct{}{},
				},
				"Verified": map[string]interface{}{
					"rejected": struct{}{},
				},
			},
			AutoSubmit: true,
		},

		// cls[3]:
		// CL with AutoSubmit label and Code-Review approved, but no Verified label.
		gerrit.Change{
			Labels: map[string]map[string]interface{}{
				"Code-Review": map[string]interface{}{
					"approved": struct{}{},
				},
				"Verified": map[string]interface{}{},
			},
			AutoSubmit: true,
		},

		// cls[4]:
		// A submittable CL.
		gerrit.Change{
			Labels: map[string]map[string]interface{}{
				"Code-Review": map[string]interface{}{
					"approved": struct{}{},
				},
				"Verified": map[string]interface{}{
					"approved": struct{}{},
				},
			},
			AutoSubmit: true,
		},

		// cls[5]:
		// Another submittable CL which doesn't have Verified label configured.
		gerrit.Change{
			Labels: map[string]map[string]interface{}{
				"Code-Review": map[string]interface{}{
					"approved": struct{}{},
				},
			},
			AutoSubmit: true,
		},

		// cls[6]:
		// MultiPart CL 1 part 1 with everything approved.
		gerrit.Change{
			Labels: map[string]map[string]interface{}{
				"Code-Review": map[string]interface{}{
					"approved": struct{}{},
				},
				"Verified": map[string]interface{}{
					"approved": struct{}{},
				},
			},
			MultiPart: &gerrit.MultiPartCLInfo{
				Topic: "test",
				Index: 1,
				Total: 2,
			},
			AutoSubmit: true,
		},

		// cls[7]:
		// MultiPart CL 1 part 2 with Verified rejected.
		gerrit.Change{
			Labels: map[string]map[string]interface{}{
				"Code-Review": map[string]interface{}{
					"approved": struct{}{},
				},
				"Verified": map[string]interface{}{
					"rejected": struct{}{},
				},
			},
			MultiPart: &gerrit.MultiPartCLInfo{
				Topic: "test",
				Index: 2,
				Total: 2,
			},
			AutoSubmit: true,
		},

		// cls[8]:
		// MultiPart CL 1 part 2 everything approved.
		gerrit.Change{
			Labels: map[string]map[string]interface{}{
				"Code-Review": map[string]interface{}{
					"approved": struct{}{},
				},
				"Verified": map[string]interface{}{
					"approved": struct{}{},
				},
			},
			MultiPart: &gerrit.MultiPartCLInfo{
				Topic: "test",
				Index: 2,
				Total: 2,
			},
			AutoSubmit: true,
		},

		// cls[9]:
		// MultiPart CL 2 part 1.
		gerrit.Change{
			Labels: map[string]map[string]interface{}{
				"Code-Review": map[string]interface{}{
					"approved": struct{}{},
				},
				"Verified": map[string]interface{}{
					"approved": struct{}{},
				},
			},
			MultiPart: &gerrit.MultiPartCLInfo{
				Topic: "test2",
				Index: 1,
				Total: 2,
			},
			AutoSubmit: true,
		},

		// cls[10]:
		// MultiPart CL 2 part 2.
		gerrit.Change{
			Labels: map[string]map[string]interface{}{
				"Code-Review": map[string]interface{}{
					"approved": struct{}{},
				},
				"Verified": map[string]interface{}{
					"approved": struct{}{},
				},
			},
			MultiPart: &gerrit.MultiPartCLInfo{
				Topic: "test2",
				Index: 2,
				Total: 2,
			},
			AutoSubmit: true,
		},
	}
	testCases := []struct {
		cls                    clList
		expectedSubmittableCLs []clList
	}{
		// Test non-multipart CLs.
		{
			cls: clList{cls[0], cls[1], cls[2], cls[3], cls[4], cls[5]},
			expectedSubmittableCLs: []clList{clList{cls[4]}, clList{cls[5]}},
		},
		// Test multi-part CLs with one of them being unsubmittable.
		{
			cls: clList{cls[6], cls[7]},
			expectedSubmittableCLs: []clList{},
		},
		// Test multi-part CLs where all CLs are submittable.
		{
			cls: clList{cls[6], cls[8]},
			expectedSubmittableCLs: []clList{clList{cls[6], cls[8]}},
		},
		// Test multiple submittable multi-part CLs.
		{
			cls: clList{cls[6], cls[8], cls[9], cls[10]},
			expectedSubmittableCLs: []clList{clList{cls[6], cls[8]}, clList{cls[9], cls[10]}},
		},
		// Mixed CLs.
		{
			cls: clList{cls[0], cls[4], cls[5], cls[6], cls[7]},
			expectedSubmittableCLs: []clList{clList{cls[4]}, clList{cls[5]}},
		},
		{
			cls: clList{cls[0], cls[4], cls[5], cls[6], cls[8], cls[9], cls[10]},
			expectedSubmittableCLs: []clList{clList{cls[4]}, clList{cls[5]}, clList{cls[6], cls[8]}, clList{cls[9], cls[10]}},
		},
	}
	for index, test := range testCases {
		if got, want := getSubmittableCLs(jirix, test.cls), test.expectedSubmittableCLs; !reflect.DeepEqual(got, want) {
			t.Fatalf("#%d: want:\n%#v\n\ngot:\n%#v", index, want, got)
		}
	}
}

func TestTestNameWithPartSuffix(t *testing.T) {
	testCases := []struct {
		testName  string
		partIndex int
		expected  string
	}{
		{
			testName:  "vanadium-go-race",
			partIndex: 0,
			expected:  "vanadium-go-race-part0",
		},
	}
	for _, test := range testCases {
		if got, want := testNameWithPartSuffix(test.testName, test.partIndex), test.expected; got != want {
			t.Fatalf("want:%s, got:%s", want, got)
		}
	}
}
