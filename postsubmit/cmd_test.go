package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"v.io/tools/lib/util"
)

func TestJenkinsTestsToStart(t *testing.T) {
	// Setup a fake VANADIUM_ROOT.
	ctx := util.DefaultContext()
	tmpDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer ctx.Run().RemoveAll(tmpDir)
	oldRoot, err := util.VanadiumRoot()
	if err := os.Setenv("VANADIUM_ROOT", tmpDir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("VANADIUM_ROOT", oldRoot)

	// Create a test config file.
	configFile, err := util.ConfigFile("common")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := ctx.Run().MkdirAll(filepath.Dir(configFile), os.FileMode(0755)); err != nil {
		t.Fatalf("%v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := ctx.Run().Symlink(filepath.Join(cwd, "testdata", "common.json"), configFile); err != nil {
		t.Fatalf("%v", err)
	}

	testCases := []struct {
		projects            []string
		expectedJenkinsTest []string
	}{
		{
			projects: []string{"release.js.core"},
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
			projects: []string{"release.go.core"},
			expectedJenkinsTest: []string{
				"vanadium-go-build",
				"vanadium-namespace-browser-test",
				"vanadium-www-tutorials",
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
