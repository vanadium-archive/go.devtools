package testutil

import (
	"path/filepath"
	"time"

	"v.io/tools/lib/collect"
	"v.io/tools/lib/runutil"
	"v.io/tools/lib/util"
)

const (
	defaultWWWTestTimeout = 5 * time.Minute
)

func vanadiumWWW(ctx *util.Context, testName string) (_ *TestResult, e error) {
	root, err := util.VanadiumRoot()
	if err != nil {
		return nil, err
	}

	// Initialize the test.
	cleanup, err := initTest(ctx, testName, []string{"web"})
	if err != nil {
		return nil, err
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	wwwDir := filepath.Join(root, "www")
	if err := ctx.Run().Chdir(wwwDir); err != nil {
		return nil, err
	}

	if err := ctx.Run().Command("make", "clean"); err != nil {
		return nil, err
	}

	// Invoke "make test"
	if err := ctx.Run().TimedCommand(defaultWWWTestTimeout, "make", "test"); err != nil {
		if err == runutil.CommandTimedOutErr {
			return &TestResult{
				Status:       TestTimedOut,
				TimeoutValue: defaultWWWTestTimeout,
			}, nil
		}
		return &TestResult{Status: TestFailed}, nil
	}

	return &TestResult{Status: TestPassed}, nil
}
