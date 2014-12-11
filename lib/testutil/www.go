package testutil

import (
	"path/filepath"

	"veyron.io/tools/lib/collect"
	"veyron.io/tools/lib/util"
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
	if err := ctx.Run().CommandWithOpts(opts, "make", "test"); err != nil {
		return &TestResult{Status: TestFailed}, nil
	}

	return &TestResult{Status: TestPassed}, nil
}
