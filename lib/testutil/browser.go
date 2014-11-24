package testutil

import (
	"path/filepath"

	"veyron.io/tools/lib/envutil"
	"veyron.io/tools/lib/runutil"
	"veyron.io/tools/lib/util"
)

// VeyronBrowserTest runs an integration test for the veyron browser.
//
// TODO(aghassemi): Port the veyron browser test logic from shell to Go.
func VeyronBrowserTest(ctx *util.Context, testName string) (*TestResult, error) {
	root, err := util.VeyronRoot()
	if err != nil {
		return nil, err
	}
	provaOutputFile := filepath.Join(root, "veyron_browser_test.out")

	// Initialize the test.
	cleanup, err := initTest(ctx, testName, []string{"web"})
	if err != nil {
		return nil, err
	}
	defer cleanup()

	// Invoke "make clean" for the veyron browser and remove the test output file if it exists.
	browserDir := filepath.Join(root, "veyron-browser")
	if err := ctx.Run().Function(runutil.Chdir(browserDir)); err != nil {
		return nil, err
	}
	if err := ctx.Run().Command("make", "clean"); err != nil {
		return nil, err
	}
	if err := ctx.Run().Function(runutil.RemoveAll(provaOutputFile)); err != nil {
		return nil, err
	}

	// Invoke "make test" for the veyron browser.
	opts := ctx.Run().Opts()
	env := envutil.NewSnapshotFromOS()
	env.Set("PROVA_OUTPUT_FILE", provaOutputFile)
	opts.Env = env.Map()
	if err := ctx.Run().CommandWithOpts(opts, "make", "test"); err != nil {
		return &TestResult{Status: TestFailed}, nil
	}

	return &TestResult{Status: TestPassed}, nil
}
