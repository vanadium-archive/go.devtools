package testutil

import (
	"bytes"
	"io"
	"os"
	"path/filepath"

	"v.io/x/devtools/lib/collect"
	"v.io/x/devtools/lib/util"
)

// vanadiumBootstrap runs a test of Vanadium bootstrapping.
func vanadiumBootstrap(ctx *util.Context, testName string, _ ...TestOpt) (_ *TestResult, e error) {
	// Initialize the test.
	cleanup, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Create a new temporary VANADIUM_ROOT.
	oldRoot := os.Getenv("VANADIUM_ROOT")
	defer collect.Error(func() error { return os.Setenv("VANADIUM_ROOT", oldRoot) }, &e)
	tmpDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		return nil, internalTestError{err, "TempDir"}
	}
	defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpDir) }, &e)

	if err := os.Setenv("VANADIUM_ROOT", tmpDir); err != nil {
		return nil, internalTestError{err, "Setenv"}
	}

	// Run the setup script.
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = io.MultiWriter(opts.Stdout, &out)
	opts.Stderr = io.MultiWriter(opts.Stderr, &out)
	if err := ctx.Run().CommandWithOpts(opts, filepath.Join(oldRoot, "scripts", "setup", "vanadium")); err != nil {
		// Create xUnit report.
		suites := []testSuite{}
		s := createTestSuiteWithFailure("VanadiumGo", "bootstrap", "Vanadium bootstrapping failed", out.String(), 0)
		suites = append(suites, *s)
		if err := createXUnitReport(ctx, testName, suites); err != nil {
			return nil, err
		}
		return &TestResult{Status: TestFailed}, nil
	}
	return &TestResult{Status: TestPassed}, nil
}