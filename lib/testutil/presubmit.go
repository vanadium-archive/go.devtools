package testutil

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"veyron.io/tools/lib/collect"
	"veyron.io/tools/lib/util"
)

var (
	jenkinsHost = "http://veyron-jenkins:8001/jenkins"
	// The token below belongs to jingjin@google.com.
	jenkinsToken = "0e67bfe70302a528807d3594730c9d8b"
	netrcFile    = filepath.Join(os.Getenv("HOME"), ".netrc")
)

const (
	dummyTestResult = `<?xml version="1.0" encoding="utf-8"?>
<!--
  This file will be used to generate a dummy test results file
  in case the presubmit tests produce no test result files.
-->
<testsuites>
  <testsuite name="NO_TESTS" tests="1" errors="0" failures="0" skip="0">
    <testcase classname="NO_TESTS" name="NO_TESTS" time="0">
    </testcase>
  </testsuite>
</testsuites>
`
)

// findTestResultFiles returns a slice of paths to presubmit test
// results.
func findTestResultFiles(ctx *util.Context) ([]string, error) {
	result := []string{}
	root, err := util.VeyronRoot()
	if err != nil {
		return nil, err
	}

	// Collect javascript test results.
	jsDir := filepath.Join(root, "veyron.js", "test_out")
	if _, err := os.Stat(jsDir); err == nil {
		fileInfoList, err := ioutil.ReadDir(jsDir)
		if err != nil {
			return nil, fmt.Errorf("ReadDir(%v) failed: %v", jsDir, err)
		}
		for _, fileInfo := range fileInfoList {
			name := fileInfo.Name()
			if strings.HasSuffix(name, "_integration.out") || strings.HasSuffix(name, "_spec.out") {
				result = append(result, filepath.Join(jsDir, name))
			}
		}
	} else {
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	// Collect non-javascript test results.
	workspaceDir := os.Getenv("WORKSPACE")
	fileInfoList, err := ioutil.ReadDir(workspaceDir)
	if err != nil {
		return nil, fmt.Errorf("ReadDir(%v) failed: %v", workspaceDir, err)
	}
	for _, fileInfo := range fileInfoList {
		if strings.HasPrefix(fileInfo.Name(), "tests_") && strings.HasSuffix(fileInfo.Name(), ".xml") {
			result = append(result, filepath.Join(workspaceDir, fileInfo.Name()))
		}
	}
	return result, nil
}

// requireEnv makes sure that the given environment variables are set.
func requireEnv(names []string) error {
	for _, name := range names {
		if os.Getenv(name) == "" {
			return fmt.Errorf("environment variable %q is not set", name)
		}
	}
	return nil
}

// veyronPresubmitPoll polls veyron projects for new patchsets for
// which to run presubmit tests.
func veyronPresubmitPoll(ctx *util.Context, testName string) (_ *TestResult, e error) {
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

	// Use the "presubmit query" command to poll for new changes.
	logfile := filepath.Join(root, ".presubmit_log")
	args := []string{"-host", jenkinsHost, "-token", jenkinsToken, "-netrc", netrcFile, "query", "-log_file", logfile}
	if ctx.Verbose() {
		args = append(args, "-v")
	}
	if err := ctx.Run().Command("presubmit", args...); err != nil {
		return nil, err
	}

	return &TestResult{Status: TestPassed}, nil
}

// veyronPresubmitTest runs presubmit tests for veyron projects.
func veyronPresubmitTest(ctx *util.Context, testName string) (_ *TestResult, e error) {
	if err := requireEnv([]string{"BUILD_NUMBER", "REFS", "REPOS", "WORKSPACE"}); err != nil {
		return nil, err
	}

	// Initialize the test.
	cleanup, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, err
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Cleanup the test results possibly left behind by the
	// previous presubmit test.
	testResultFiles, err := findTestResultFiles(ctx)
	if err != nil {
		return nil, err
	}
	for _, file := range testResultFiles {
		if err := ctx.Run().RemoveAll(file); err != nil {
			return nil, err
		}
	}

	// Use the "presubmit test" command to run the presubmit test.
	args := []string{
		"-host", jenkinsHost, "-token", jenkinsToken, "-netrc", netrcFile, "test",
		"-build_number", os.Getenv("BUILD_NUMBER"),
		"-repos", os.Getenv("REPOS"),
		"-refs", os.Getenv("REFS"),
	}
	if ctx.Verbose() {
		args = append(args, "-v")
	}
	if err := ctx.Run().Command("presubmit", args...); err != nil {
		return nil, err
	}

	// Remove any test result files that are empty.
	testResultFiles, err = findTestResultFiles(ctx)
	if err != nil {
		return nil, err
	}
	for _, file := range testResultFiles {
		if fileInfo, err := os.Stat(file); err != nil {
			return nil, err
		} else {
			if fileInfo.Size() == 0 {
				if err := ctx.Run().RemoveAll(file); err != nil {
					return nil, err
				}
			}
		}
	}

	// Generate a dummy test results file if the tests we run
	// didn't produce any non-empty files.
	testResultFiles, err = findTestResultFiles(ctx)
	if err != nil {
		return nil, err
	}
	if len(testResultFiles) == 0 {
		workspaceDir := os.Getenv("WORKSPACE")
		dummyFile, perm := filepath.Join(workspaceDir, "tests_dummy.xml"), os.FileMode(0644)
		if err := ctx.Run().WriteFile(dummyFile, []byte(dummyTestResult), perm); err != nil {
			return nil, fmt.Errorf("WriteFile(%v) failed: %v", dummyFile, err)
		}
	}

	return &TestResult{Status: TestPassed}, nil
}
