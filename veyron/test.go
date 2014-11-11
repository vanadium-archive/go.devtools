package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"tools/lib/cmdline"
	"tools/lib/util"
)

// cmdTest represents the "veyron test" command.
var cmdTest = &cmdline.Command{
	Name:     "test",
	Short:    "Manage veyron tests",
	Long:     "Manage veyron tests.",
	Children: []*cmdline.Command{cmdTestProject, cmdTestRun, cmdTestList},
}

// cmdTestProject represents the "veyron test project" command.
var cmdTestProject = &cmdline.Command{
	Run:   runTestProject,
	Name:  "project",
	Short: "Run tests for a veyron project",
	Long: `
Runs tests for a veyron project that is by the remote URL specified as
the command-line argument. Projects hosted on googlesource.com, can be
specified using the basename of the URL (e.g. "veyron.go.core" implies
"https://veyron.googlesource.com/veyron.go.core").
`,
	ArgsName: "<project>",
	ArgsLong: "<project> identifies the project for which to run tests.",
}

func runTestProject(command *cmdline.Command, args []string) error {
	if len(args) != 1 {
		return command.UsageErrorf("unexpected number of arguments")
	}
	ctx := util.NewContextFromCommand(command, verboseFlag)
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		return fmt.Errorf("TempDir() failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	if err := os.Chdir(tmpDir); err != nil {
		return err
	}
	project := args[0]
	if !strings.HasPrefix(project, "http") {
		project = "https://veyron.googlesource.com/" + project
	}
	results, err := util.RunProjectTests(ctx, project)
	if err != nil {
		return err
	}
	printSummary(ctx, results)
	return nil
}

// cmdTestRun represents the "veyron test run" command.
var cmdTestRun = &cmdline.Command{
	Run:      runTestRun,
	Name:     "run",
	Short:    "Run veyron tests",
	Long:     "Run veyron tests.",
	ArgsName: "<name ...>",
	ArgsLong: "<name ...> is a list names identifying the tests to run.",
}

func runTestRun(command *cmdline.Command, args []string) error {
	if len(args) == 0 {
		return command.UsageErrorf("unexpected number of arguments")
	}
	ctx := util.NewContextFromCommand(command, verboseFlag)
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		return fmt.Errorf("TempDir() failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	if err := os.Chdir(tmpDir); err != nil {
		return err
	}
	results, err := util.RunTests(ctx, args)
	if err != nil {
		return err
	}
	printSummary(ctx, results)
	return nil
}

func printSummary(ctx *util.Context, results map[string]*util.TestResult) {
	fmt.Fprintf(ctx.Stdout(), "\nSummary:\n")
	for name, result := range results {
		switch result.Status {
		case util.TestPassed:
			fmt.Fprintf(ctx.Stdout(), "%v PASSED\n", name)
		case util.TestFailed:
			fmt.Fprintf(ctx.Stdout(), "%v FAILED\n", name)
		case util.TestTimedOut:
			fmt.Fprintf(ctx.Stdout(), "%v TIMED OUT\n", name)
		case util.TestSkipped:
			fmt.Fprintf(ctx.Stdout(), "%v SKIPPED\n", name)
		}
	}
}

// cmdTestList represents the "veyron test list" command.
var cmdTestList = &cmdline.Command{
	Run:   runTestList,
	Name:  "list",
	Short: "List veyron tests",
	Long:  "List veyron tests.",
}

func runTestList(command *cmdline.Command, _ []string) error {
	ctx := util.NewContextFromCommand(command, verboseFlag)
	testDir, err := util.TestScriptDir()
	if err != nil {
		return err
	}
	fileInfoList, err := ioutil.ReadDir(testDir)
	if err != nil {
		return err
	}
	for _, fileInfo := range fileInfoList {
		// Only list test scripts that end with ".sh" and do
		// not contain the "common". Script names that contain
		// "common" are reserved for shell libraries for the
		// test scripts.
		if strings.HasSuffix(fileInfo.Name(), ".sh") && strings.Index(fileInfo.Name(), "common") == -1 {
			fmt.Fprintf(ctx.Stdout(), "%v\n", strings.TrimSuffix(fileInfo.Name(), ".sh"))
		}
	}
	return nil
}
