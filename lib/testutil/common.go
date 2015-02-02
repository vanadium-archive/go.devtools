package testutil

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"strings"

	"v.io/tools/lib/runutil"
	"v.io/tools/lib/util"
)

var (
	// cleanGo is used to control whether the initTest function removes
	// all stale Go object files and binaries. It is use to prevent the
	// test of this package from interfering with other concurrently
	// running tests that might be sharing the same object files.
	cleanGo = true
)

const (
	// Number of lines to be included in the error messsage of an xUnit report.
	numLinesToOutput = 50

	// The initial size of the buffer for storing command output in the
	// genXUnitReportOnCmdError function.
	largeBufferBytes = 1048576
)

// binDirPath returns the path to the directory for storing temporary
// binaries.
func binDirPath() string {
	return filepath.Join(os.Getenv("TMPDIR"), "bin")
}

// initTestHelper carries out the initial actions for the given test.
func initTestHelper(ctx *util.Context, testName string, profiles []string) (func() error, error) {
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

	// Remove xUnit test report file.
	if err := ctx.Run().RemoveAll(XUnitReportPath(testName)); err != nil {
		return nil, err
	}

	return func() error {
		return ctx.Run().Chdir(cwd)
	}, nil
}

// initTest initializes the given test and set of profiles, and wraps
// initTestHelper with genXUnitReportOnCmdError so errors can be captured
// in xUnit report.
//
// Possible return values:
// - _, _, err: initialization failed and so did the error handling logic.
// - _, result, nil: initialization failed and <result> captures the failure.
// - cleanup, nil, nil: initialization succeeded.
func initTest(ctx *util.Context, testName string, profiles []string) (func() error, *TestResult, error) {
	var cleanup func() error
	initTestFunc := func(opts runutil.Opts) error {
		var initError error
		cleanup, initError = initTestHelper(ctx, testName, profiles)
		return initError
	}
	if testResult, err := genXUnitReportOnCmdError(ctx, testName, "Init", "failure", initTestFunc); err != nil {
		return nil, nil, err
	} else if testResult != nil {
		return nil, testResult, nil
	}
	return cleanup, nil, nil
}

// genXUnitReportOnCmdError generates an xUnit test report if the given command
// function returns an error.
func genXUnitReportOnCmdError(ctx *util.Context, testName, testCaseName, failureSummary string, commandFunc func(runutil.Opts) error) (*TestResult, error) {
	largeBuffer := make([]byte, 0, largeBufferBytes)
	out := bytes.NewBuffer(largeBuffer)
	opts := ctx.Run().Opts()
	opts.Stdout = io.MultiWriter(out, opts.Stdout)
	opts.Stderr = io.MultiWriter(out, opts.Stderr)
	if err := commandFunc(opts); err != nil && err != io.ErrShortWrite {
		xUnitFilePath := XUnitReportPath(testName)

		// Only create the report when the xUnit file doesn't exist, is invalid, or
		// exist but doesn't have failed test cases.
		createXUnitFile := false
		if _, err := os.Stat(xUnitFilePath); err != nil {
			if os.IsNotExist(err) {
				createXUnitFile = true
			} else {
				return nil, fmt.Errorf("Stat(%s) failed: %v", xUnitFilePath, err)
			}
		} else {
			bytes, err := ioutil.ReadFile(xUnitFilePath)
			if err != nil {
				return nil, fmt.Errorf("ReadFile(%s) failed: %v", xUnitFilePath, err)
			}
			var existingSuites testSuites
			if err := xml.Unmarshal(bytes, &existingSuites); err != nil {
				createXUnitFile = true
			} else {
				createXUnitFile = true
				for _, curSuite := range existingSuites.Suites {
					if curSuite.Failures > 0 || curSuite.Errors > 0 {
						createXUnitFile = false
						break
					}
				}
			}
		}

		if createXUnitFile {
			// Create a test suite to wrap up the error.
			// Include last <numLinesToOutput> lines of the output in the error message.
			lines := strings.Split(out.String(), "\n")
			startLine := int(math.Max(0, float64(len(lines)-numLinesToOutput)))
			consoleOutput := "......\n" + strings.Join(lines[startLine:], "\n")
			errMsg := fmt.Sprintf("Error message:\n%s\n\nConsole output:\n%s", err.Error(), consoleOutput)
			s := createTestSuiteWithFailure(testName, testCaseName, failureSummary, errMsg, 0)
			suites := []testSuite{*s}

			if err := createXUnitReport(ctx, testName, suites); err != nil {
				return nil, err
			}

			// Return test result.
			if err == runutil.CommandTimedOutErr {
				return &TestResult{Status: TestTimedOut}, nil
			}
		}
		return &TestResult{Status: TestFailed}, nil
	}
	return nil, nil
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
