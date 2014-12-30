package main

import (
	"reflect"
	"testing"
)

func TestJenkinsTestsToStart(t *testing.T) {
	testCases := []struct {
		projects            []string
		expectedJenkinsTest []string
	}{
		{
			projects: []string{"https://vanadium.googlesource.com/release.js.core"},
			expectedJenkinsTest: []string{
				"vanadium-js-browser-integration",
				"vanadium-js-build-extension",
				"vanadium-js-node-integration",
				"vanadium-js-unit",
				"vanadium-js-vdl",
				"vanadium-js-vom",
				"vanadium-namespace-browser-test",
			},
		},
		{
			projects: []string{"https://vanadium.googlesource.com/release.go.core"},
			expectedJenkinsTest: []string{
				"vanadium-go-build",
				"vanadium-namespace-browser-test",
			},
		},
	}

	for _, test := range testCases {
		got, err := jenkinsTestsToStart(test.projects)
		if err != nil {
			t.Fatalf("want no errors, got: %v", err)
		}
		if !reflect.DeepEqual(test.expectedJenkinsTest, got) {
			t.Fatalf("want %v, got %v", test.expectedJenkinsTest, got)
		}
	}
}
