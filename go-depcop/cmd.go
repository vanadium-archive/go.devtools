// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $V23_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go .

package main

import (
	"fmt"
	"go/build"

	"v.io/x/devtools/internal/tool"
	"v.io/x/lib/cmdline"
)

var (
	flagShowGoroot bool
	flagIndent     bool
	flagDirect     bool
)

const descDirect = "Only consider direct dependencies, rather than transitive dependencies."
const descGoroot = "Show packages in goroot."

func init() {
	cmdList.Flags.BoolVar(&flagIndent, "indent", false, "List dependencies with pretty indentation.")
	cmdList.Flags.BoolVar(&flagDirect, "direct", false, descDirect)
	cmdList.Flags.BoolVar(&flagShowGoroot, "show-goroot", false, descGoroot)
	cmdListImporters.Flags.BoolVar(&flagDirect, "direct", false, descDirect)
	cmdListImporters.Flags.BoolVar(&flagShowGoroot, "show-goroot", false, descGoroot)
}

func main() {
	cmdline.Main(cmdRoot)
}

var cmdRoot = &cmdline.Command{
	Name:  "go-depcop",
	Short: "Check Go package dependencies against user-defined rules",
	Long: `
Command go-depcop checks Go package dependencies against constraints described
in GO.PACKAGE files.  In addition to user-defined constraints, the Go 1.5
internal package rules are also enforced.
`,
	Children: []*cmdline.Command{cmdCheck, cmdList, cmdListImporters, cmdVersion},
}

var cmdCheck = &cmdline.Command{
	Runner:   cmdline.RunnerFunc(runCheck),
	Name:     "check",
	ArgsName: "<packages>",
	ArgsLong: "<packages> is a list of packages to check",
	Short:    "Check package dependency constraints",
	Long: `
Check package dependency constraints.

Every Go package directory may contain an optional GO.PACKAGE file.  Each file
specifies dependency rules, which either allow or deny imports by that package.
The files are traversed hierarchically, from the deepmost package to the root of
the source tree, until a matching rule is found.  If no matching rule is found,
the default behavior is to allow the dependency, to stay compatible with
existing packages that do not include dependency rules.

GO.PACKAGE is a JSON file that looks like this:
   {
     "imports": [
       {"allow": "pattern1/..."},
       {"allow": "pattern2"},
       {"deny":  "..."}
     ]
   }

Each item in "imports" is a rule, which either allows or denies imports based on
the given pattern.  Patterns that end with "/..." are special: "foo/..." means
that foo and all its subpackages match the rule.  The special-case pattern "..."
means that all packages in GOPATH, but not GOROOT, match the rule.
`}

func runCheck(env *cmdline.Env, args []string) error {
	// Gather packages specified in args.
	var pkgs []*build.Package
	paths, err := listPackagePaths(env, args...)
	if err != nil {
		return err
	}
	for _, path := range paths {
		pkg, err := importPackage(path)
		if err != nil {
			return err
		}
		pkgs = append(pkgs, pkg)
	}
	// Check each package.
	var violations []importViolation
	for _, pkg := range pkgs {
		v, err := checkImports(pkg)
		if err != nil {
			return err
		}
		violations = append(violations, v...)
	}
	for _, v := range violations {
		fmt.Fprintf(env.Stdout, "%q not allowed to import %q (%v)\n", v.Src.ImportPath, v.Dst.ImportPath, v.Err)
	}
	if len(violations) > 0 {
		return fmt.Errorf("dependency violation")
	}
	return nil
}

var cmdList = &cmdline.Command{
	Runner:   cmdline.RunnerFunc(runList),
	Name:     "list",
	ArgsName: "<packages>",
	ArgsLong: "<packages> is a list of packages",
	Short:    "List packages imported by the given packages",
	Long: `
List packages imported by the given <packages>.

Lists all transitive imports by default; set the -direct flag to limit the
listing to direct imports by the given <packages>.

Elides $GOROOT packages by default; set the -show-goroot flag to show packages
in $GOROOT.  If any of the given <packages> are $GOROOT packages, list behaves
as if -show-goroot were set to true.

Lists each imported package exactly once.  Set the -indent flag for pretty
indentation to help visualize the dependency hierarchy.  Setting -indent may
cause the same package to be listed multiple times.
`}

func runList(env *cmdline.Env, args []string) error {
	// Gather packages specified in args.
	var pkgs []*build.Package
	paths, err := listPackagePaths(env, args...)
	if err != nil {
		return err
	}
	showGoroot := flagShowGoroot
	for _, path := range paths {
		pkg, err := importPackage(path)
		if err != nil {
			return err
		}
		if pkg.Goroot {
			// If any package in args is a GOROOT package, always show GOROOT deps.
			showGoroot = true
		}
		pkgs = append(pkgs, pkg)
	}
	if flagIndent {
		// Print indented deps for each package.
		for _, pkg := range pkgs {
			if err := printIndentedDeps(env.Stdout, pkg, 0, flagDirect, showGoroot); err != nil {
				return err
			}
		}
		return nil
	}
	// Print deps for all combined packages.
	deps := make(map[string]*build.Package)
	for _, pkg := range pkgs {
		if err := packageDeps(pkg, deps, flagDirect, showGoroot); err != nil {
			return err
		}
	}
	for _, dep := range sortPackages(deps) {
		fmt.Fprintln(env.Stdout, dep.ImportPath)
	}
	return nil
}

var cmdListImporters = &cmdline.Command{
	Runner:   cmdline.RunnerFunc(runListImporters),
	Name:     "list-importers",
	ArgsName: "<packages>",
	ArgsLong: "<packages> is a list of packages",
	Short:    "List packages that import the given packages",
	Long: `
List packages that import the given <packages>; the reverse of "list".  The
listed packages are called "importers".

Lists all transitive importers by default; set the -direct flag to limit the
listing to importers that directly import the given <packages>.

Elides $GOROOT packages by default; set the -show-goroot flag to show importers
in $GOROOT.  If any of the given <packages> are $GOROOT packages, list-reverse
behaves as if -show-goroot were set to true.

Lists each importer package exactly once.
`}

func runListImporters(env *cmdline.Env, args []string) error {
	// Gather target packages specified in args.
	targets := make(map[string]*build.Package)
	targetPaths, err := listPackagePaths(env, args...)
	if err != nil {
		return err
	}
	showGoroot := flagShowGoroot
	for _, path := range targetPaths {
		pkg, err := importPackage(path)
		if err != nil {
			return err
		}
		if pkg.Goroot {
			// If any package in args is a GOROOT package, always show GOROOT deps.
			showGoroot = true
		}
		targets[path] = pkg
	}
	// Gather all known packages.
	allPaths, err := listPackagePaths(env, "all")
	if err != nil {
		return err
	}
	// Print every package that has dependencies that overlap with the targets.
	matches := make(map[string]*build.Package)
	for _, path := range allPaths {
		pkg, err := importPackage(path)
		if err != nil {
			return err
		}
		deps := make(map[string]*build.Package)
		if err := packageDeps(pkg, deps, flagDirect, showGoroot); err != nil {
			return err
		}
		if hasOverlap(deps, targets) {
			matches[path] = pkg
		}
	}
	for _, pkg := range sortPackages(matches) {
		fmt.Fprintln(env.Stdout, pkg.ImportPath)
	}
	return nil
}

var cmdVersion = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runVersion),
	Name:   "version",
	Short:  "Print version",
	Long:   "Print version of the go-depcop tool.",
}

func runVersion(env *cmdline.Env, _ []string) error {
	fmt.Fprintf(env.Stdout, "go-depcop tool version %v\n", tool.Version)
	return nil
}
