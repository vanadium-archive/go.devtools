package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/template"

	"v.io/lib/cmdline"
	"v.io/tools/lib/collect"
	"v.io/tools/lib/gitutil"
	"v.io/tools/lib/testutil"
	"v.io/tools/lib/util"
)

// cmdTest represents the 'test' command of the presubmit tool.
var cmdTest = &cmdline.Command{
	Name:  "test",
	Short: "Run tests for a CL",
	Long: `
This subcommand pulls the open CLs from Gerrit, runs tests specified in a config
file, and posts test results back to the corresponding Gerrit review thread.
`,
	Run: runTest,
}

const failureReportTmpl = `<?xml version="1.0" encoding="utf-8"?>
<testsuites>
  <testsuite name="timeout" tests="1" errors="0" failures="1" skip="0">
    <testcase classname="{{.ClassName}}" name="{{.TestName}}" time="0">
      <failure type="error">
<![CDATA[
{{.ErrorMessage}}
]]>
      </failure>
    </testcase>
  </testsuite>
</testsuites>
`

const (
	mergeConflictTestClass   = "merge conflict"
	mergeConflictMessageTmpl = "Possible merge conflict detected in %s.\nPresubmit tests will be executed after a new patchset that resolves the conflicts is submitted."
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
func runTest(command *cmdline.Command, args []string) (e error) {
	ctx := util.NewContextFromCommand(command, !noColorFlag, dryRunFlag, verboseFlag)

	// Basic sanity checks.
	if err := sanityChecks(command); err != nil {
		return err
	}

	// Generate cls from the refs and projects flags.
	cls, err := parseCLs()
	if err != nil {
		return err
	}

	projects, tools, err := util.ReadManifest(ctx, manifestFlag)
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

	// Prepare presubmit test branch.
	if failedCL, err := preparePresubmitTestBranch(ctx, cls, projects); err != nil {
		if failedCL != nil {
			fmt.Fprintf(ctx.Stderr(), "%s: %v\n", failedCL.String(), err)
		}
		// Possible merge conflict.
		if strings.Contains(err.Error(), "git pull") {
			if err := recordMergeConflict(ctx, failedCL); err != nil {
				return err
			}
			return nil
		}
		return err
	}

	// Rebuild developer tools and override VANADIUM_ROOT/bin.
	env, errs := rebuildDeveloperTools(ctx, projects, tools, tmpBinDir)
	if len(errs) > 0 {
		// Don't fail on errors.
		for _, err := range errs {
			printf(ctx.Stderr(), "%v\n", err)
		}
	}

	// Run the tests.
	printf(ctx.Stdout(), "### Running the presubmit test\n")
	if results, err := testutil.RunTests(ctx, env, []string{testFlag}, testutil.ShortOpt(true)); err == nil {
		return writeTestStatusFile(ctx, results)
	} else {
		return err
	}
}

// sanityChecks performs basic sanity checks for various flags.
func sanityChecks(command *cmdline.Command) error {
	manifestFilePath, err := util.RemoteManifestFile(manifestFlag)
	if err != nil {
		return err
	}
	if _, err := os.Stat(manifestFilePath); err != nil {
		return fmt.Errorf("Stat(%q) failed: %v", manifestFilePath, err)
	}
	if projectsFlag == "" {
		return command.UsageErrorf("-projects flag is required")
	}
	if reviewTargetRefsFlag == "" {
		return command.UsageErrorf("-refs flag is required")
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
func preparePresubmitTestBranch(ctx *util.Context, cls []cl, projects map[string]util.Project) (_ *cl, e error) {
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
		if err := ctx.Git().Pull(util.VanadiumGitRepoHost()+localRepo.Name, curCL.ref); err != nil {
			return err
		}
		return nil
	}
	for _, cl := range cls {
		if err := prepareFn(cl); err != nil {
			testutil.Fail(ctx, "pull changes from %s\n", cl.String())
			return &cl, err
		}
		testutil.Pass(ctx, "pull changes from %s\n", cl.String())
	}
	return nil, nil
}

// recordMergeConflict records possible merge conflict in the test status file
// and xUnit report.
func recordMergeConflict(ctx *util.Context, failedCL *cl) error {
	message := fmt.Sprintf(mergeConflictMessageTmpl, failedCL.String())
	result := testutil.TestResult{
		Status:          testutil.TestFailedMergeConflict,
		MergeConflictCL: failedCL.String(),
	}
	if err := generateFailureReport(testFlag, mergeConflictTestClass, message); err != nil {
		return err
	}
	if err := writeTestStatusFile(ctx, map[string]*testutil.TestResult{testFlag: &result}); err != nil {
		return err
	}
	return nil
}

// rebuildDeveloperTools rebuilds developer tools (e.g. v23, vdl..) in a
// temporary directory, which is used to replace VANADIUM_ROOT/bin in the PATH.
func rebuildDeveloperTools(ctx *util.Context, projects util.Projects, tools util.Tools, tmpBinDir string) (map[string]string, []error) {
	errs := []error{}
	toolsProject, ok := projects["release.go.tools"]
	env := map[string]string{}
	if !ok {
		errs = append(errs, fmt.Errorf("tools project not found, not rebuilding tools."))
	} else {
		// Find target Tools.
		targetTools := []util.Tool{}
		for name, tool := range tools {
			if name == "v23" || name == "vdl" || name == "go-depcop" {
				targetTools = append(targetTools, tool)
			}
		}
		// Rebuild.
		for _, tool := range targetTools {
			if err := util.BuildTool(ctx, tmpBinDir, tool.Name, tool.Package, toolsProject); err != nil {
				errs = append(errs, err)
			}
		}
		// Create a new PATH that replaces VANADIUM_ROOT/bin
		// with the temporary directory in which the tools
		// were rebuilt.
		env["PATH"] = strings.Replace(os.Getenv("PATH"), filepath.Join(vroot, "bin"), tmpBinDir, -1)
	}
	return env, errs
}

// cleanupPresubmitTestBranch removes the presubmit test branch.
func cleanupAllPresubmitTestBranches(ctx *util.Context, projects map[string]util.Project) (e error) {
	printf(ctx.Stdout(), "### Cleaning up\n")
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer collect.Error(func() error { return ctx.Run().Chdir(wd) }, &e)
	for _, project := range projects {
		localProjectDir := project.Path
		if err := ctx.Run().Chdir(localProjectDir); err != nil {
			return fmt.Errorf("Chdir(%v) failed: %v", localProjectDir, err)
		}
		if err := resetLocalProject(ctx); err != nil {
			return err
		}
	}
	return nil
}

// resetLocalProject cleans up untracked files and uncommitted changes of the
// current branch, checks out the master branch, and deletes all the
// other branches.
func resetLocalProject(ctx *util.Context) error {
	// Clean up changes and check out master.
	curBranchName, err := ctx.Git().CurrentBranchName()
	if err != nil {
		return err
	}
	if curBranchName != "master" {
		if err := ctx.Git().CheckoutBranch("master", gitutil.Force); err != nil {
			return err
		}
	}
	if err := ctx.Git().RemoveUntrackedFiles(); err != nil {
		return err
	}
	// Discard any uncommitted changes.
	if err := ctx.Git().Reset("HEAD"); err != nil {
		return err
	}

	// Delete all the other branches.
	// At this point we should be at the master branch.
	branches, _, err := ctx.Git().GetBranches()
	if err != nil {
		return err
	}
	for _, branch := range branches {
		if branch == "master" {
			continue
		}
		if strings.HasPrefix(branch, "presubmit_refs") {
			if err := ctx.Git().DeleteBranch(branch, gitutil.Force); err != nil {
				return nil
			}
		}
	}

	return nil
}

// generateFailureReport generates a xunit test report file for
// the given failing test.
func generateFailureReport(testName string, className, errorMessage string) (e error) {
	type tmplData struct {
		ClassName    string
		TestName     string
		ErrorMessage string
	}
	tmpl, err := template.New("failureReport").Parse(failureReportTmpl)
	if err != nil {
		return fmt.Errorf("Parse(%q) failed: %v", failureReportTmpl, err)
	}
	reportFileName := fmt.Sprintf("tests_%s.xml", strings.Replace(testName, "-", "_", -1))
	reportFile := filepath.Join(vroot, "..", reportFileName)
	f, err := os.Create(reportFile)
	if err != nil {
		return fmt.Errorf("Create(%q) failed: %v", reportFile, err)
	}
	defer collect.Error(func() error { return f.Close() }, &e)
	return tmpl.Execute(f, tmplData{
		ClassName:    className,
		TestName:     testName,
		ErrorMessage: errorMessage,
	})
}

// writeTestStatusFile writes the given TestResult map to a JSON file.
// This file will be collected (along with the test report xunit file) by the
// "master" presubmit project for generating final test results message.
//
// For more details, see comments in result.go.
func writeTestStatusFile(ctx *util.Context, results map[string]*testutil.TestResult) error {
	// Get the file path.
	workspace, fileName := os.Getenv("WORKSPACE"), fmt.Sprintf("status_%s.json", strings.Replace(testFlag, "-", "_", -1))
	statusFilePath := ""
	if workspace == "" {
		statusFilePath = filepath.Join(os.Getenv("HOME"), "tmp", testFlag, fileName)
	} else {
		statusFilePath = filepath.Join(workspace, fileName)
	}

	// Write to file.
	bytes, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("Marshal(%v) failed: %v", results, err)
	}
	if err := ctx.Run().WriteFile(statusFilePath, bytes, os.FileMode(0644)); err != nil {
		return fmt.Errorf("WriteFile(%v) failed: %v", statusFilePath, err)
	}
	return nil
}
