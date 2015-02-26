package testutil

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"v.io/tools/lib/collect"
	"v.io/tools/lib/util"
)

// vanadiumGoVDL checks that all VDL-based Go source files are
// up-to-date.
func vanadiumGoVDL(ctx *util.Context, testName string, _ ...TestOpt) (_ *TestResult, e error) {
	fmt.Fprintf(ctx.Stdout(), "NOTE: This test checks that all VDL-based Go source files are up-to-date.\nIf it fails, you probably just need to run 'v23 run vdl generate --lang=go all'.\n")

	root, err := util.VanadiumRoot()
	if err != nil {
		return nil, err
	}

	cleanup, err := initTest(ctx, testName, []string{})
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Install the vdl tool.
	if err := ctx.Run().Command("v23", "go", "install", "v.io/core/veyron/tools/vdl"); err != nil {
		return nil, internalTestError{err, "Install VDL"}
	}

	// Check that "vdl audit --lang=go all" produces no output.
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	venv, err := util.VanadiumEnvironment(util.HostPlatform())
	if err != nil {
		return nil, err
	}
	opts.Env["VDLPATH"] = venv.Get("VDLPATH")
	vdl := filepath.Join(root, "release", "go", "bin", "vdl")
	err = ctx.Run().CommandWithOpts(opts, vdl, "audit", "--lang=go", "all")
	output := strings.TrimSpace(out.String())
	if err != nil || len(output) != 0 {
		fmt.Fprintf(ctx.Stdout(), "%v\n", output)
		// Create xUnit report.
		files := strings.Split(output, "\n")
		suites := []testSuite{}
		for _, file := range files {
			s := createTestSuiteWithFailure("VDLAudit", file, "VDL audit failure", "Outdated file:\n"+file, 0)
			suites = append(suites, *s)
		}
		if err := createXUnitReport(ctx, testName, suites); err != nil {
			return nil, err
		}
		return &TestResult{Status: TestFailed}, nil
	}
	return &TestResult{Status: TestPassed}, nil
}
