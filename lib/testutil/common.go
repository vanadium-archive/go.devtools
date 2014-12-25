package testutil

import (
	"fmt"
	"os"
	"path/filepath"

	"veyron.io/tools/lib/util"
)

var (
	// cleanGo is used to control whether the initTest function removes
	// all stale Go object files and binaries. It is use to prevent the
	// test of this package from interfering with other concurrently
	// running tests that might be sharing the same object files.
	cleanGo = true
)

// binDirPath returns the path to the directory for storing temporary
// binaries.
func binDirPath() string {
	return filepath.Join(os.Getenv("TMPDIR"), "bin")
}

// initTest carries out the initial actions for the given test.
func initTest(ctx *util.Context, testName string, profiles []string) (func() error, error) {
	// Output the hostname.
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("Hostname() failed: %v", err)
	}
	fmt.Fprintf(ctx.Stdout(), "hostname = %q\n", hostname)

	// Create a working test directory under $HOME/tmp and set the
	// TMPDIR environment variable to it.
	rootDir := filepath.Join(os.Getenv("HOME"), "tmp", testName)
	if err := ctx.Run().MkdirAll(rootDir, os.FileMode(0755)); err != nil {
		return nil, err
	}
	workDir, err := ctx.Run().TempDir(rootDir, "")
	if err != nil {
		return nil, fmt.Errorf("TempDir() failed: %v", err)
	}
	if err := os.Setenv("TMPDIR", workDir); err != nil {
		return nil, err
	}
	fmt.Fprintf(ctx.Stdout(), "workdir = %q\n", workDir)

	// Create a temporary directory for storing binaries.
	if err := ctx.Run().MkdirAll(binDirPath(), os.FileMode(0755)); err != nil {
		return nil, err
	}

	// Setup profiles.
	for _, profile := range profiles {
		if err := ctx.Run().Command("v23", "profile", "setup", profile); err != nil {
			return nil, err
		}
	}

	// Descend into the working directory (unless doing a "dry
	// run" in which case the working directory does not exist).
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if !ctx.DryRun() {
		if err := ctx.Run().Chdir(workDir); err != nil {
			return nil, err
		}
	}

	// Remove all stale Go object files and binaries.
	if cleanGo {
		if err := ctx.Run().Command("v23", "goext", "distclean"); err != nil {
			return nil, err
		}
	}

	// Remove xunit test report file.
	if err := ctx.Run().RemoveAll(XUnitReportPath(testName)); err != nil {
		return nil, err
	}

	return func() error {
		return ctx.Run().Chdir(cwd)
	}, nil
}

func Pass(ctx *util.Context, format string, a ...interface{}) {
	strOK := "ok"
	if ctx.Color() {
		strOK = util.ColorString("ok", util.Green)
	}
	fmt.Fprintf(ctx.Stdout(), "%s   ", strOK)
	fmt.Fprintf(ctx.Stdout(), format, a...)
}

func Fail(ctx *util.Context, format string, a ...interface{}) {
	strFail := "fail"
	if ctx.Color() {
		strFail = util.ColorString("fail", util.Red)
	}
	fmt.Fprintf(ctx.Stderr(), "%s ", strFail)
	fmt.Fprintf(ctx.Stderr(), format, a...)
}
