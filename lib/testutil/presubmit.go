package testutil

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"v.io/x/devtools/lib/collect"
	"v.io/x/devtools/lib/util"
)

var (
	jenkinsHost = "http://localhost:8080/jenkins"
	netrcFile   = filepath.Join(os.Getenv("HOME"), ".netrc")
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
	root, err := util.VanadiumRoot()
	if err != nil {
		return nil, err
	}

	// Collect javascript test results.
	jsDir := filepath.Join(root, "release/javascript/core", "test_out")
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
		fileName := fileInfo.Name()
		if strings.HasPrefix(fileName, "tests_") && strings.HasSuffix(fileName, ".xml") ||
			strings.HasPrefix(fileName, "status_") && strings.HasSuffix(fileName, ".json") {
			result = append(result, filepath.Join(workspaceDir, fileName))
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

// vanadiumPresubmitPoll polls vanadium projects for new patchsets for
// which to run presubmit tests.
func vanadiumPresubmitPoll(ctx *util.Context, testName string, _ ...TestOpt) (_ *TestResult, e error) {
	root, err := util.VanadiumRoot()
	if err != nil {
		return nil, err
	}

	// Initialize the test.
	cleanup, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Use the "presubmit query" command to poll for new changes.
	logfile := filepath.Join(root, ".presubmit_log")
	args := []string{}
	if ctx.Verbose() {
		args = append(args, "-v")
	}
	args = append(args,
		"-host", jenkinsHost,
		"-netrc", netrcFile,
		"query",
		"-manifest", "public",
		"-log_file", logfile)
	if err := ctx.Run().Command("presubmit", args...); err != nil {
		return nil, err
	}

	return &TestResult{Status: TestPassed}, nil
}

// vanadiumPresubmitTest runs presubmit tests for a given project specified
// in TEST environment variable.
func vanadiumPresubmitTest(ctx *util.Context, testName string, _ ...TestOpt) (_ *TestResult, e error) {
	if err := requireEnv([]string{"BUILD_NUMBER", "REFS", "PROJECTS", "TEST", "WORKSPACE"}); err != nil {
		return nil, err
	}

	// Initialize the test.
	cleanup, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, internalTestError{err, "Init"}
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
	args := []string{}
	if ctx.Verbose() {
		args = append(args, "-v")
	}
	args = append(args,
		"-host", jenkinsHost,
		"-netrc", netrcFile,
		"test",
		"-build_number", os.Getenv("BUILD_NUMBER"),
		"-manifest", "public",
		"-projects", os.Getenv("PROJECTS"),
		"-refs", os.Getenv("REFS"),
		"-test", os.Getenv("TEST"),
	)
	if err := ctx.Run().Command("presubmit", args...); err != nil {
		return nil, err
	}

	// Remove any test result files that are empty.
	testResultFiles, err = findTestResultFiles(ctx)
	if err != nil {
		return nil, err
	}
	for _, file := range testResultFiles {
		fileInfo, err := os.Stat(file)
		if err != nil {
			return nil, err
		}
		if fileInfo.Size() == 0 {
			if err := ctx.Run().RemoveAll(file); err != nil {
				return nil, err
			}
		}
	}

	if testResult, err := generateDummyTestReportFile(ctx, testName); err != nil {
		return nil, err
	} else {
		return testResult, nil
	}
}

// generateDummyTestReportFile generate a dummy test report if
// - the tests we run didn't produce any non-empty files, or
// - the existing test report is invalid, or
// - the existing test report has no test cases.
func generateDummyTestReportFile(ctx *util.Context, testName string) (*TestResult, error) {
	testResultFiles, err := findTestResultFiles(ctx)
	if err != nil {
		return nil, err
	}
	xUnitReportFile := ""
	for _, file := range testResultFiles {
		if strings.HasSuffix(file, ".xml") {
			xUnitReportFile = file
			break
		}
	}
	// No test report.
	if xUnitReportFile == "" {
		workspaceDir := os.Getenv("WORKSPACE")
		dummyFile, perm := filepath.Join(workspaceDir, "tests_dummy.xml"), os.FileMode(0644)
		if err := ctx.Run().WriteFile(dummyFile, []byte(dummyTestResult), perm); err != nil {
			return nil, fmt.Errorf("WriteFile(%v) failed: %v", dummyFile, err)
		}
		return &TestResult{Status: TestPassed}, nil
	}

	// Invalid xUnit file.
	bytes, err := ioutil.ReadFile(xUnitReportFile)
	if err != nil {
		return nil, fmt.Errorf("ReadFile(%s) failed: %v", xUnitReportFile, err)
	}
	var suites testSuites
	if err := xml.Unmarshal(bytes, &suites); err != nil {
		ctx.Run().RemoveAll(xUnitReportFile)
		s := createTestSuiteWithFailure(testName, "Invalid xUnit Report", "Invalid xUnit Report", err.Error(), 0)
		suites := []testSuite{*s}
		if err := createXUnitReport(ctx, testName, suites); err != nil {
			return nil, err
		}
		return &TestResult{Status: TestFailed}, nil
	}

	// No test cases.
	numTestCases := 0
	for _, suite := range suites.Suites {
		numTestCases += len(suite.Cases)
	}
	if numTestCases == 0 {
		ctx.Run().RemoveAll(xUnitReportFile)
		s := createTestSuiteWithFailure(testName, "No Test Cases", "No Test Cases", "", 0)
		suites := []testSuite{*s}
		if err := createXUnitReport(ctx, testName, suites); err != nil {
			return nil, err
		}
		return &TestResult{Status: TestFailed}, nil
	}

	return &TestResult{Status: TestPassed}, nil
}

// vanadiumPresubmitResult runs "presubmit result" command to process and post test resutls.
func vanadiumPresubmitResult(ctx *util.Context, testName string, _ ...TestOpt) (_ *TestResult, e error) {
	if err := requireEnv([]string{"BUILD_NUMBER", "REFS", "PROJECTS", "WORKSPACE"}); err != nil {
		return nil, err
	}

	// Initialize the test.
	cleanup, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Run "presubmit result".
	args := []string{}
	if ctx.Verbose() {
		args = append(args, "-v")
	}
	args = append(args,
		"-host", jenkinsHost,
		"-netrc", netrcFile,
		"result",
		"-build_number", os.Getenv("BUILD_NUMBER"),
		"-refs", os.Getenv("REFS"),
		"-projects", os.Getenv("PROJECTS"),
	)
	if err := ctx.Run().Command("presubmit", args...); err != nil {
		return nil, err
	}

	return &TestResult{Status: TestPassed}, nil
}
