// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"v.io/jiri"
	"v.io/x/devtools/internal/test"
)

const (
	defaultMojoTestTimeout = 10 * time.Minute
)

// vanadiumMojoDiscoveryTest runs the tests for the Vanadium Mojo Discovery service.
func vanadiumMojoDiscoveryTest(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "mojo", "discovery")
	if r, err := runMakefileTest(jirix, testName, testDir, "test", nil, []string{"v23:mojo", "v23:dart"}, defaultMojoTestTimeout); err != nil {
		return r, err
	}
	// For android, just make sure that everything builds.
	initExtraDeps := func() (func() error, error) {
		s := jirix.NewSeq()

		args := []string{"-v", "profile-v23", "install", "--target=arm-android", "v23:mojo"}
		fmt.Fprintf(jirix.Stdout(), "Running: jiri %s\n", strings.Join(args, " "))
		if err := s.Last("jiri", args...); err != nil {
			return nil, fmt.Errorf("jiri %v: %v", strings.Join(args, " "), err)
		}
		fmt.Fprintf(jirix.Stdout(), "jiri %v: success\n", strings.Join(args, " "))

		args = []string{"profile", "update"}
		if err := s.Capture(os.Stdout, os.Stderr).Last("jiri", args...); err != nil {
			return nil, fmt.Errorf("jiri %v: %v", strings.Join(args, " "), err)
		}
		fmt.Fprintf(jirix.Stdout(), "jiri %v: success\n", strings.Join(args, " "))

		return func() error { return nil }, nil
	}
	return runMakefileTestWithExtraDeps(jirix, testName, testDir, "build", map[string]string{"ANDROID": "1"}, []string{"v23:android", "v23:mojo", "v23:dart"}, initExtraDeps, defaultMojoTestTimeout)
}

// vanadiumMojoSyncbaseTest runs the tests for the Vanadium Mojo Syncbase service.
func vanadiumMojoSyncbaseTest(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "mojo", "syncbase")
	return runMakefileTest(jirix, testName, testDir, "test", nil, []string{"v23:mojo", "v23:dart"}, defaultMojoTestTimeout)
}

// vanadiumMojoV23ProxyUnitTest runs the unit tests for the vanadium <-> mojo "v23proxy"
func vanadiumMojoV23ProxyUnitTest(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "mojo", "v23proxy")
	return runMakefileTest(jirix, testName, testDir, "test-unit", nil, []string{"v23:mojo", "v23:dart"}, defaultMojoTestTimeout)
}

// vanadiumMojoV23ProxyIntegrationTest runs the integration tests for the vanadium <-> mojo "v23proxy"
func vanadiumMojoV23ProxyIntegrationTest(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "mojo", "v23proxy")
	return runMakefileTest(jirix, testName, testDir, "test-integration", nil, []string{"v23:mojo", "v23:dart"}, defaultMojoTestTimeout)
}
