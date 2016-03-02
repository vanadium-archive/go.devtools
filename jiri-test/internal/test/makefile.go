// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"time"

	"v.io/jiri"
	"v.io/jiri/collect"
	"v.io/jiri/runutil"
	"v.io/x/devtools/internal/test"
	"v.io/x/lib/envvar"
)

// runMakefileTest is a helper for running tests through make commands.
func runMakefileTest(jirix *jiri.X, testName, testDir, target string, env map[string]string, profiles []string, timeout time.Duration) (_ *test.Result, e error) {
	// Install base profile first, before any test-specific profiles.
	profiles = append([]string{"base"}, profiles...)

	// Initialize the test.
	cleanup, err := initTest(jirix, testName, profiles)
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	s := jirix.NewSeq()

	// Set up the environment
	merged := envvar.MergeMaps(jirix.Env(), env)

	// Navigate to project directory, run make clean and make target.
	err = s.Pushd(testDir).
		Verbose(true).
		Run("make", "clean").
		Verbose(true).
		Timeout(timeout).Env(merged).Last("make", target)
	if err != nil {
		if runutil.IsTimeout(err) {
			return &test.Result{
				Status:       test.TimedOut,
				TimeoutValue: timeout,
			}, nil
		} else {
			return nil, newInternalError(err, "Make "+target)
		}
	}

	return &test.Result{Status: test.Passed}, nil
}
