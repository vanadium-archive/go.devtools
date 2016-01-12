// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go -env=CMDLINE_PREFIX=jiri .

package main

import (
	"fmt"
	"runtime"
	"strings"

	"v.io/jiri/jiri"
	"v.io/jiri/profiles/profilescmdline"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/test"
	jiriTest "v.io/x/devtools/jiri-test/internal/test"
	"v.io/x/devtools/jiri-v23-profile/v23_profile"
	"v.io/x/lib/cmdline"
)

var (
	blessingsRootFlag string
	cleanGoFlag       bool
	namespaceRootFlag string
	numWorkersFlag    int
	outputDirFlag     string
	partFlag          int
	pkgsFlag          string
	oauthBlesserFlag  string
	adminRoleFlag     string
	publisherRoleFlag string
	readerFlags       profilescmdline.ReaderFlagValues
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
	tool.InitializeRunFlags(&cmdTest.Flags)
	profilescmdline.RegisterReaderFlags(&cmdTest.Flags, &readerFlags, v23_profile.DefaultDBFilename)

}

// cmdTest represents the "jiri test" command.
var cmdTest = &cmdline.Command{
	Name:     "test",
	Short:    "Manage vanadium tests",
	Long:     "Manage vanadium tests.",
	Children: []*cmdline.Command{cmdTestProject, cmdTestRun, cmdTestList},
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
