// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"path/filepath"
	"time"

	"v.io/jiri/jiri"
	"v.io/x/devtools/internal/test"
)

const (
	defaultMojoTestTimeout = 10 * time.Minute
)

// vanadiumMojoSyncbaseTest runs the tests for the Vanadium Mojo Syncbase
// service.
func vanadiumMojoSyncbaseTest(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "mojo", "syncbase")
	return runMakefileTest(jirix, testName, testDir, "test", nil, []string{"dart", "mojo"}, defaultMojoTestTimeout)
}
