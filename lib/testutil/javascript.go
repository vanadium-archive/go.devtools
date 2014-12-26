package testutil

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"v.io/tools/lib/collect"
	"v.io/tools/lib/runutil"
	"v.io/tools/lib/util"
)

const (
	defaultJSTestTimeout = 10 * time.Minute
)

// runJSTest is a harness for executing javascript tests.
func runJSTest(ctx *util.Context, testName, testDir, target string, cleanFn func() error, env map[string]string) (_ *TestResult, e error) {
	// Initialize the test.
	cleanup, err := initTest(ctx, testName, []string{"web"})
	if err != nil {
		return nil, err
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Navigate to the target directory.
	if err := ctx.Run().Chdir(testDir); err != nil {
		return nil, err
	}

	// Clean up after previous instances of the test.
	opts := ctx.Run().Opts()
	for key, value := range env {
		opts.Env[key] = value
	}
	if err := ctx.Run().CommandWithOpts(opts, "make", "clean"); err != nil {
		return nil, err
	}
	if cleanFn != nil {
		if err := cleanFn(); err != nil {
			return nil, err
		}
	}

	// Run the test target.
	var stderr bytes.Buffer
	opts = ctx.Run().Opts()
	opts.Stderr = &stderr
	if err := ctx.Run().TimedCommandWithOpts(defaultJSTestTimeout, opts, "make", target); err != nil {
		fmt.Fprintf(ctx.Stderr(), "Stderr:\n%s\n", stderr.String())
		if err == runutil.CommandTimedOutErr {
			return &TestResult{
				Status:       TestTimedOut,
				TimeoutValue: defaultJSTestTimeout,
			}, nil
		}
		// Generate an xunit report if none exists.
		// This can happen when errors are not in javascript tests themselves.
		xunitFilePath := XUnitReportPath(testName)
		if _, err := os.Stat(xunitFilePath); err != nil {
			if !os.IsNotExist(err) {
				return nil, err
			}
			s := createTestSuiteWithFailure(testName, "Test", "failure", stderr.String(), 0)
			if err := createXUnitReport(ctx, testName, []testSuite{*s}); err != nil {
				return nil, err
			}
		}
		return &TestResult{Status: TestFailed}, nil
	}

	return &TestResult{Status: TestPassed}, nil
}

// veyronJSBuildExtension tests the veyron javascript build extension.
func veyronJSBuildExtension(ctx *util.Context, testName string) (*TestResult, error) {
	root, err := util.VanadiumRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "core")
	target := "extension/veyron.crx"
	return runJSTest(ctx, testName, testDir, target, nil, nil)
}

// veyronJSDoc (re)generates the content of the veyron javascript
// documentation server.
func veyronJSDoc(ctx *util.Context, testName string) (*TestResult, error) {
	root, err := util.VanadiumRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "core")
	target := "docs"
	webDir, jsDocDir := "/usr/share/nginx/www/jsdoc", filepath.Join(testDir, "docs")
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
	// Move generated js documentation to the web server directory.
	if err := ctx.Run().Rename(jsDocDir, webDir); err != nil {
		return nil, err
	}
	return result, nil
}

// veyronJSBrowserIntegration runs the veyron javascript integration test in a browser environment using nacl plugin.
func veyronJSBrowserIntegration(ctx *util.Context, testName string) (*TestResult, error) {
	root, err := util.VanadiumRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "core")
	target := "test-integration-browser"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["BROWSER_OUTPUT"] = XUnitReportPath(testName)
	return runJSTest(ctx, testName, testDir, target, nil, env)
}

// veyronJSNodeIntegration runs the veyron javascript integration test in NodeJS environment using wspr.
func veyronJSNodeIntegration(ctx *util.Context, testName string) (*TestResult, error) {
	root, err := util.VanadiumRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "core")
	target := "test-integration-node"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["NODE_OUTPUT"] = XUnitReportPath(testName)
	return runJSTest(ctx, testName, testDir, target, nil, env)
}

// veyronJSUnit runs the veyron javascript unit test.
func veyronJSUnit(ctx *util.Context, testName string) (*TestResult, error) {
	root, err := util.VanadiumRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "core")
	target := "test-unit"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["NODE_OUTPUT"] = XUnitReportPath(testName)
	return runJSTest(ctx, testName, testDir, target, nil, env)
}

// veyronJSVdl runs the veyron javascript vdl test.
func veyronJSVdl(ctx *util.Context, testName string) (*TestResult, error) {
	root, err := util.VanadiumRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "core")
	target := "test-vdl"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["NODE_OUTPUT"] = XUnitReportPath(testName)
	return runJSTest(ctx, testName, testDir, target, nil, env)
}

// veyronJSVom runs the veyron javascript vom test.
func veyronJSVom(ctx *util.Context, testName string) (*TestResult, error) {
	root, err := util.VanadiumRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "vom")
	target := "test"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["NODE_OUTPUT"] = XUnitReportPath(testName)
	return runJSTest(ctx, testName, testDir, target, nil, env)
}

func setCommonJSEnv(env map[string]string) {
	env["XUNIT"] = "true"
	env["NOVDLGEN"] = "true"
}
