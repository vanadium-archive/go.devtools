// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"path/filepath"
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
	return runMakefileTest(jirix, testName, testDir, "build", map[string]string{"ANDROID": "1"}, []string{"v23:mojo", "v23:android"}, defaultMojoTestTimeout)
}

// vanadiumMojoSyncbaseTest runs the tests for the Vanadium Mojo Syncbase service.
func vanadiumMojoSyncbaseTest(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "mojo", "syncbase")
	return runMakefileTest(jirix, testName, testDir, "test", nil, []string{"v23:dart", "v23:mojo"}, defaultMojoTestTimeout)
}

// vanadiumMojoV23ProxyUnitTest runs the unit tests for the vanadium <-> mojo "v23proxy"
func vanadiumMojoV23ProxyUnitTest(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "mojo", "v23proxy")
	return runMakefileTest(jirix, testName, testDir, "test-unit", nil, []string{"v23:base", "v23:mojo", "v23:dart"}, defaultMojoTestTimeout)
}

// vanadiumMojoV23ProxyIntegrationTest runs the integration tests for the vanadium <-> mojo "v23proxy"
func vanadiumMojoV23ProxyIntegrationTest(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "mojo", "v23proxy")
	return runMakefileTest(jirix, testName, testDir, "test-integration", nil, []string{"v23:base", "v23:mojo", "v23:dart"}, defaultMojoTestTimeout)
}
