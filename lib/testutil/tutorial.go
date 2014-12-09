package testutil

import (
	"path/filepath"

	"veyron.io/tools/lib/collect"
	"veyron.io/tools/lib/envutil"
	"veyron.io/tools/lib/runutil"
	"veyron.io/tools/lib/util"
)

// VeyronTutorial runs the veyron tutorial examples.
//
// TODO(jregan): Merge the mdrip logic into this package.
func VeyronTutorial(ctx *util.Context, testName string) (_ *TestResult, e error) {
	root, err := util.VeyronRoot()
	if err != nil {
		return nil, err
	}

	// Initialize the test.
	cleanup, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, err
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Install the mdrip tool.
	opts := ctx.Run().Opts()
	env := envutil.NewSnapshotFromOS()
	env.Set("GOPATH", filepath.Join(root, "tutorial", "testing"))
	opts.Env = env.Map()
	if err := ctx.Run().CommandWithOpts(opts, "go", "install", "mdrip"); err != nil {
		return nil, err
	}

	// Run the tutorials.
	content := filepath.Join(root, "tutorial", "www", "content")
	mdrip := filepath.Join(root, "tutorial", "testing", "bin", "mdrip")
	args := []string{"--subshell", "1",
		filepath.Join(content, "docs", "installation", "index.md"),
		filepath.Join(content, "tutorials", "basics.md"),
		filepath.Join(content, "tutorials", "security.md"),
	}
	if err := ctx.Run().TimedCommand(DefaultTestTimeout, mdrip, args...); err != nil {
		if err == runutil.CommandTimedOutErr {
			return &TestResult{Status: TestTimedOut}, nil
		}
		return &TestResult{Status: TestFailed}, nil
	}
	return &TestResult{Status: TestPassed}, nil
}
