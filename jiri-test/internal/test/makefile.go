// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"time"

	"v.io/jiri/collect"
	"v.io/jiri/jiri"
	"v.io/jiri/runutil"
	"v.io/x/devtools/internal/test"
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

	// Navigate to project directory.
	if err := jirix.Run().Chdir(testDir); err != nil {
		return nil, err
	}

	// Clean.
	if err := jirix.Run().Command("make", "clean"); err != nil {
		return nil, err
	}

	// Set environment from the env argument map.
	opts := jirix.Run().Opts()
	for k, v := range env {
		opts.Env[k] = v
	}

	// Run the tests.
	if err := jirix.Run().TimedCommandWithOpts(timeout, opts, "make", target); err != nil {
		if err == runutil.CommandTimedOutErr {
			return &test.Result{
				Status:       test.TimedOut,
				TimeoutValue: timeout,
			}, nil
		} else {
			return nil, newInternalError(err, "Make " + target)
		}
	}

	return &test.Result{Status: test.Passed}, nil
}
