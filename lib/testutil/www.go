package testutil

import (
	"path/filepath"
	"time"

	"v.io/tools/lib/collect"
	"v.io/tools/lib/runutil"
	"v.io/tools/lib/util"
)

const (
	defaultWWWTestTimeout           = 5 * time.Minute
	defaultWWWPlaygroundTestTimeout = 5 * time.Minute
)

// Runs specified make target in WWW Makefile as a test.
func commonVanadiumWWW(ctx *util.Context, testName, makeTarget string, timeout time.Duration) (_ *TestResult, e error) {
	root, err := util.VanadiumRoot()
	if err != nil {
		return nil, err
	}

	// Initialize the test.
	cleanup, testResult, err := initTest(ctx, testName, []string{"web"})
	if err != nil {
		return nil, err
	} else if testResult != nil {
		return testResult, nil
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	wwwDir := filepath.Join(root, "www")
	if err := ctx.Run().Chdir(wwwDir); err != nil {
		return nil, err
	}

	if err := ctx.Run().Command("make", "clean"); err != nil {
		return nil, err
	}

	// Invoke the make target.
	if err := ctx.Run().TimedCommand(timeout, "make", makeTarget); err != nil {
		if err == runutil.CommandTimedOutErr {
			return &TestResult{
				Status:       TestTimedOut,
				TimeoutValue: timeout,
			}, nil
		}
		return &TestResult{Status: TestFailed}, nil
	}

	return &TestResult{Status: TestPassed}, nil
}

func vanadiumWWWPlayground(ctx *util.Context, testName string) (*TestResult, error) {
	return commonVanadiumWWW(ctx, testName, "test-playground", defaultWWWPlaygroundTestTimeout)
}

func vanadiumWWWSite(ctx *util.Context, testName string) (*TestResult, error) {
	return commonVanadiumWWW(ctx, testName, "test-site", defaultWWWTestTimeout)
}

func vanadiumWWWTutorials(ctx *util.Context, testName string) (*TestResult, error) {
	return commonVanadiumWWW(ctx, testName, "test-tutorials", defaultWWWTestTimeout)
}
