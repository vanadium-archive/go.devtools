// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"fmt"
	"os"
	"path/filepath"

	"v.io/jiri"
	"v.io/jiri/collect"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/xunit"
)

var (
	jenkinsHost = "http://localhost:8001/jenkins"
)

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
func vanadiumPresubmitPoll(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTestImpl(jirix, false, false, false, testName, nil, "")
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Use the "presubmit query" command to poll for new changes.
	logfile := filepath.Join(jirix.Root, ".presubmit_log")
	args := []string{}
	if jirix.Verbose() {
		args = append(args, "-v")
	} else {
		// append this for testing this CL only - remove on checkin.
		args = append(args, "-v")
	}
	args = append(args,
		"-host", jenkinsHost,
		"query",
		"-log-file", logfile,
		"-manifest", "tools",
	)
	if err := jirix.NewSeq().Capture(jirix.Stdout(), jirix.Stderr()).Last("presubmit", args...); err != nil {
		return nil, err
	}

	return &test.Result{Status: test.Passed}, nil
}

// vanadiumPresubmitTest runs presubmit tests for a given project specified
// in TEST environment variable.
func vanadiumPresubmitTest(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	if err := requireEnv([]string{"BUILD_NUMBER", "REFS", "PROJECTS", "TEST", "WORKSPACE"}); err != nil {
		return nil, err
	}

	// Initialize the test.
	cleanup, err := initTestImpl(jirix, false, false, false, testName, nil, "")
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	displayProfiles(jirix, "presubmit")

	s := jirix.NewSeq()

	// Get number of test workers from opts.
	numWorkers := 1
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case NumWorkersOpt:
			numWorkers = int(typedOpt)
			break
		}
	}

	// Use the "presubmit test" command to run the presubmit test.
	args := []string{}
	if jirix.Verbose() {
		args = append(args, "-v")
	}
	name := os.Getenv("TEST")
	args = append(args,
		"-host", jenkinsHost,
		"test",
		"-build-number", os.Getenv("BUILD_NUMBER"),
		"-manifest", "tools",
		"-num-test-workers", fmt.Sprintf("%d", numWorkers),
		"-projects", os.Getenv("PROJECTS"),
		"-refs", os.Getenv("REFS"),
		"-test", name,
	)
	if err := s.Capture(jirix.Stdout(), jirix.Stderr()).Last("presubmit", args...); err != nil {
		return nil, newInternalError(err, "Presubmit")
	}

	// Remove any test result files that are empty.
	testResultFiles, err := findTestResultFiles(jirix, name)
	if err != nil {
		return nil, err
	}
	for _, file := range testResultFiles {
		fileInfo, err := s.Stat(file)
		if err != nil {
			return nil, err
		}
		if fileInfo.Size() == 0 {
			if err := s.RemoveAll(file).Done(); err != nil {
				return nil, err
			}
		}
	}

	return &test.Result{Status: test.Passed}, nil
}

// vanadiumPresubmitResult runs "presubmit result" command to process and post test results.
func vanadiumPresubmitResult(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	if err := requireEnv([]string{"BUILD_NUMBER", "REFS", "PROJECTS", "WORKSPACE"}); err != nil {
		return nil, err
	}

	// Initialize the test.
	cleanup, err := initTestImpl(jirix, false, false, false, testName, nil, "")
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Run "presubmit result".
	args := []string{}
	if jirix.Verbose() {
		args = append(args, "-v")
	}
	args = append(args,
		"-host", jenkinsHost,
		"result",
		"-build-number", os.Getenv("BUILD_NUMBER"),
		"-manifest", "tools",
		"-refs", os.Getenv("REFS"),
		"-projects", os.Getenv("PROJECTS"),
	)
	if err := jirix.NewSeq().Capture(jirix.Stdout(), jirix.Stderr()).Last("presubmit", args...); err != nil {
		return nil, err
	}

	return &test.Result{Status: test.Passed}, nil
}

// TestPresubmitTestOpt is an option used in testPresubmitTest for checking
// the content of a file.
type TestPresubmitTestOpt struct {
	FilePaths            []string
	ExpectedFileContents []string
}

func (TestPresubmitTestOpt) Opt() {}

// testPresubmitTest is used to test the "test" phase of presubmit.
// It reads a set of given files and compares their contents with the given
// expected contents. The test will pass when the file contents can be read and
// correctly matched.
func testPresubmitTest(jirix *jiri.X, testName string, opts ...Opt) (*test.Result, error) {
	testData := TestPresubmitTestOpt{}
	gotTestData := false
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case TestPresubmitTestOpt:
			testData = TestPresubmitTestOpt(typedOpt)
			gotTestData = true
		}
	}
	if !gotTestData {
		return nil, fmt.Errorf("TestPresubmitTestOpt not found")
	}

	status := test.Passed
	errMsg := "Failed to match file content"
	if len(testData.FilePaths) != len(testData.ExpectedFileContents) {
		status = test.Failed
		if err := xunit.CreateFailureReport(jirix, testName, "MockTest", "MockTest", errMsg, ""); err != nil {
			return nil, err
		}
	} else {
		for i, path := range testData.FilePaths {
			expectedContent := testData.ExpectedFileContents[i]
			content, err := jirix.NewSeq().ReadFile(path)
			if err != nil || expectedContent != string(content) {
				status = test.Failed
				if err := xunit.CreateFailureReport(jirix, testName, "MockTest", "MockTest", errMsg, ""); err != nil {
					return nil, err
				}
				break
			}
		}
	}
	return &test.Result{Status: status}, nil
}
