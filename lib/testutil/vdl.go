package testutil

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"v.io/tools/lib/util"
)

// veyronVDL checks that all VDL-based Go source files are up-to-date.
func veyronVDL(ctx *util.Context, testName string) (*TestResult, error) {
	fmt.Fprintf(ctx.Stdout(), "NOTE: This test checks that all VDL-based Go source files are up-to-date.\nIf it fails, you probably just need to run 'v23 run vdl generate all'.\n")

	root, err := util.VanadiumRoot()
	if err != nil {
		return nil, err
	}

	// Install the vdl tool.
	opts := ctx.Run().Opts()
	opts.Env["GOPATH"] = filepath.Join(root, "release", "go")
	if err := ctx.Run().CommandWithOpts(opts, "go", "install", "v.io/core/veyron2/vdl/vdl"); err != nil {
		return nil, err
	}

	// Check that "vdl audit --lang=go all" produces no output.
	var out bytes.Buffer
	opts = ctx.Run().Opts()
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
		return &TestResult{Status: TestFailed}, nil
	}
	return &TestResult{Status: TestPassed}, nil
}
