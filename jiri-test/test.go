// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go -env=CMDLINE_PREFIX=jiri .

package main

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"v.io/jiri"
	"v.io/jiri/profiles/profilescmdline"
	"v.io/jiri/project"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/test"
	jiriTest "v.io/x/devtools/jiri-test/internal/test"
	"v.io/x/devtools/tooldata"
	"v.io/x/lib/cmdline"
	"v.io/x/lib/set"
)

var (
	blessingsRootFlag    string
	cleanGoFlag          bool
	mockTestFilePaths    string
	mockTestFileContents string
	namespaceRootFlag    string
	numWorkersFlag       int
	outputDirFlag        string
	partFlag             int
	pkgsFlag             string
	oauthBlesserFlag     string
	adminRoleFlag        string
	publisherRoleFlag    string
	readerFlags          profilescmdline.ReaderFlagValues
)

func init() {
	cmdTestRun.Flags.StringVar(&blessingsRootFlag, "blessings-root", "dev.v.io", "The blessings root.")
	cmdTestRun.Flags.StringVar(&namespaceRootFlag, "v23.namespace.root", "/ns.dev.v.io:8101", "The namespace root.")
	cmdTestRun.Flags.IntVar(&numWorkersFlag, "num-test-workers", runtime.NumCPU(), "Set the number of test workers to use; use 1 to serialize all tests.")
	cmdTestRun.Flags.Lookup("num-test-workers").DefValue = "<runtime.NumCPU()>"
	cmdTestRun.Flags.StringVar(&outputDirFlag, "output-dir", "", "Directory to output test results into.")
	cmdTestRun.Flags.IntVar(&partFlag, "part", -1, "Specify which part of the test to run.")
	cmdTestRun.Flags.StringVar(&pkgsFlag, "pkgs", "", "Comma-separated list of Go package expressions that identify a subset of tests to run; only relevant for Go-based tests. Example usage: jiri test run -pkgs v.io/x/ref vanadium-go-test")
	cmdTestRun.Flags.BoolVar(&cleanGoFlag, "clean-go", true, "Specify whether to remove Go object files and binaries before running the tests. Setting this flag to 'false' may lead to faster Go builds, but it may also result in some source code changes not being reflected in the tests (e.g., if the change was made in a different Go workspace).")
	cmdTestRun.Flags.StringVar(&mockTestFilePaths, "mock-file-paths", "", "Colon-separated file paths to read when testing presubmit test. This flag is only used when running presubmit end-to-end test.")
	cmdTestRun.Flags.StringVar(&mockTestFileContents, "mock-file-contents", "", "Colon-separated file contents to check when testing presubmit test. This flag is only used when running presubmit end-to-end test.")
	tool.InitializeRunFlags(&cmdTest.Flags)
	tool.InitializeProjectFlags(&cmdProjectPoll.Flags)
	profilescmdline.RegisterReaderFlags(&cmdTest.Flags, &readerFlags, jiri.ProfilesDBDir)
}

// cmdTest represents the "jiri test" command.
var cmdTest = &cmdline.Command{
	Name:     "test",
	Short:    "Manage vanadium tests",
	Long:     "Manage vanadium tests.",
	Children: []*cmdline.Command{cmdProjectPoll, cmdTestProject, cmdTestRun, cmdTestList},
}

// cmdTestProject represents the "jiri test project" command.
var cmdTestProject = &cmdline.Command{
	Runner: jiri.RunnerFunc(runTestProject),
	Name:   "project",
	Short:  "Run tests for a vanadium project",
	Long: `
Runs tests for a vanadium project that is by the remote URL specified as
the command-line argument. Projects hosted on googlesource.com, can be
specified using the basename of the URL (e.g. "vanadium.go.core" implies
"https://vanadium.googlesource.com/vanadium.go.core").
`,
	ArgsName: "<project>",
	ArgsLong: "<project> identifies the project for which to run tests.",
}

func runTestProject(jirix *jiri.X, args []string) error {
	jiriTest.ProfilesDBFilename = readerFlags.DBFilename
	if len(args) != 1 {
		return jirix.UsageErrorf("unexpected number of arguments")
	}
	project := args[0]
	results, err := jiriTest.RunProjectTests(jirix, nil, []string{project}, optsFromFlags()...)
	if err != nil {
		return err
	}
	printSummary(jirix, results)
	for _, result := range results {
		if result.Status != test.Passed {
			return cmdline.ErrExitCode(test.FailedExitCode)
		}
	}
	return nil
}

// cmdTestRun represents the "jiri test run" command.
var cmdTestRun = &cmdline.Command{
	Runner:   jiri.RunnerFunc(runTestRun),
	Name:     "run",
	Short:    "Run vanadium tests",
	Long:     "Run vanadium tests.",
	ArgsName: "<name...>",
	ArgsLong: "<name...> is a list names identifying the tests to run.",
}

func runTestRun(jirix *jiri.X, args []string) error {
	jiriTest.ProfilesDBFilename = readerFlags.DBFilename
	if len(args) == 0 {
		return jirix.UsageErrorf("unexpected number of arguments")
	}
	results, err := jiriTest.RunTests(jirix, nil, args, optsFromFlags()...)
	if err != nil {
		return err
	}
	printSummary(jirix, results)
	for _, result := range results {
		if result.Status != test.Passed {
			return cmdline.ErrExitCode(test.FailedExitCode)
		}
	}
	return nil
}

// cmdProjectPoll represents the "jiri project poll" command.
var cmdProjectPoll = &cmdline.Command{
	Runner: jiri.RunnerFunc(runProjectPoll),
	Name:   "poll",
	Short:  "Poll existing jiri projects",
	Long: `
Poll jiri projects that can affect the outcome of the given tests
and report whether any new changes in these projects exist. If no
tests are specified, all projects are polled by default.
`,
	ArgsName: "<test ...>",
	ArgsLong: "<test ...> is a list of tests that determine what projects to poll.",
}

// runProjectPoll generates a description of changes that exist
// remotely but do not exist locally.
func runProjectPoll(jirix *jiri.X, args []string) error {
	projectSet := map[string]struct{}{}
	if len(args) > 0 {
		config, err := tooldata.LoadConfig(jirix)
		if err != nil {
			return err
		}
		// Compute a map from tests to projects that can change the
		// outcome of the test.
		testProjects := map[string][]string{}
		for _, project := range config.Projects() {
			for _, test := range config.ProjectTests([]string{project}) {
				testProjects[test] = append(testProjects[test], project)
			}
		}
		for _, arg := range args {
			projects, ok := testProjects[arg]
			if !ok {
				return fmt.Errorf("failed to find any projects for test %q", arg)
			}
			set.String.Union(projectSet, set.String.FromSlice(projects))
		}
	}
	update, err := project.PollProjects(jirix, projectSet)
	if err != nil {
		return err
	}

	// Remove projects with empty changes.
	for project := range update {
		if changes := update[project]; len(changes) == 0 {
			delete(update, project)
		}
	}

	// Print update if it is not empty.
	if len(update) > 0 {
		bytes, err := json.MarshalIndent(update, "", "  ")
		if err != nil {
			return fmt.Errorf("MarshalIndent() failed: %v", err)
		}
		fmt.Fprintf(jirix.Stdout(), "%s\n", bytes)
	}
	return nil
}

func optsFromFlags() (opts []jiriTest.Opt) {
	if partFlag >= 0 {
		opt := jiriTest.PartOpt(partFlag)
		opts = append(opts, opt)
	}
	pkgs := []string{}
	for _, pkg := range strings.Split(pkgsFlag, ",") {
		if len(pkg) > 0 {
			pkgs = append(pkgs, pkg)
		}
	}
	opts = append(opts, jiriTest.PkgsOpt(pkgs))
	opts = append(opts,
		jiriTest.BlessingsRootOpt(blessingsRootFlag),
		jiriTest.NamespaceRootOpt(namespaceRootFlag),
		jiriTest.NumWorkersOpt(numWorkersFlag),
		jiriTest.OutputDirOpt(outputDirFlag),
		jiriTest.CleanGoOpt(cleanGoFlag),
		jiriTest.MergePoliciesOpt(readerFlags.MergePolicies),
	)
	if mockTestFilePaths != "" && mockTestFileContents != "" {
		opts = append(opts, jiriTest.TestPresubmitTestOpt{
			FilePaths:            strings.Split(mockTestFilePaths, ":"),
			ExpectedFileContents: strings.Split(mockTestFileContents, ":"),
		})
	}
	return
}

func printSummary(jirix *jiri.X, results map[string]*test.Result) {
	fmt.Fprintf(jirix.Stdout(), "SUMMARY:\n")
	for name, result := range results {
		fmt.Fprintf(jirix.Stdout(), "%v %s\n", name, result.Status)
		if len(result.ExcludedTests) > 0 {
			for pkg, tests := range result.ExcludedTests {
				fmt.Fprintf(jirix.Stdout(), "  excluded %d tests from package %v: %v\n", len(tests), pkg, tests)
			}
		}
		if len(result.SkippedTests) > 0 {
			for pkg, tests := range result.SkippedTests {
				fmt.Fprintf(jirix.Stdout(), "  skipped %d tests from package %v: %v\n", len(tests), pkg, tests)
			}
		}
	}
}

// cmdTestList represents the "jiri test list" command.
var cmdTestList = &cmdline.Command{
	Runner: jiri.RunnerFunc(runTestList),
	Name:   "list",
	Short:  "List vanadium tests",
	Long:   "List vanadium tests.",
}

func runTestList(jirix *jiri.X, _ []string) error {
	jiriTest.ProfilesDBFilename = readerFlags.DBFilename
	testList, err := jiriTest.ListTests()
	if err != nil {
		fmt.Fprintf(jirix.Stderr(), "%v\n", err)
		return err
	}
	for _, test := range testList {
		fmt.Fprintf(jirix.Stdout(), "%v\n", test)
	}
	return nil
}

func main() {
	cmdline.Main(cmdTest)
}
