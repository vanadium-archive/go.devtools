// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"path/filepath"
	"time"

	"v.io/jiri/collect"
	"v.io/jiri/jiri"
	"v.io/jiri/runutil"
	"v.io/x/devtools/internal/test"
)

const (
	defaultPlaygroundTestTimeout = 10 * time.Minute
)

// vanadiumPlaygroundTest runs integration tests for the Vanadium playground.
//
// TODO(ivanpi): Port the namespace browser test logic from shell to Go. Add more tests.
func vanadiumPlaygroundTest(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	// Need the new-stype base profile since many web tests will build
	// go apps that need it.
	cleanup, err := initTest(jirix, testName, []string{"base", "nodejs"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	playgroundDir := filepath.Join(jirix.Root, "release", "projects", "playground")
	backendDir := filepath.Join(playgroundDir, "go", "src", "v.io", "x", "playground")
	clientDir := filepath.Join(playgroundDir, "client")

	// Clean the playground client build.
	if err := jirix.Run().Chdir(clientDir); err != nil {
		return nil, err
	}
	if err := jirix.Run().Command("make", "clean"); err != nil {
		return nil, err
	}

	// Run builder integration test.
	if testResult, err := vanadiumPlaygroundSubtest(jirix, testName, "builder integration", backendDir, "test"); testResult != nil || err != nil {
		return testResult, err
	}

	// Run client embedded example test.
	if testResult, err := vanadiumPlaygroundSubtest(jirix, testName, "client embedded example", clientDir, "test"); testResult != nil || err != nil {
		return testResult, err
	}

	return &test.Result{Status: test.Passed}, nil
}

// Runs specified make target in the specified directory as a test case.
// On success, both return values are nil.
func vanadiumPlaygroundSubtest(jirix *jiri.X, testName, caseName, casePath, caseTarget string) (tr *test.Result, err error) {
	if err = jirix.Run().Chdir(casePath); err != nil {
		return
	}
	if err := jirix.Run().TimedCommand(defaultPlaygroundTestTimeout, "make", caseTarget); err != nil {
		if runutil.IsTimeout(err) {
			return &test.Result{
				Status:       test.TimedOut,
				TimeoutValue: defaultPlaygroundTestTimeout,
			}, nil
		} else {
			return nil, newInternalError(err, "Make "+caseTarget)
		}
	}
	return
}
