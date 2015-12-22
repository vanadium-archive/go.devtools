// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go .

package main

import (
	"strings"

	"v.io/jiri/jiri"
	"v.io/jiri/profiles/commandline"
	"v.io/jiri/profiles/reader"
	"v.io/jiri/tool"
	"v.io/x/lib/cmdline"
)

func main() {
	cmdline.Main(cmdRoot)
}

var (
	interfacesFlag       string
	progressFlag         bool
	gofmtFlag            bool
	diffOnlyFlag         bool
	useContextFlag       bool
	removeCallFlag       string
	injectCallFlag       string
	injectCallImportFlag string
	mergePoliciesFlag    reader.MergePolicies
)

const (
	apilogCall       = "LogCall"
	apilogImport     = "v.io/x/ref/lib/apilog"
	apilogRemoveCall = "apilog.LogCall"
)

func init() {
	cmdCheck.Flags.StringVar(&interfacesFlag, "interface", "", "Comma-separated list of interface packages (required).")

	cmdCheck.Flags.StringVar(&injectCallFlag, "call", apilogCall, "The function call to be checked for as defer <pkg>.<call>()() and defer <pkg>.<call>f(...)(...). The value of <pkg> is determined from --import.")
	cmdCheck.Flags.StringVar(&injectCallImportFlag, "import", apilogImport, "Import path for the injected call.")

	cmdInject.Flags.StringVar(&interfacesFlag, "interface", "", "Comma-separated list of interface packages (required).")
	cmdInject.Flags.BoolVar(&gofmtFlag, "gofmt", true, "Automatically run gofmt on the modified files.")
	cmdInject.Flags.BoolVar(&diffOnlyFlag, "diff-only", false, "Show changes that would be made without actually making them.")
	cmdInject.Flags.StringVar(&injectCallFlag, "call", apilogCall, "The function call to be injected as defer <pkg>.<call>()() and defer <pkg>.<call>f(...)(...). The value of <pkg> is determined from --import.")
	cmdInject.Flags.StringVar(&injectCallImportFlag, "import", apilogImport, "Import path for the injected call.")

	cmdRemove.Flags.BoolVar(&gofmtFlag, "gofmt", true, "Automatically run gofmt on the modified files.")
	cmdRemove.Flags.BoolVar(&diffOnlyFlag, "diff-only", false, "Show changes that would be made without actually making them.")
	cmdRemove.Flags.StringVar(&removeCallFlag, "call", apilogRemoveCall, "The function call to be removed. Note, that the package selector must be included. No attempt is made to remove the import declaration if the package is no longer used as a result of the removal.")

	cmdRoot.Flags.BoolVar(&progressFlag, "progress", false, "Print verbose progress information.")
	cmdRoot.Flags.BoolVar(&useContextFlag, "use-v23-context", true, "Pass a context.T argument (which must be of type v.io/v23/context.T), if available, to the injected call as its first parameter.")

	mergePoliciesFlag = reader.JiriMergePolicies()
	commandline.RegisterMergePoliciesFlag(&cmdRoot.Flags, &mergePoliciesFlag)
	tool.InitializeRunFlags(&cmdRoot.Flags)
}

var cmdRoot = &cmdline.Command{
	Name:  "gologcop",
	Short: "Tool for checking and injecting log statements in code",
	Long: `

Command gologcop checks for and injects logging statements into Go source code.

When checking, it ensures that all implementations in <packages> of all exported
interfaces declared in packages passed to the -interface flag have an
appropriate logging construct.

When injecting or removing, it modifies the source code to inject or remove
such logging constructs.

LIMITATIONS:

Removal will not automatically remove the package import for the call to
be removed.
`,
	Children: []*cmdline.Command{cmdCheck, cmdInject, cmdRemove},
}

// cmdCheck represents the 'check' command of the gologcop tool.
var cmdCheck = &cmdline.Command{
	Runner:   jiri.RunnerFunc(runCheck),
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
func runCheck(jirix *jiri.X, args []string) error {
	interfacePackageList := splitCommaSeparatedValues(interfacesFlag)
	implementationPackageList := args
	if len(interfacePackageList) == 0 {
		return jirix.UsageErrorf("no interface packages listed")
	}

	if len(implementationPackageList) == 0 {
		return jirix.UsageErrorf("no implementation package listed")
	}
	return runInjector(jirix, nil, interfacePackageList, implementationPackageList, true)
}

// cmdInject represents the 'inject' command of the gologcop tool.
var cmdInject = &cmdline.Command{
	Runner: jiri.RunnerFunc(runInject),
	Name:   "inject",
	Short:  "Inject log statements in public API implementations",
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
func runInject(jirix *jiri.X, args []string) error {
	return runInjector(jirix, nil, splitCommaSeparatedValues(interfacesFlag), args, false)
}

// cmdRemove represents the 'remove' command of the gologcop tool.
var cmdRemove = &cmdline.Command{
	Runner: jiri.RunnerFunc(runRemove),
	Name:   "remove",
	Short:  "Remove log statements",
	Long: `Remove log statements.
Note that remove modifies <packages> in-place.  It is a good idea
to commit changes to version control before running this tool so
you can see the diff or revert the changes.
`,
	ArgsName: "<packages>",
	ArgsLong: "<packages> is the list of packages to remove log statements from.",
}

// runRemove handles the "remove" command.
func runRemove(jirix *jiri.X, args []string) error {
	return runRemover(jirix, nil, args)
}
