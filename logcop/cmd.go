// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"strings"

	"v.io/x/devtools/internal/tool"
	"v.io/x/lib/cmdline"
)

var (
	interfacesFlag string
	progressFlag   bool
	verboseFlag    bool
	gofmtFlag      bool
	dryRunFlag     bool
	colorFlag      bool
)

func init() {
	cmdCheck.Flags.StringVar(&interfacesFlag, "interface", "", "Comma-separated list of interface packages (required)")
	cmdInject.Flags.StringVar(&interfacesFlag, "interface", "", "Comma-separated list of interface packages (required)")
	cmdInject.Flags.BoolVar(&gofmtFlag, "gofmt", true, "Automatically run gofmt on the modified files")
	cmdRoot.Flags.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
	cmdRoot.Flags.BoolVar(&dryRunFlag, "n", false, "Show what commands will run but do not execute them.")
	cmdRoot.Flags.BoolVar(&colorFlag, "color", true, "Use color to format output.")
	cmdRoot.Flags.BoolVar(&progressFlag, "progress", false, "Print verbose progress information.")
}

// root returns a command that represents the root of the logcop tool.
func root() *cmdline.Command {
	return cmdRoot
}

var cmdRoot = &cmdline.Command{
	Name:  "logcop",
	Short: "Tool for checking and injecting log statements in code",
	Long: `

Command logcop checks for and injects logging statements into Go source code.

When checking, it ensures that all implementations in <packages> of all exported
interfaces declared in packages passed to the -interface flag have an
appropriate logging construct.

When injecting, it modifies the source code to inject such logging constructs.

LIMITATIONS:

logcop requires the ` + logPackageQuotedImportPath + ` to be
imported as "` + logPackageIdentifier + `".  Aliasing the log package
to another name makes logcop ignore the calls.  Importing any
other package with the name "` + logPackageIdentifier + `" will
invoke undefined behavior.
`,
	Children: []*cmdline.Command{cmdCheck, cmdInject, cmdVersion},
}

// cmdCheck represents the 'check' command of the logcop tool.
var cmdCheck = &cmdline.Command{
	Run:      runCheck,
	Name:     "check",
	Short:    "Check for log statements in public API implementations",
	Long:     "Check for log statements in public API implementations.",
	ArgsName: "<packages>",
	ArgsLong: "<packages> is the list of packages to be checked.",
}

// splitCommaSeparatedValues splits a comma-separated string
// containing a list of components to a slice of strings.
// It also cleans the whitespaces in each component and
// ignores empty components, so that "x, y,z," would be
// parsed to ["x", "y", "z"].
func splitCommaSeparatedValues(s string) []string {
	result := []string{}
	for _, v := range strings.Split(s, ",") {
		trimmed := strings.TrimSpace(v)
		if len(trimmed) > 0 {
			result = append(result, trimmed)
		}
	}
	return result
}

// runCheck handles the "check" command and executes
// the log injector in check-only mode.
func runCheck(command *cmdline.Command, args []string) error {
	interfacePackageList := splitCommaSeparatedValues(interfacesFlag)
	implementationPackageList := args
	if len(interfacePackageList) == 0 {
		return command.UsageErrorf("no interface packages listed")
	}

	if len(implementationPackageList) == 0 {
		return command.UsageErrorf("no implementation package listed")
	}
	ctx := tool.NewContextFromCommand(command, tool.ContextOpts{
		Color:   &colorFlag,
		DryRun:  &dryRunFlag,
		Verbose: &verboseFlag,
	})
	return executeInjector(ctx, interfacePackageList, implementationPackageList, true)
}

// cmdInject represents the 'inject' command of the logcop tool.
var cmdInject = &cmdline.Command{
	Run:   runInject,
	Name:  "inject",
	Short: "Inject log statements in public API implementations",
	Long: `Inject log statements in public API implementations.
Note that inject modifies <packages> in-place.  It is a good idea
to commit changes to version control before running this tool so
you can see the diff or revert the changes.
`,
	ArgsName: "<packages>",
	ArgsLong: "<packages> is the list of packages to inject log statements in.",
}

// runInject handles the "inject" command and executes
// the log injector in injection mode.
func runInject(command *cmdline.Command, args []string) error {
	ctx := tool.NewContextFromCommand(command, tool.ContextOpts{
		Color:   &colorFlag,
		DryRun:  &dryRunFlag,
		Verbose: &verboseFlag,
	})
	return executeInjector(ctx, splitCommaSeparatedValues(interfacesFlag), args, false)
}

// cmdVersion represents the 'version' command of the logcop tool.
var cmdVersion = &cmdline.Command{
	Run:   runVersion,
	Name:  "version",
	Short: "Print version",
	Long:  "Print version of the logcop tool.",
}

func runVersion(command *cmdline.Command, _ []string) error {
	fmt.Fprintf(command.Stdout(), "logcop tool version %v\n", tool.Version)
	return nil
}

// executeInjector creates a new LogInjector instance and runs it.
func executeInjector(ctx *tool.Context, interfacePackageList, implementationPackageList []string, checkOnly bool) error {
	return run(ctx, interfacePackageList, implementationPackageList, checkOnly)
}