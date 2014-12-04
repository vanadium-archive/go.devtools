package testutil

import (
	"path/filepath"

	"veyron.io/tools/lib/envutil"
	"veyron.io/tools/lib/util"
)

// runJSTest is a harness for executing javascript tests.
func runJSTest(ctx *util.Context, testName, testDir, target string, cleanFn func() error, env map[string]string) (*TestResult, error) {
	// Initialize the test.
	cleanup, err := initTest(ctx, testName, []string{"web"})
	if err != nil {
		return nil, err
	}
	defer cleanup()

	// Navigate to the target directory.
	if err := ctx.Run().Chdir(testDir); err != nil {
		return nil, err
	}

	// Clean up after previous instances of the test.
	if err := ctx.Run().Command("make", "clean"); err != nil {
		return nil, err
	}
	if cleanFn != nil {
		if err := cleanFn(); err != nil {
			return nil, err
		}
	}

	// Run the test target.
	opts := ctx.Run().Opts()
	osEnv := envutil.NewSnapshotFromOS()
	for key, value := range env {
		osEnv.Set(key, value)
	}
	opts.Env = osEnv.Map()
	if err := ctx.Run().CommandWithOpts(opts, "make", target); err != nil {
		return &TestResult{Status: TestFailed}, nil
	}

	return &TestResult{Status: TestPassed}, nil
}

// VeyronJSBuildExtension tests the veyron javascript build extension.
func VeyronJSBuildExtension(ctx *util.Context, testName string) (*TestResult, error) {
	root, err := util.VeyronRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "veyron.js")
	target := "extension/veyron.crx"
	return runJSTest(ctx, testName, testDir, target, nil, nil)
}

// VeyronJSDoc (re)generates the content of the veyron javascript
// documentation server.
func VeyronJSDoc(ctx *util.Context, testName string) (*TestResult, error) {
	root, err := util.VeyronRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "veyron.js")
	target := "docs"
	webDir, jsDocDir := "/var/www/jsdoc", filepath.Join(testDir, "docs")
	cleanFn := func() error {
		if err := ctx.Run().RemoveAll(webDir); err != nil {
			return err
		}
		return nil
	}
	result, err := runJSTest(ctx, testName, testDir, target, cleanFn, nil)
	if err != nil {
		return nil, err
	}
	// Move generated js documentation to the web server directory
	// using "mv" instead of "os.Rename()" to account for the fact
	// the the source and the destination may be on different
	// partitions.
	if err := ctx.Run().Command("mv", jsDocDir, webDir); err != nil {
		return nil, err
	}
	return result, nil
}

// VeyronJSIntegrationTest runs the veyron javascript integration test.
func VeyronJSIntegrationTest(ctx *util.Context, testName string) (*TestResult, error) {
	root, err := util.VeyronRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "veyron.js")
	target := "test-integration"
	env := map[string]string{}
	env["TAP"] = "true"
	env["NODE_OUTPUT"] = "integration_test.tap"
	return runJSTest(ctx, testName, testDir, target, nil, env)
}

// VeyronJSUnitTest runs the veyron javascript unit test.
func VeyronJSUnitTest(ctx *util.Context, testName string) (*TestResult, error) {
	root, err := util.VeyronRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "veyron.js")
	target := "test-unit"
	env := map[string]string{}
	env["TAP"] = "true"
	env["NODE_OUTPUT"] = "unit_test.tap"
	return runJSTest(ctx, testName, testDir, target, nil, env)
}

// VeyronJSVdlTest runs the veyron javascript vdl test.
func VeyronJSVdlTest(ctx *util.Context, testName string) (*TestResult, error) {
	root, err := util.VeyronRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "veyron.js")
	target := "test-vdl"
	env := map[string]string{}
	env["TAP"] = "true"
	env["NODE_OUTPUT"] = "vdl_test.tap"
	return runJSTest(ctx, testName, testDir, target, nil, env)
}

// VeyronJSVomTest runs the veyron javascript vom test.
func VeyronJSVomTest(ctx *util.Context, testName string) (*TestResult, error) {
	root, err := util.VeyronRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "veyron", "javascript", "vom")
	target := "test"
	return runJSTest(ctx, testName, testDir, target, nil, nil)
}
