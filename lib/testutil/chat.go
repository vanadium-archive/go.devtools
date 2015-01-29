package testutil

import (
	"path/filepath"
	"time"

	"v.io/tools/lib/collect"
	"v.io/tools/lib/runutil"
	"v.io/tools/lib/util"
)

const (
	defaultChatTestTimeout = 5 * time.Minute
)

// runTest is a helper for running the chat tests.
func runTest(ctx *util.Context, testName, target string, profiles []string) (_ *TestResult, e error) {
	// Initialize the test.
	cleanup, result, err := initTest(ctx, testName, profiles)
	if err != nil {
		return nil, err
	} else if result != nil {
		return result, nil
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Navigate to chat directory.
	root, err := util.VanadiumRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "projects", "chat")
	if err := ctx.Run().Chdir(testDir); err != nil {
		return nil, err
	}

	// Clean and run the test.
	if err := ctx.Run().Command("make", "clean"); err != nil {
		return nil, err
	}
	makeTargetFunc := func(opts runutil.Opts) error {
		return ctx.Run().TimedCommandWithOpts(defaultChatTestTimeout, opts, "make", target)
	}
	if testResult, err := genXUnitReportOnCmdError(ctx, testName, "Make "+target, "failure", makeTargetFunc); err != nil {
		return nil, err
	} else if testResult != nil {
		if testResult.Status == TestTimedOut {
			testResult.TimeoutValue = defaultJSTestTimeout
		}
		return testResult, nil
	}

	return &TestResult{Status: TestPassed}, nil
}

// vanadiumChatShellTest runs the tests for the chat shell client.
func vanadiumChatShellTest(ctx *util.Context, testName string) (*TestResult, error) {
	return runTest(ctx, testName, "test-shell", nil)
}

// vanadiumChatWebTest runs the tests for the chat web client.
func vanadiumChatWebTest(ctx *util.Context, testName string) (*TestResult, error) {
	return runTest(ctx, testName, "test-web", []string{"web"})
}
