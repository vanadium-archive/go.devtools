package testutil

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"veyron.io/tools/lib/envutil"
	"veyron.io/tools/lib/util"
)

// VeyronVDL checks that all VDL-based Go source files are up-to-date.
func VeyronVDL(ctx *util.Context, testName string) (*TestResult, error) {
	root, err := util.VeyronRoot()
	if err != nil {
		return nil, err
	}

	// Install the vdl tool.
	opts := ctx.Run().Opts()
	env := envutil.NewSnapshotFromOS()
	env.Set("GOPATH", filepath.Join(root, "veyron", "go"))
	opts.Env = env.Map()
	if err := ctx.Run().CommandWithOpts(opts, "go", "install", "veyron.io/veyron/veyron2/vdl/vdl"); err != nil {
		return nil, err
	}

	// Check that "vdl audit --lang=go all" produces no output.
	var out bytes.Buffer
	opts = ctx.Run().Opts()
	env = envutil.NewSnapshotFromOS()
	opts.Stdout = &out
	opts.Stderr = &out
	venv, err := util.VeyronEnvironment(util.HostPlatform())
	if err != nil {
		return nil, err
	}
	env.Set("VDLPATH", venv.Get("VDLPATH"))
	opts.Env = env.Map()
	vdl := filepath.Join(root, "veyron", "go", "bin", "vdl")
	err = ctx.Run().CommandWithOpts(opts, vdl, "audit", "--lang=go", "all")
	output := strings.TrimSpace(out.String())
	if err != nil || len(output) != 0 {
		fmt.Fprintf(ctx.Stdout(), "%v\n", output)
		return &TestResult{Status: TestFailed}, nil
	}
	return &TestResult{Status: TestPassed}, nil
}
