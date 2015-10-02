// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"path/filepath"
	"time"

	"v.io/jiri/collect"
	"v.io/jiri/project"
	"v.io/jiri/runutil"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/test"
)

const (
	defaultPlaygroundTestTimeout = 5 * time.Minute
)

// vanadiumPlaygroundTest runs integration tests for the Vanadium playground.
//
// TODO(ivanpi): Port the namespace browser test logic from shell to Go. Add more tests.
func vanadiumPlaygroundTest(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	root, err := project.JiriRoot()
	if err != nil {
		return nil, err
	}

	// Initialize the test.
	// Need the new-stype base profile since many web tests will build
	// go apps that need it.
	cleanup, err := initTestX(ctx, testName, []string{"base"})
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	cleanup, err = initTest(ctx, testName, []string{"nodejs"})
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	playgroundDir := filepath.Join(root, "release", "projects", "playground")
	backendDir := filepath.Join(playgroundDir, "go", "src", "v.io", "x", "playground")
	clientDir := filepath.Join(playgroundDir, "client")

	// Clean the playground client build.
	if err := ctx.Run().Chdir(clientDir); err != nil {
		return nil, err
	}
	if err := ctx.Run().Command("make", "clean"); err != nil {
		return nil, err
	}

	// Run builder integration test.
	if testResult, err := vanadiumPlaygroundSubtest(ctx, testName, "builder integration", backendDir, "test"); testResult != nil || err != nil {
		return testResult, err
	}

	// Run client embedded example test.
	if testResult, err := vanadiumPlaygroundSubtest(ctx, testName, "client embedded example", clientDir, "test"); testResult != nil || err != nil {
		return testResult, err
	}

	return &test.Result{Status: test.Passed}, nil
}

// Runs specified make target in the specified directory as a test case.
// On success, both return values are nil.
func vanadiumPlaygroundSubtest(ctx *tool.Context, testName, caseName, casePath, caseTarget string) (tr *test.Result, err error) {
	if err = ctx.Run().Chdir(casePath); err != nil {
		return
	}
	if err := ctx.Run().TimedCommand(defaultPlaygroundTestTimeout, "make", caseTarget); err != nil {
		if err == runutil.CommandTimedOutErr {
			return &test.Result{
				Status:       test.TimedOut,
				TimeoutValue: defaultPlaygroundTestTimeout,
			}, nil
		} else {
			return nil, internalTestError{err, "Make " + caseTarget}
		}
	}
	return
}
