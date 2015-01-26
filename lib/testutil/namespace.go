package testutil

import (
	"path/filepath"
	"time"

	"v.io/tools/lib/collect"
	"v.io/tools/lib/runutil"
	"v.io/tools/lib/util"
)

const (
	defaultBrowserTestTimeout = 5 * time.Minute
)

// vanadiumBrowserTest runs an integration test for the vanadium
// namespace browser.
//
// TODO(aghassemi): Port the namespace browser test logic from shell to Go.
func vanadiumNamespaceBrowserTest(ctx *util.Context, testName string) (_ *TestResult, e error) {
	root, err := util.VanadiumRoot()
	if err != nil {
		return nil, err
	}
	xUnitFile := XUnitReportPath(testName)

	// Initialize the test.
	cleanup, result, err := initTest(ctx, testName, []string{"web"})
	if err != nil {
		return nil, err
	} else if result != nil {
		return result, nil
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Invoke "make clean" for the vanadium namespace browser and remove
	// the test output file if it exists.
	browserDir := filepath.Join(root, "release", "projects", "namespace_browser")
	if err := ctx.Run().Chdir(browserDir); err != nil {
		return nil, err
	}
	if err := ctx.Run().Command("make", "clean"); err != nil {
		return nil, err
	}
	if err := ctx.Run().RemoveAll(xUnitFile); err != nil {
		return nil, err
	}

	// Invoke "make test" for the vanadium namepsace browser.
	makeTargetFunc := func(opts runutil.Opts) error {
		opts.Env["XUNIT_OUTPUT_FILE"] = xUnitFile
		return ctx.Run().TimedCommandWithOpts(defaultBrowserTestTimeout, opts, "make", "test")
	}
	if testResult, err := genXUnitReportOnCmdError(ctx, testName, "Make test", "failure", makeTargetFunc); err != nil {
		return nil, err
	} else if testResult != nil {
		if testResult.Status == TestTimedOut {
			testResult.TimeoutValue = defaultBrowserTestTimeout
		}
		return testResult, nil
	}

	return &TestResult{Status: TestPassed}, nil
}
