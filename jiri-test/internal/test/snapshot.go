// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"path/filepath"

	"v.io/jiri/collect"
	"v.io/jiri/jiri"
	"v.io/jiri/retry"
	"v.io/x/devtools/internal/test"
)

// vanadiumGoSnapshot create a snapshot of Vanadium Go code base.
func vanadiumGoSnapshot(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(jirix, testName, nil)
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// TODO(nlacasse): Are we going to continue storing snapshots here?  Maybe
	// we need some configuation to tell us where these should be, so we don't
	// need to hard-code this path.
	manifestDir := filepath.Join(jirix.Root, "manifest")
	snapshotDir := filepath.Join(manifestDir, "snapshot")

	// Create a new snapshot.
	fn := func() error {
		return jirix.NewSeq().Last("jiri",
			"snapshot",
			"--dir="+snapshotDir,
			"create",
			"-push-remote",
			"stable-go")
	}
	if err := retry.Function(jirix.Context, fn); err != nil {
		return nil, newInternalError(err, "Snapshot")
	}
	return &test.Result{Status: test.Passed}, nil
}
