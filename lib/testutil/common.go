package testutil

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"veyron.io/tools/lib/runutil"
	"veyron.io/tools/lib/util"
)

// cleanGo is used to control whether the initTest function removes
// all stale Go object files and binaries. It is use to prevent the
// test of this package from interfering with other concurrently
// running tests that might be sharing the same object files.
var cleanGo = true

// binDirPath returns the path to the directory for storing temporary
// binaries.
func binDirPath() string {
	return filepath.Join(os.Getenv("TMPDIR"), "bin")
}

// initTest carries out the initial actions for the given test.
func initTest(ctx *util.Context, testName string, profiles []string) (func(), error) {
	// Output the hostname.
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("Hostname() failed: %v", err)
	}
	fmt.Fprintf(ctx.Stdout(), "hostname = %s\n", hostname)

	// Create a working test directory under $HOME/tmp and set the
	// TMPDIR environment variable to it.
	rootDir := filepath.Join(os.Getenv("HOME"), "tmp", testName)
	if err := ctx.Run().Function(runutil.MkdirAll(rootDir, os.FileMode(0755))); err != nil {
		return nil, err
	}
	workDir, err := ioutil.TempDir(rootDir, "")
	if err != nil {
		return nil, fmt.Errorf("TempDir() failed: %v", err)
	}
	if err := os.Setenv("TMPDIR", workDir); err != nil {
		return nil, err
	}
	fmt.Fprintf(ctx.Stdout(), "workdir = %s\n", workDir)

	// Create a temporary directory for storing binaries.
	if err := ctx.Run().Function(runutil.MkdirAll(binDirPath(), os.FileMode(0755))); err != nil {
		return nil, err
	}

	// Setup profiles.
	for _, profile := range profiles {
		if err := ctx.Run().Command("veyron", "profile", "setup", profile); err != nil {
			return nil, err
		}
	}

	// Descend to the working directory.
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if err := ctx.Run().Function(runutil.Chdir(workDir)); err != nil {
		return nil, err
	}

	// Remove all stale Go object files and binaries.
	if cleanGo {
		if err := ctx.Run().Command("veyron", "goext", "distclean"); err != nil {
			return nil, err
		}
	}

	return func() {
		os.Chdir(cwd)
	}, nil
}
