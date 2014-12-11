package testutil

import (
	"path/filepath"
	"time"

	"veyron.io/tools/lib/collect"
	"veyron.io/tools/lib/runutil"
	"veyron.io/tools/lib/util"
)

const (
	defaultWWWTestTimeout = 5 * time.Minute
)

func (t *testEnv) veyronWWW(ctx *util.Context, testName string) (_ *TestResult, e error) {
	root, err := util.VeyronRoot()
	if err != nil {
		return nil, err
	}

	// Initialize the test.
	cleanup, err := t.initTest(ctx, testName, []string{"web"})
	if err != nil {
		return nil, err
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	wwwDir := filepath.Join(root, "www")
	if err := ctx.Run().Chdir(wwwDir); err != nil {
		return nil, err
	}

	opts := t.setTestEnv(ctx.Run().Opts())
	if err := ctx.Run().CommandWithOpts(opts, "make", "clean"); err != nil {
		return nil, err
	}

	// Invoke "make test"
	if err := ctx.Run().TimedCommandWithOpts(defaultWWWTestTimeout, opts, "make", "test"); err != nil {
		if err == runutil.CommandTimedOutErr {
			return &TestResult{Status: TestTimedOut}, nil
		}
		return &TestResult{Status: TestFailed}, nil
	}

	return &TestResult{Status: TestPassed}, nil
}
