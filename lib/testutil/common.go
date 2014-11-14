package testutil

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"tools/lib/runutil"
	"tools/lib/util"
)

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

	return func() {
		os.Chdir(cwd)
	}, nil
}
