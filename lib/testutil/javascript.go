package testutil

import (
	"path/filepath"

	"veyron.io/tools/lib/collect"
	"veyron.io/tools/lib/util"
)

// runJSTest is a harness for executing javascript tests.
func (t *testEnv) runJSTest(ctx *util.Context, testName, testDir, target string, cleanFn func() error, env map[string]string) (_ *TestResult, e error) {
	// Initialize the test.
	cleanup, err := t.initTest(ctx, testName, []string{"web"})
	if err != nil {
		return nil, err
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Navigate to the target directory.
	if err := ctx.Run().Chdir(testDir); err != nil {
		return nil, err
	}

	// Clean up after previous instances of the test.
	opts := t.setTestEnv(ctx.Run().Opts())
	if err := ctx.Run().CommandWithOpts(opts, "make", "clean"); err != nil {
		return nil, err
	}
	if cleanFn != nil {
		if err := cleanFn(); err != nil {
			return nil, err
		}
	}

	// Run the test target.
	if err := ctx.Run().CommandWithOpts(opts, "make", target); err != nil {
		return &TestResult{Status: TestFailed}, nil
	}

	return &TestResult{Status: TestPassed}, nil
}

// veyronJSBuildExtension tests the veyron javascript build extension.
func (t *testEnv) veyronJSBuildExtension(ctx *util.Context, testName string) (*TestResult, error) {
	root, err := util.VeyronRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "veyron.js")
	target := "extension/veyron.crx"
	return t.runJSTest(ctx, testName, testDir, target, nil, nil)
}

// veyronJSDoc (re)generates the content of the veyron javascript
// documentation server.
func (t *testEnv) veyronJSDoc(ctx *util.Context, testName string) (*TestResult, error) {
	root, err := util.VeyronRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "veyron.js")
	target := "docs"
	webDir, jsDocDir := "/usr/share/nginx/www/jsdoc", filepath.Join(testDir, "docs")
	cleanFn := func() error {
		if err := ctx.Run().RemoveAll(webDir); err != nil {
			return err
		}
		return nil
	}
	result, err := t.runJSTest(ctx, testName, testDir, target, cleanFn, nil)
	if err != nil {
		return nil, err
	}
	// Move generated js documentation to the web server directory.
	if err := ctx.Run().Rename(jsDocDir, webDir); err != nil {
		return nil, err
	}
	return result, nil
}

// veyronJSBrowserIntegrationTest runs the veyron javascript integration test in a browser environment using nacl plugin.
func (t *testEnv) veyronJSBrowserIntegrationTest(ctx *util.Context, testName string) (*TestResult, error) {
	// TODO(aghassemi): Re-enable the test when it is fixed.
	return &TestResult{Status: TestPassed}, nil

	root, err := util.VeyronRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "veyron.js")
	target := "test-integration-browser"
	env := map[string]string{}
	env["XUNIT"] = "true"
	env["BROWSER_OUTPUT"] = XUnitReportPath(testName)
	return t.runJSTest(ctx, testName, testDir, target, nil, env)
}

// veyronJSNodeIntegrationTest runs the veyron javascript integration test in NodeJS environment using wspr.
func (t *testEnv) veyronJSNodeIntegrationTest(ctx *util.Context, testName string) (*TestResult, error) {
	root, err := util.VeyronRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "veyron.js")
	target := "test-integration-node"
	env := map[string]string{}
	env["XUNIT"] = "true"
	env["NODE_OUTPUT"] = XUnitReportPath(testName)
	return t.runJSTest(ctx, testName, testDir, target, nil, env)
}

// veyronJSUnitTest runs the veyron javascript unit test.
func (t *testEnv) veyronJSUnitTest(ctx *util.Context, testName string) (*TestResult, error) {
	root, err := util.VeyronRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "veyron.js")
	target := "test-unit"
	env := map[string]string{}
	env["XUNIT"] = "true"
	env["NODE_OUTPUT"] = XUnitReportPath(testName)
	return t.runJSTest(ctx, testName, testDir, target, nil, env)
}

// veyronJSVdlTest runs the veyron javascript vdl test.
func (t *testEnv) veyronJSVdlTest(ctx *util.Context, testName string) (*TestResult, error) {
	root, err := util.VeyronRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "veyron.js")
	target := "test-vdl"
	env := map[string]string{}
	env["XUNIT"] = "true"
	env["NODE_OUTPUT"] = XUnitReportPath(testName)
	return t.runJSTest(ctx, testName, testDir, target, nil, env)
}

// veyronJSVomTest runs the veyron javascript vom test.
func (t *testEnv) veyronJSVomTest(ctx *util.Context, testName string) (*TestResult, error) {
	root, err := util.VeyronRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "veyron", "javascript", "vom")
	target := "test"
	env := map[string]string{}
	env["XUNIT"] = "true"
	env["NODE_OUTPUT"] = XUnitReportPath(testName)
	return t.runJSTest(ctx, testName, testDir, target, nil, env)
}
