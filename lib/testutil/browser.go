package testutil

import (
	"path/filepath"
	"time"

	"veyron.io/tools/lib/collect"
	"veyron.io/tools/lib/runutil"
	"veyron.io/tools/lib/util"
)

const (
	defaultBrowserTestTimeout = 5 * time.Minute
)

// veyronBrowserTest runs an integration test for the veyron browser.
//
// TODO(aghassemi): Port the veyron browser test logic from shell to Go.
func (t *testEnv) veyronBrowserTest(ctx *util.Context, testName string) (_ *TestResult, e error) {
	root, err := util.VeyronRoot()
	if err != nil {
		return nil, err
	}
	xUnitFile := XUnitReportPath(testName)

	// Initialize the test.
	cleanup, err := t.initTest(ctx, testName, []string{"web"})
	if err != nil {
		return nil, err
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Invoke "make clean" for the veyron browser and remove the test output file if it exists.
	browserDir := filepath.Join(root, "veyron-browser")
	if err := ctx.Run().Chdir(browserDir); err != nil {
		return nil, err
	}
	if err := ctx.Run().CommandWithOpts(t.setTestEnv(ctx.Run().Opts()), "make", "clean"); err != nil {
		return nil, err
	}
	if err := ctx.Run().RemoveAll(xUnitFile); err != nil {
		return nil, err
	}

	// Invoke "make test" for the veyron browser.
	opts := t.setTestEnv(ctx.Run().Opts())
	opts.Env["XUNIT_OUTPUT_FILE"] = xUnitFile
	if err := ctx.Run().TimedCommandWithOpts(defaultBrowserTestTimeout, opts, "make", "test"); err != nil {
		if err == runutil.CommandTimedOutErr {
			return &TestResult{Status: TestTimedOut}, nil
		}
		return &TestResult{Status: TestFailed}, nil
	}

	return &TestResult{Status: TestPassed}, nil
}
