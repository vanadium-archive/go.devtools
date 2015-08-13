// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
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

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/devtools/internal/xunit"
	"v.io/x/lib/cmdline"
)

const (
	// gsPrefix identifies the prefix of a Google Storage location where
	// the test results are stored.
	gsPrefix = "gs://vanadium-test-results/v0/"
)

var (
	reviewTargetRefsFlag string
	testFlag             string
	testPartRE           = regexp.MustCompile(`(.*)-part(\d)$`)
)

func init() {
	cmdTest.Flags.IntVar(&jenkinsBuildNumberFlag, "build-number", -1, "The number of the Jenkins build.")
	cmdTest.Flags.StringVar(&manifestFlag, "manifest", "", "Name of the project manifest.")
	cmdTest.Flags.StringVar(&projectsFlag, "projects", "", "The base names of the remote projects containing the CLs pointed by the refs, separated by ':'.")
	cmdTest.Flags.StringVar(&reviewTargetRefsFlag, "refs", "", "The review references separated by ':'.")
	cmdTest.Flags.StringVar(&testFlag, "test", "", "The name of a single test to run.")
}

// cmdTest represents the 'test' command of the presubmit tool.
var cmdTest = &cmdline.Command{
	Name:  "test",
	Short: "Run tests for a CL",
	Long: `
This subcommand pulls the open CLs from Gerrit, runs tests specified in a config
file, and posts test results back to the corresponding Gerrit review thread.
`,
	Runner: cmdline.RunnerFunc(runTest),
}

const (
	mergeConflictTestClass    = "merge conflict"
	mergeConflictMessageTmpl  = "Possible merge conflict detected in %s.\nPresubmit tests will be executed after a new patchset that resolves the conflicts is submitted."
	nanoToMiliSeconds         = 1000000
	prepareTestBranchAttempts = 3
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
func runTest(cmdlineEnv *cmdline.Env, args []string) (e error) {
	ctx := tool.NewContextFromEnv(cmdlineEnv, tool.ContextOpts{
		Color:    &colorFlag,
		DryRun:   &dryRunFlag,
		Manifest: &manifestFlag,
		Verbose:  &verboseFlag,
	})

	// Basic sanity checks.
	if err := sanityChecks(ctx, cmdlineEnv); err != nil {
		return err
	}

	// Record the current timestamp so we can get the correct postsubmit build
	// when processing the results.
	curTimestamp := time.Now().UnixNano() / nanoToMiliSeconds

	// Generate cls from the refs and projects flags.
	cls, err := parseCLs()
	if err != nil {
		return err
	}

	projects, tools, err := util.ReadManifest(ctx)
	if err != nil {
		return err
	}

	// tmpBinDir is where developer tools are built after changes are
	// pulled from the target CLs.
	tmpBinDir := filepath.Join(vroot, "tmpBin")

	// Setup cleanup function for cleaning up presubmit test branch.
	cleanupFn := func() error {
		os.RemoveAll(tmpBinDir)
		return cleanupAllPresubmitTestBranches(ctx, projects)
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
		if failedCL, err := preparePresubmitTestBranch(ctx, cls, projects); err != nil {
			if i > 1 {
				fmt.Fprintf(ctx.Stdout(), "Attempt #%d:\n", i)
			}
			if failedCL != nil {
				fmt.Fprintf(ctx.Stderr(), "%s: %v\n", failedCL.String(), err)
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
				if err := recordMergeConflict(ctx, failedCL, testName); err != nil {
					return err
				}
				return nil
			}
			return err
		}
		break
	}

	// Rebuild developer tools and override V23_ROOT/devtools/bin.
	env, errs := rebuildDeveloperTools(ctx, projects, tools, tmpBinDir)
	if len(errs) > 0 {
		// Don't fail on errors.
		for _, err := range errs {
			printf(ctx.Stderr(), "%v\n", err)
		}
	}

	// Run the tests via "v23 test run" and collect the test results.
	printf(ctx.Stdout(), "### Running the presubmit test\n")
	outputDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return ctx.Run().RemoveAll(outputDir) }, &e)

	v23Args := []string{
		"test", "run",
		"-output-dir", outputDir,
	}
	if partIndex != -1 {
		v23Args = append(v23Args, "-part", fmt.Sprintf("%d", partIndex))
	}
	v23Args = append(v23Args, testName)

	opts := ctx.Run().Opts()
	opts.Env = env
	if err := ctx.Run().CommandWithOpts(opts, "v23", v23Args...); err != nil {
		// Check the error status to differentiate failed test errors.
		exiterr, ok := err.(*exec.ExitError)
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
	bytes, err := ctx.Run().ReadFile(resultsFile)
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
	if err := persistTestData(ctx, outputDir, testName, partIndex, path); err != nil {
		fmt.Fprintf(ctx.Stderr(), "failed to store test results: %v\n", err)
	}

	return writeTestStatusFile(ctx, *result, curTimestamp, testName, partIndex)
}

// persistTestData uploads test data to Google Storage.
func persistTestData(ctx *tool.Context, outputDir string, testName string, partIndex int, path string) error {
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
	if err := ctx.Run().WriteFile(confFile, bytes, os.FileMode(0600)); err != nil {
		return err
	}
	if partIndex == -1 {
		partIndex = 0
	}
	// Upload test data to Google Storage.
	dstDir := fmt.Sprintf("%s/%s/%d", path, testName, partIndex)
	args := []string{"-q", "-m", "cp", filepath.Join(outputDir, "*"), dstDir}
	if err := ctx.Run().Command("gsutil", args...); err != nil {
		return err
	}
	xUnitFile := xunit.ReportPath(testName)
	if _, err := ctx.Run().Stat(xUnitFile); err == nil {
		args := []string{"-q", "cp", xUnitFile, dstDir + "/" + "xunit.xml"}
		if err := ctx.Run().Command("gsutil", args...); err != nil {
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
func sanityChecks(ctx *tool.Context, env *cmdline.Env) error {
	manifestFilePath, err := util.ManifestFile(manifestFlag)
	if err != nil {
		return err
	}
	if _, err := ctx.Run().Stat(manifestFilePath); err != nil {
		return err
	}
	if projectsFlag == "" {
		return env.UsageErrorf("-projects flag is required")
	}
	if reviewTargetRefsFlag == "" {
		return env.UsageErrorf("-refs flag is required")
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
func preparePresubmitTestBranch(ctx *tool.Context, cls []cl, projects map[string]util.Project) (_ *cl, e error) {
	strCLs := []string{}
	for _, cl := range cls {
		strCLs = append(strCLs, cl.String())
	}
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("Getwd() failed: %v", err)
	}
	defer collect.Error(func() error { return ctx.Run().Chdir(wd) }, &e)
	if err := cleanupAllPresubmitTestBranches(ctx, projects); err != nil {
		return nil, fmt.Errorf("%v\n", err)
	}
	// Pull changes for each cl.
	printf(ctx.Stdout(), "### Preparing to test %s\n", strings.Join(strCLs, ", "))
	prepareFn := func(curCL cl) error {
		localRepo, ok := projects[curCL.project]
		if !ok {
			return fmt.Errorf("project %q not found", curCL.project)
		}
		localRepoDir := localRepo.Path
		if err := ctx.Run().Chdir(localRepoDir); err != nil {
			return fmt.Errorf("Chdir(%v) failed: %v", localRepoDir, err)
		}
		branchName := presubmitTestBranchName(curCL.ref)
		if err := ctx.Git().CreateAndCheckoutBranch(branchName); err != nil {
			return err
		}
		if err := ctx.Git().Pull(util.VanadiumGitHost()+localRepo.Name, curCL.ref); err != nil {
			return err
		}
		return nil
	}
	for _, cl := range cls {
		if err := prepareFn(cl); err != nil {
			test.Fail(ctx, "pull changes from %s\n", cl.String())
			return &cl, err
		}
		test.Pass(ctx, "pull changes from %s\n", cl.String())
	}
	return nil, nil
}

// recordMergeConflict records possible merge conflict in the test status file
// and xUnit report.
func recordMergeConflict(ctx *tool.Context, failedCL *cl, testName string) error {
	message := fmt.Sprintf(mergeConflictMessageTmpl, failedCL.String())
	if err := xunit.CreateFailureReport(ctx, testName, testName, "MergeConflict", message, message); err != nil {
		return nil
	}
	result := test.Result{
		Status:          test.MergeConflict,
		MergeConflictCL: failedCL.String(),
	}
	// We use math.MaxInt64 here so that the logic that tries to find the newest
	// build before the given timestamp terminates after the first iteration.
	if err := writeTestStatusFile(ctx, result, math.MaxInt64, testName, 0); err != nil {
		return err
	}
	return nil
}

// rebuildDeveloperTools rebuilds developer tools (e.g. v23, vdl..) in
// a temporary directory, which is used to replace
// V23_ROOT/devtools/bin in the PATH.
func rebuildDeveloperTools(ctx *tool.Context, projects util.Projects, tools util.Tools, tmpBinDir string) (map[string]string, []error) {
	errs := []error{}
	toolsProject, ok := projects["release.go.x.devtools"]
	env := map[string]string{}
	if !ok {
		errs = append(errs, fmt.Errorf("tools project not found, not rebuilding tools."))
	} else {
		// Find target Tools.
		targetTools := []util.Tool{}
		for name, tool := range tools {
			if name == "v23" || name == "vdl" || name == "godepcop" {
				targetTools = append(targetTools, tool)
			}
		}
		// Rebuild.
		for _, tool := range targetTools {
			if err := util.BuildTool(ctx, tmpBinDir, tool.Name, tool.Package, toolsProject); err != nil {
				errs = append(errs, err)
			}
		}
		// Create a new PATH that replaces V23_ROOT/devtools/bin with the
		// temporary directory in which the tools were rebuilt.
		env["PATH"] = strings.Replace(os.Getenv("PATH"), filepath.Join(vroot, "devtools", "bin"), tmpBinDir, -1)
	}
	return env, errs
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
func cleanupAllPresubmitTestBranches(ctx *tool.Context, projects util.Projects) (e error) {
	printf(ctx.Stdout(), "### Cleaning up\n")
	if err := util.CleanupProjects(ctx, projects, true); err != nil {
		return err
	}
	return nil
}

// writeTestStatusFile writes the given TestResult and timestamp to a JSON file.
// This file will be collected (along with the test report xUnit file) by the
// "master" presubmit project for generating final test results message.
//
// For more details, see comments in result.go.
func writeTestStatusFile(ctx *tool.Context, result test.Result, curTimestamp int64, testName string, partIndex int) error {
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
	if err := ctx.Run().WriteFile(statusFilePath, bytes, os.FileMode(0644)); err != nil {
		return fmt.Errorf("WriteFile(%v) failed: %v", statusFilePath, err)
	}
	return nil
}
