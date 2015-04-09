// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"go/build"

	"v.io/x/devtools/internal/tool"
	"v.io/x/lib/cmdline"
)

var (
	includeTestsFlag bool
	gorootFlag       bool
	prettyFlag       bool
	recursiveFlag    bool
	transitiveFlag   bool
	verboseFlag      bool
)

func init() {
	cmdCheck.Flags.BoolVar(&recursiveFlag, "r", false, "Check dependencies recursively.")
	cmdList.Flags.BoolVar(&gorootFlag, "show_goroot", false, "Show packages in goroot.")
	cmdList.Flags.BoolVar(&prettyFlag, "pretty_print", false, "Make output easy to read, indenting nested dependencies.")
	cmdList.Flags.BoolVar(&transitiveFlag, "transitive", false, "List transitive dependencies.")
	cmdRoot.Flags.BoolVar(&includeTestsFlag, "include_tests", false, "Include tests in computing dependencies.")
	cmdRoot.Flags.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
}

// Root returns a command that represents the root of the go-depcop tool.
func root() *cmdline.Command {
	return cmdRoot
}

var cmdRoot = &cmdline.Command{
	Name:  "go-depcop",
	Short: "checks Go package dependencies against constraints",
	Long: `
Command go-depcop checks Go package dependencies against constraints described
in GO.PACKAGE files.  Both incoming and outgoing dependencies may be configured,
and Go "internal" package rules are enforced.

GO.PACKAGE files are traversed hierarchically, from the deepmost package to
GOROOT, until a matching rule is found.  If no matching rule is found, the
default behavior is to allow the dependency, to stay compatible with existing
packages that do not include dependency rules.

GO.PACKAGE is a JSON file that looks like this:
   {
     "dependencies": {
       "outgoing": [
         {"allow": "allowpattern1/..."},
         {"deny": "denypattern"},
         {"allow": "pattern2"}
       ],
       "incoming": [
         {"allow": "pattern3"},
         {"deny": "pattern4"}
       ]
     }
   }
`,
	Children: []*cmdline.Command{cmdCheck, cmdList, cmdRevList, cmdVersion},
}

// cmdCheck represents the 'check' command of the go-depcop tool.
var cmdCheck = &cmdline.Command{
	Run:      runCheck,
	Name:     "check",
	Short:    "Check package dependency constraints",
	Long:     "Check package dependency constraints.",
	ArgsName: "<packages>",
	ArgsLong: "<packages> is a list of packages",
}

func runCheck(command *cmdline.Command, args []string) error {
	violations := []dependencyRuleReference{}

	for _, arg := range args {
		p, err := importPackage(arg)
		if err != nil {
			return err
		}
		var v []dependencyRuleReference
		v, err = verifyDependencyHierarchy(p, map[*build.Package]bool{}, nil, recursiveFlag)
		if err != nil {
			return err
		}
		violations = append(violations, v...)
	}

	for _, v := range violations {
		switch v.Direction {
		case outgoingDependency:
			fmt.Fprintf(command.Stdout(), "%q violates its outgoing rule by depending on %q:\n    {\"deny\": %q} (in %s)\n",
				v.Package.ImportPath, v.MatchingPackage.ImportPath, v.RuleSet[v.RuleIndex].PackageExpression, v.Path)
		case incomingDependency:
			if v.InternalPackage {
				fmt.Fprintf(command.Stdout(), "%q is inaccessible by package %q because it is internal\n", v.Package.ImportPath, v.MatchingPackage.ImportPath)
			} else {
				fmt.Fprintf(command.Stdout(), "%q violates incoming rule of package %q:\n    {\"deny\": %q} (in %s)\n",
					v.MatchingPackage.ImportPath, v.Package.ImportPath, v.RuleSet[v.RuleIndex].PackageExpression, v.Path)
			}
		}
	}

	if len(violations) > 0 {
		return fmt.Errorf("dependency violation")
	}

	return nil
}

// cmdList represents the 'list' command of the go-depcop tool.
var cmdList = &cmdline.Command{
	Run:      runList,
	Name:     "list",
	Short:    "List outgoing package dependencies",
	Long:     "List outgoing package dependencies.",
	ArgsName: "<packages>",
	ArgsLong: "<packages> is a list of packages",
}

func runList(command *cmdline.Command, args []string) error {
	if len(args) == 0 {
		return command.UsageErrorf("not enough arguments")
	}

	for _, arg := range args {
		p, err := importPackage(arg)
		if err != nil {
			return err
		}
		if err := printDependencyHierarchy(command.Stdout(), p, map[*build.Package]bool{}, 0); err != nil {
			return err
		}
	}
	return nil
}

// cmdRevList represents the 'rlist' command of the go-depcop tool.
var cmdRevList = &cmdline.Command{
	Run:      runRevList,
	Name:     "rlist",
	Short:    "List incoming package dependencies",
	Long:     "List incoming package dependencies.",
	ArgsName: "<packages>",
	ArgsLong: "<packages> is a list of packages",
}

// TODO(jsimsa): Implement transitive incoming dependencies as a
// fix-point.
func runRevList(command *cmdline.Command, args []string) error {
	if len(args) == 0 {
		return command.UsageErrorf("not enough arguments")
	}
	revDeps, err := computeIncomingDependencies()
	if err != nil {
		return err
	}
	for _, arg := range args {
		if deps, ok := revDeps[arg]; !ok {
			fmt.Fprintf(command.Stderr(), "package %v not found\n", arg)
		} else {
			for dep, _ := range deps {
				fmt.Fprintf(command.Stdout(), "%v\n", dep)
			}
		}
	}
	return nil
}

// cmdVersion represent the 'version' command of the go-depcop tool.
var cmdVersion = &cmdline.Command{
	Run:   runVersion,
	Name:  "version",
	Short: "Print version",
	Long:  "Print version of the go-depcop tool.",
}

func runVersion(command *cmdline.Command, _ []string) error {
	fmt.Fprintf(command.Stdout(), "go-depcop tool version %v\n", tool.Version)
	return nil
}
