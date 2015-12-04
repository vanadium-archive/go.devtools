// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"v.io/jiri/collect"
	"v.io/jiri/jiri"
	"v.io/jiri/project"
	"v.io/jiri/runutil"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/xunit"
	"v.io/x/lib/cmdline"
)

const (
	// gsPrefix identifies the prefix of a Google Storage location where
	// the test results are stored.
	gsPrefix = "gs://vanadium-test-results/v0/"

	// Timeout value for jiri-test command.
	jiriTestTimeout = time.Minute * 55
)

var (
	reviewTargetRefsFlag string
	testFlag             string
	testPartRE           = regexp.MustCompile(`(.*)-part(\d)$`)
)

func init() {
	cmdTest.Flags.IntVar(&jenkinsBuildNumberFlag, "build-number", -1, "The number of the Jenkins build.")
	cmdTest.Flags.StringVar(&projectsFlag, "projects", "", "The base names of the remote projects containing the CLs pointed by the refs, separated by ':'.")
	cmdTest.Flags.StringVar(&reviewTargetRefsFlag, "refs", "", "The review references separated by ':'.")
	cmdTest.Flags.StringVar(&testFlag, "test", "", "The name of a single test to run.")

	tool.InitializeProjectFlags(&cmdTest.Flags)
}

// cmdTest represents the 'test' command of the presubmit tool.
var cmdTest = &cmdline.Command{
	Name:  "test",
	Short: "Run tests for a CL",
	Long: `
This subcommand pulls the open CLs from Gerrit, runs tests specified in a config
file, and posts test results back to the corresponding Gerrit review thread.
`,
	Runner: jiri.RunnerFunc(runTest),
}

const (
	mergeConflictMessageTmpl     = "Possible merge conflict detected in %s.\nPresubmit tests will be executed after a new patchset that resolves the conflicts is submitted."
	toolsBuildFailureMessageTmpl = "Failed to build required tools. This is likely caused by your changes.\n%s"
	nanoToMiliSeconds            = 1000000
	prepareTestBranchAttempts    = 3
)

type cl struct {
	clNumber int
	patchset int
	ref      string
	project  string
}

func (c cl) String() string {
	return fmt.Sprintf("http://go/vcl/%d/%d", c.clNumber, c.patchset)
}

// runTest implements the 'test' subcommand.
func runTest(jirix *jiri.X, args []string) (e error) {
	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("Hostname() failed: %v", err)
	}
	printf(jirix.Stdout(), "### Running the presubmit binary on %s\n", hostname)

	// Basic sanity checks.
	if err := sanityChecks(jirix); err != nil {
		return err
	}

	// Warn users that presubmit will delete all non-master branches when
	// running on their local machines.
	if os.Getenv("USER") != "veyron" {
		fmt.Printf("WARNING: Presubmit will delete all non-master branches.\nContinue? y/N:")
		var response string
		if _, err := fmt.Scanf("%s\n", &response); err != nil || response != "y" {
			return fmt.Errorf("Test aborted by user.")
		}
	}

	// Record the current timestamp so we can get the correct postsubmit build
	// when processing the results.
	curTimestamp := time.Now().UnixNano() / nanoToMiliSeconds

	// Generate cls from the refs and projects flags.
	cls, err := parseCLs()
	if err != nil {
		return err
	}

	projects, tools, err := project.ReadManifest(jirix)
	if err != nil {
		return err
	}

	// tmpBinDir is where developer tools are built after changes are
	// pulled from the target CLs.
	tmpBinDir := filepath.Join(jirix.Root, "tmpBin")

	// Setup cleanup function for cleaning up presubmit test branch.
	cleanupFn := func() error {
		os.RemoveAll(tmpBinDir)
		return cleanupAllPresubmitTestBranches(jirix, projects)
	}
	defer collect.Error(func() error { return cleanupFn() }, &e)

	// Trap SIGTERM and SIGINT signal when the program is aborted
	// on Jenkins.
	go func() {
		sigchan := make(chan os.Signal, 1)
		signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM)
		<-sigchan
		if err := cleanupFn(); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}
		// Linux convention is to use 128+signal as the exit
		// code. We use 0 here to let Jenkins properly mark a
		// run as "Aborted" instead of "Failed".
		os.Exit(0)
	}()

	// Extract test name without part suffix and part index from the original
	// test name (stored in testFlag).
	testName, partIndex, err := processTestPartSuffix(testFlag)
	if err != nil {
		return err
	}

	// Prepare presubmit test branch.
	for i := 1; i <= prepareTestBranchAttempts; i++ {
		if failedCL, err := preparePresubmitTestBranch(jirix, cls, projects); err != nil {
			if i > 1 {
				fmt.Fprintf(jirix.Stdout(), "Attempt #%d:\n", i)
			}
			if failedCL != nil {
				fmt.Fprintf(jirix.Stderr(), "%s: %v\n", failedCL.String(), err)
			}
			errMsg := err.Error()
			if strings.Contains(errMsg, "unable to access") {
				// Cannot access googlesource.com, try again.
				continue
			}
			if strings.Contains(errMsg, "hung up") {
				// The remote end hung up unexpectedly, try again.
				continue
			}
			if strings.Contains(errMsg, "git pull") {
				// Possible merge conflict.
				message := fmt.Sprintf(mergeConflictMessageTmpl, failedCL.String())
				result := test.Result{
					Status:          test.MergeConflict,
					MergeConflictCL: failedCL.String(),
				}
				if err := recordPresubmitFailure(jirix, "MergeConflict", "Merge conflict detected", message, testName, -1, result); err != nil {
					return err
				}
				return nil
			}
			return err
		}
		break
	}

	// Rebuild developer tools and override PATH to point there.
	env, err := rebuildDeveloperTools(jirix, tools, tmpBinDir)
	if err != nil {
		message := fmt.Sprintf(toolsBuildFailureMessageTmpl, err.Error())
		result := test.Result{
			Status:               test.ToolsBuildFailure,
			ToolsBuildFailureMsg: err.Error(),
		}
		if err := recordPresubmitFailure(jirix, "BuildTools", "Failed to build tools", message, testName, -1, result); err != nil {
			return err
		}
		fmt.Fprintf(jirix.Stderr(), "failed to build tools:\n%s\n", err.Error())
		return nil
	}

	// Run the tests via "jiri test run" and collect the test results.
	printf(jirix.Stdout(), "### Running the presubmit test\n")
	s := jirix.NewSeq()
	outputDir, err := s.TempDir("", "")
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return jirix.NewSeq().RemoveAll(outputDir).Done() }, &e)

	jiriArgs := []string{
		"run",
		"-output-dir", outputDir,
	}
	if partIndex != -1 {
		jiriArgs = append(jiriArgs, "-part", fmt.Sprintf("%d", partIndex))
	}
	jiriArgs = append(jiriArgs, testName)

	var out bytes.Buffer
	out.Grow(1 << 20)
	stdout := io.MultiWriter(&out, jirix.Stdout())
	stderr := io.MultiWriter(&out, jirix.Stderr())
	if err := s.Env(env).Capture(stdout, stderr).Timeout(jiriTestTimeout).
		Last("jiri-test", jiriArgs...); err != nil {
		oe := runutil.GetOriginalError(err)
		// jiri-test command times out.
		if oe == runutil.CommandTimedOutErr {
			result := test.Result{
				Status:       test.TimedOut,
				TimeoutValue: jiriTestTimeout,
			}
			failureMessage := fmt.Sprintf("Test timed out after %v", jiriTestTimeout)
			if err := recordPresubmitFailure(jirix, "Timeout", failureMessage, out.String(), testName, partIndex, result); err != nil {
				return err
			}
		}
		// Check the error status to differentiate failed test errors.
		exiterr, ok := oe.(*exec.ExitError)
		if !ok {
			return err
		}
		status, ok := exiterr.Sys().(syscall.WaitStatus)
		if !ok {
			return err
		}
		if status.ExitStatus() != test.FailedExitCode {
			return err
		}
	}
	var results map[string]*test.Result
	resultsFile := filepath.Join(outputDir, "results")
	bytes, err := s.ReadFile(resultsFile)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(bytes, &results); err != nil {
		return fmt.Errorf("Unmarshal() failed: %v\n%v", err, string(bytes))
	}
	result, ok := results[testName]
	if !ok {
		return fmt.Errorf("no test result found for %q", testName)
	}

	// Upload the test results to Google Storage.
	path := gsPrefix + fmt.Sprintf("presubmit/%d/%s/%s", jenkinsBuildNumberFlag, os.Getenv("OS"), os.Getenv("ARCH"))
	if err := persistTestData(jirix, outputDir, testName, partIndex, path); err != nil {
		fmt.Fprintf(jirix.Stderr(), "failed to store test results: %v\n", err)
	}

	return writeTestStatusFile(jirix, *result, curTimestamp, testName, partIndex)
}

// persistTestData uploads test data to Google Storage.
func persistTestData(jirix *jiri.X, outputDir string, testName string, partIndex int, path string) error {
	// Write out a file that records the host configuration.
	conf := struct {
		Arch string
		OS   string
	}{
		Arch: runtime.GOARCH,
		OS:   runtime.GOOS,
	}
	bytes, err := json.Marshal(conf)
	if err != nil {
		return fmt.Errorf("Marshal(%v) failed: %v", conf, err)
	}
	confFile := filepath.Join(outputDir, "conf")
	s := jirix.NewSeq()
	if err := s.WriteFile(confFile, bytes, os.FileMode(0600)).Done(); err != nil {
		return err
	}
	if partIndex == -1 {
		partIndex = 0
	}
	// Upload test data to Google Storage.
	dstDir := fmt.Sprintf("%s/%s/%d", path, testName, partIndex)
	args := []string{"-q", "-m", "cp", filepath.Join(outputDir, "*"), dstDir}
	if err := s.Last("gsutil", args...); err != nil {
		return err
	}
	xUnitFile := xunit.ReportPath(testName)
	if _, err := s.Stat(xUnitFile); err == nil {
		args := []string{"-q", "cp", xUnitFile, dstDir + "/" + "xunit.xml"}
		if err := s.Last("gsutil", args...); err != nil {
			return err
		}
	} else {
		if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// sanityChecks performs basic sanity checks for various flags.
func sanityChecks(jirix *jiri.X) error {
	manifestFilePath := jirix.ManifestFile(tool.ManifestFlag)
	if _, err := jirix.NewSeq().Stat(manifestFilePath); err != nil {
		return err
	}
	if projectsFlag == "" {
		return jirix.UsageErrorf("-projects flag is required")
	}
	if reviewTargetRefsFlag == "" {
		return jirix.UsageErrorf("-refs flag is required")
	}
	return nil
}

// parseCLs parses cl info from refs and projects flag, and returns a
// slice of "cl" objects.
func parseCLs() ([]cl, error) {
	refs := strings.Split(reviewTargetRefsFlag, ":")
	projects := strings.Split(projectsFlag, ":")
	if got, want := len(refs), len(projects); got != want {
		return nil, fmt.Errorf("Mismatching lengths of %v and %v: %v vs. %v", refs, projects, len(refs), len(projects))
	}
	cls := []cl{}
	for i, ref := range refs {
		project := projects[i]
		clNumber, patchset, err := parseRefString(ref)
		if err != nil {
			return nil, err
		}
		cls = append(cls, cl{
			clNumber: clNumber,
			patchset: patchset,
			ref:      ref,
			project:  project,
		})
	}
	return cls, nil
}

// presubmitTestBranchName returns the name of the branch where the cl
// content is pulled.
func presubmitTestBranchName(ref string) string {
	return "presubmit_" + ref
}

// preparePresubmitTestBranch creates and checks out the presubmit
// test branch and pulls the CL there.
func preparePresubmitTestBranch(jirix *jiri.X, cls []cl, projects map[string]project.Project) (_ *cl, e error) {
	strCLs := []string{}
	for _, cl := range cls {
		strCLs = append(strCLs, cl.String())
	}
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("Getwd() failed: %v", err)
	}
	defer collect.Error(func() error { return jirix.NewSeq().Chdir(wd).Done() }, &e)
	if err := cleanupAllPresubmitTestBranches(jirix, projects); err != nil {
		return nil, fmt.Errorf("%v\n", err)
	}
	// Pull changes for each cl.
	printf(jirix.Stdout(), "### Preparing to test %s\n", strings.Join(strCLs, ", "))
	prepareFn := func(curCL cl) error {
		localRepo, ok := projects[curCL.project]
		if !ok {
			return fmt.Errorf("project %q not found", curCL.project)
		}
		localRepoDir := localRepo.Path
		if err := jirix.NewSeq().Chdir(localRepoDir).Done(); err != nil {
			return fmt.Errorf("Chdir(%v) failed: %v", localRepoDir, err)
		}
		branchName := presubmitTestBranchName(curCL.ref)
		if err := jirix.Git().CreateAndCheckoutBranch(branchName); err != nil {
			return err
		}
		gitHost, err := project.GitHost(jirix)
		if err != nil {
			return err
		}
		if err := jirix.Git().Pull(gitHost+localRepo.Name, curCL.ref); err != nil {
			return err
		}
		return nil
	}
	for _, cl := range cls {
		if err := prepareFn(cl); err != nil {
			test.Fail(jirix.Context, "pull changes from %s\n", cl.String())
			return &cl, err
		}
		test.Pass(jirix.Context, "pull changes from %s\n", cl.String())
	}
	return nil, nil
}

// recordPresubmitFailure records failure from presubmit binary itself
// (not from the test it runs) in the test status file and xUnit report.
func recordPresubmitFailure(jirix *jiri.X, testCaseName, failureMessage, failureOutput, testName string, partIndex int, result test.Result) error {
	if err := xunit.CreateFailureReport(jirix, testName, testName, testCaseName, failureMessage, failureOutput); err != nil {
		return nil
	}
	// We use math.MaxInt64 here so that the logic that tries to find the newest
	// build before the given timestamp terminates after the first iteration.
	if err := writeTestStatusFile(jirix, result, math.MaxInt64, testName, partIndex); err != nil {
		return err
	}
	return nil
}

// rebuildDeveloperTools rebuilds developer tools (e.g. jiri, vdl..) in a
// temporary directory, and overrides the PATH to use that directory.
func rebuildDeveloperTools(jirix *jiri.X, tools project.Tools, tmpBinDir string) (map[string]string, error) {
	if err := project.BuildTools(jirix, tools, tmpBinDir); err != nil {
		return nil, err
	}
	// Create a new PATH that replaces JIRI_ROOT/devtools/bin and
	// JIRI_ROOT/.jiri_root/bin with the temporary bin directory.
	//
	// TODO(toddw): Remove replacement of devtools/bin when the transition to
	// .jiri_root is done.
	oldBinDir := filepath.Join(jirix.Root, "devtools", "bin")
	path := os.Getenv("PATH")
	path = strings.Replace(path, oldBinDir, tmpBinDir, -1)
	path = strings.Replace(path, jirix.BinDir(), tmpBinDir, -1)
	return map[string]string{"PATH": path}, nil
}

// processTestPartSuffix extracts the test name without part suffix as well
// as the part index from the given test name that might have part suffix
// (vanadium-go-race_part0). If the given test name doesn't have part suffix,
// the returned test name will be the same as the given test name, and the
// returned part index will be -1.
func processTestPartSuffix(testName string) (string, int, error) {
	matches := testPartRE.FindStringSubmatch(testName)
	partIndex := -1
	if matches != nil {
		testName = matches[1]
		strPartIndex := matches[2]
		var err error
		if partIndex, err = strconv.Atoi(strPartIndex); err != nil {
			return "", partIndex, err
		}
	}
	return testName, partIndex, nil
}

// cleanupPresubmitTestBranch removes the presubmit test branch.
func cleanupAllPresubmitTestBranches(jirix *jiri.X, projects project.Projects) (e error) {
	printf(jirix.Stdout(), "### Cleaning up\n")
	if err := project.CleanupProjects(jirix, projects, true); err != nil {
		return err
	}
	return nil
}

// writeTestStatusFile writes the given TestResult and timestamp to a JSON file.
// This file will be collected (along with the test report xUnit file) by the
// "master" presubmit project for generating final test results message.
//
// For more details, see comments in result.go.
func writeTestStatusFile(jirix *jiri.X, result test.Result, curTimestamp int64, testName string, partIndex int) error {
	// Get the file path.
	workspace, fileName := os.Getenv("WORKSPACE"), fmt.Sprintf("status_%s.json", strings.Replace(testName, "-", "_", -1))
	statusFilePath := ""
	if workspace == "" {
		statusFilePath = filepath.Join(os.Getenv("HOME"), "tmp", testFlag, fileName)
	} else {
		statusFilePath = filepath.Join(workspace, fileName)
	}

	// Write to file.
	r := testResultInfo{
		Result:    result,
		TestName:  testName,
		Timestamp: curTimestamp,
		AxisValues: axisValuesInfo{
			Arch:      os.Getenv("ARCH"), // Architecture is stored in environment variable "ARCH"
			OS:        os.Getenv("OS"),   // OS is stored in environment variable "OS"
			PartIndex: partIndex,
		},
	}
	bytes, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("Marshal(%v) failed: %v", r, err)
	}
	if err := jirix.Run().WriteFile(statusFilePath, bytes, os.FileMode(0644)); err != nil {
		return fmt.Errorf("WriteFile(%v) failed: %v", statusFilePath, err)
	}
	return nil
}
