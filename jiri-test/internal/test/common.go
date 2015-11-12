// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"v.io/jiri/profiles"
	"v.io/jiri/project"
	"v.io/jiri/tool"
	"v.io/x/devtools/jiri-v23-profile/v23_profile"
)

var (
	// The name of the v23-profile manifest file to use. It is intended
	// to be set via a command line flag.
	ManifestFilename = v23_profile.DefaultManifestFilename

	// cleanGo is used to control whether the initTest function removes
	// all stale Go object files and binaries. It is used to prevent the
	// test of this package from interfering with other concurrently
	// running tests that might be sharing the same object files.
	cleanGo = true

	// Regexp to match javascript result files.
	reJSResult = regexp.MustCompile(`.*_(integration|spec)\.out$`)

	// Regexp to match common test result files.
	reTestResult = regexp.MustCompile(`^((tests_.*\.xml)|(status_.*\.json))$`)
)

// internalTestError represents an internal test error.
type internalTestError struct {
	err  error
	name string
}

func (e internalTestError) Error() string {
	return fmt.Sprintf("%s:\n%s\n", e.name, e.err.Error())
}

var testTmpDir = ""

type initTestOpt interface {
	initTestOpt()
}

type rootDirOpt string

func (rootDirOpt) initTestOpt() {}

// binDirPath returns the path to the directory for storing temporary
// binaries.
func binDirPath() string {
	if len(testTmpDir) == 0 {
		panic("binDirPath() shouldn't be called before initTest()")
	}
	return filepath.Join(testTmpDir, "bin")
}

// regTestBinDirPath returns the path to the directory for storing
// regression test binaries.
func regTestBinDirPath() string {
	if len(testTmpDir) == 0 {
		panic("regTestBinDirPath() shouldn't be called before initTest()")
	}
	return filepath.Join(testTmpDir, "regtest")
}

// initTest carries out the initial actions for the given test.
func initTest(ctx *tool.Context, testName string, profileNames []string, opts ...initTestOpt) (func() error, error) {
	return initTestImpl(ctx, testName, "v23-profile", profileNames, "", opts...)
}

// initTestForTarget carries out the initial actions for the given test using
// a specific profile Target..
func initTestForTarget(ctx *tool.Context, testName string, profileNames []string, target string, opts ...initTestOpt) (func() error, error) {
	return initTestImpl(ctx, testName, "v23-profile", profileNames, target, opts...)
}

func initTestImpl(ctx *tool.Context, testName string, profileCommand string, profileNames []string, target string, opts ...initTestOpt) (func() error, error) {
	// Output the hostname.
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("Hostname() failed: %v", err)
	}
	fmt.Fprintf(ctx.Stdout(), "hostname = %q\n", hostname)

	// Create a working test directory under $HOME/tmp and set the
	// TMPDIR environment variable to it.
	rootDir := filepath.Join(os.Getenv("HOME"), "tmp", testName)
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case rootDirOpt:
			rootDir = string(typedOpt)
		}
	}
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
	testTmpDir = workDir
	fmt.Fprintf(ctx.Stdout(), "workdir = %q\n", workDir)
	fmt.Fprintf(ctx.Stdout(), "bin dir = %q\n", binDirPath())

	// Create a directory for storing built binaries.
	if err := ctx.Run().MkdirAll(binDirPath(), os.FileMode(0755)); err != nil {
		return nil, fmt.Errorf("MkdirAll(%s): %v", binDirPath(), err)
	}

	// Create a directory for storing regression test binaries.
	if err := ctx.Run().MkdirAll(regTestBinDirPath(), os.FileMode(0755)); err != nil {
		return nil, err
	}

	if err := cleanupProfiles(ctx); err != nil {
		return nil, internalTestError{err, "Init"}
	}

	insertTarget := func(profile string) []string {
		if len(target) > 0 {
			return []string{"--target=" + target, profile}
		}
		return []string{profile}
	}

	// Install profiles.
	args := []string{"-v", profileCommand, "install"}
	for _, profile := range profileNames {
		if profileCommand == "v23-profile" {
			t := profiles.NativeTarget()
			if len(target) > 0 {
				var err error
				t, err = profiles.NewTarget(target)
				if err != nil {
					return nil, fmt.Errorf("NewTarget(%v): %v", target, err)
				}
			}
			if profiles.LookupProfileTarget(profile, t) != nil {
				continue
			}
		}
		clargs := append(args, insertTarget(profile)...)
		fmt.Fprintf(ctx.Stdout(), "Running: jiri %s\n", strings.Join(clargs, " "))
		if err := ctx.Run().Command("jiri", clargs...); err != nil {
			return nil, fmt.Errorf("jiri %v: %v", strings.Join(clargs, " "), err)
		}
		fmt.Fprintf(ctx.Stdout(), "jiri %v: success\n", strings.Join(clargs, " "))
	}

	// Update profiles.
	args = []string{profileCommand, "update"}

	if err := ctx.Run().Command("jiri", args...); err != nil {
		return nil, fmt.Errorf("jiri %v: %v", strings.Join(args, " "), err)
	}
	fmt.Fprintf(ctx.Stdout(), "jiri %v: success\n", strings.Join(args, " "))
	displayProfiles(ctx, "initTest:")

	// Descend into the working directory (unless doing a "dry
	// run" in which case the working directory does not exist).
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if !ctx.DryRun() {
		if err := ctx.Run().Chdir(workDir); err != nil {
			return nil, fmt.Errorf("Chdir(%s): %v", workDir, err)
		}
	}

	// Remove all stale Go object files and binaries.
	if cleanGo {
		// TODO(nlacasse, cnicolaou): Remove this once goext distclean is fixed
		// on jenkins.
		root, err := project.JiriRoot()
		if err != nil {
			return nil, err
		}
		if err := ctx.Run().RemoveAll(filepath.Join(root, "release", "go", "pkg")); err != nil {
			return nil, err
		}

		if err := ctx.Run().Command("jiri", "goext", "distclean"); err != nil {
			return nil, fmt.Errorf("jiri goext distclean: %v", err)
		}
	}

	// Cleanup the test results possibly left behind by the
	// previous test.
	testResultFiles, err := findTestResultFiles(ctx, testName)
	if err != nil {
		return nil, err
	}
	for _, file := range testResultFiles {
		if err := ctx.Run().RemoveAll(file); err != nil {
			return nil, fmt.Errorf("RemoveAll(%s): %v", file, err)
		}
	}

	return func() error {
		if err := ctx.Run().Chdir(cwd); err != nil {
			return fmt.Errorf("Chdir(%s): %v", cwd, err)
		}
		return nil
	}, nil
}

// findTestResultFiles returns a slice of paths to test result related files.
func findTestResultFiles(ctx *tool.Context, testName string) ([]string, error) {
	result := []string{}
	root, err := project.JiriRoot()
	if err != nil {
		return nil, err
	}

	// Collect javascript test results.
	jsDir := filepath.Join(root, "release", "javascript", "core", "test_out")
	if _, err := ctx.Run().Stat(jsDir); err == nil {
		fileInfoList, err := ctx.Run().ReadDir(jsDir)
		if err != nil {
			return nil, err
		}
		for _, fileInfo := range fileInfoList {
			name := fileInfo.Name()
			if reJSResult.MatchString(name) {
				result = append(result, filepath.Join(jsDir, name))
			}
		}
	} else {
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	// Collect xUnit xml files and test status json files.
	workspaceDir := os.Getenv("WORKSPACE")
	if workspaceDir == "" {
		workspaceDir = filepath.Join(os.Getenv("HOME"), "tmp", testName)
	}
	fileInfoList, err := ctx.Run().ReadDir(workspaceDir)
	if err != nil {
		return nil, err
	}
	for _, fileInfo := range fileInfoList {
		fileName := fileInfo.Name()
		if reTestResult.MatchString(fileName) {
			result = append(result, filepath.Join(workspaceDir, fileName))
		}
	}
	return result, nil
}
