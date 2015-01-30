package testutil

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"v.io/tools/lib/runutil"
	"v.io/tools/lib/util"
)

// vanadiumVDL checks that all VDL-based Go source files are up-to-date.
func vanadiumVDL(ctx *util.Context, testName string) (*TestResult, error) {
	fmt.Fprintf(ctx.Stdout(), "NOTE: This test checks that all VDL-based Go source files are up-to-date.\nIf it fails, you probably just need to run 'v23 run vdl generate --lang=go all'.\n")

	root, err := util.VanadiumRoot()
	if err != nil {
		return nil, err
	}

	// Install the vdl tool.
	if testResult, err := genXUnitReportOnCmdError(ctx, testName, "VDLInstall", "failure",
		func(opts runutil.Opts) error {
			opts.Env["GOPATH"] = filepath.Join(root, "release", "go")
			return ctx.Run().CommandWithOpts(opts, "go", "install", "v.io/core/veyron2/vdl/vdl")
		}); err != nil {
		return nil, err
	} else if testResult != nil {
		return testResult, nil
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
		s := createTestSuiteWithFailure(testName, "VDLAudit", "failure", output, 0)
		suites := []testSuite{*s}
		if err := createXUnitReport(ctx, testName, suites); err != nil {
			return nil, err
		}
		return &TestResult{Status: TestFailed}, nil
	}
	return &TestResult{Status: TestPassed}, nil
}
