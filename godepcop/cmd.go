// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $V23_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go .

package main

import (
	"fmt"
	"go/build"

	"v.io/jiri/lib/tool"
	"v.io/x/lib/cmdline"
)

var (
	flagStyle  string
	flagDirect bool
	flagGoroot bool
	flagTest   bool
	flagXTest  bool
)

const (
	styleSet    = "set"
	styleIndent = "indent"
	styleDot    = "dot"

	descDirect = "Only show direct dependencies, rather than showing transitive dependencies."
	descGoroot = "Show $GOROOT packages."
	descTest   = "Show imports from test files in the same package."
	descXTest  = "Show imports from test files in the same package or in the *_test package."
)

func init() {
	cmdList.Flags.StringVar(&flagStyle, "style", styleSet, `
List dependencies with the given style:
   set    - As a sorted set of unique packages.
   indent - As a hierarchical list with pretty indentation.
   dot    - As a DOT graph (http://www.graphviz.org)
`)
	cmdList.Flags.BoolVar(&flagDirect, "direct", false, descDirect)
	cmdList.Flags.BoolVar(&flagGoroot, "goroot", false, descGoroot)
	cmdList.Flags.BoolVar(&flagTest, "test", false, descTest)
	cmdList.Flags.BoolVar(&flagXTest, "xtest", false, descXTest)
	cmdListImporters.Flags.BoolVar(&flagDirect, "direct", false, descDirect)
	cmdListImporters.Flags.BoolVar(&flagGoroot, "goroot", false, descGoroot)
	cmdListImporters.Flags.BoolVar(&flagTest, "test", false, descTest)
	cmdListImporters.Flags.BoolVar(&flagXTest, "xtest", false, descXTest)
}

func main() {
	cmdline.Main(cmdRoot)
}

func depOptsFromFlags() depOpts {
	return depOpts{
		DirectOnly:    flagDirect,
		IncludeGoroot: flagGoroot,
		IncludeTest:   flagTest,
		IncludeXTest:  flagXTest,
	}
}

var cmdRoot = &cmdline.Command{
	Name:  "godepcop",
	Short: "Check Go package dependencies against user-defined rules",
	Long: `
Command godepcop checks Go package dependencies against constraints described in
.godepcop files.  In addition to user-defined constraints, the Go 1.5 internal
package rules are also enforced.
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

Every Go package directory may contain an optional .godepcop file.  Each file
specifies dependency rules, which either allow or deny imports by that package.
The files are traversed hierarchically, from the deepmost package to the root of
the source tree, until a matching rule is found.  If no matching rule is found,
the default behavior is to allow the dependency, to support packages that do not
have any dependency rules.

The .godepcop file is encoded in XML:

  <godepcop>
    <pkg allow="pattern1/..."/>
    <pkg allow="pattern2"/>
    <pkg deny="..."/>
    <test allow="pattern3"/>
    <xtest allow="..."/>
  </godepcop>

Each element in godepcop is a rule, which either allows or denies imports based
on the given pattern.  Patterns that end with "/..." are special: "foo/..."
means that foo and all its subpackages match the rule.  The special-case pattern
"..."  means that all packages in GOPATH, but not GOROOT, match the rule.

There are three groups of rules:
  pkg   - Rules applied to all imports from the package.
  test  - Extra rules for imports from all test files.
  xtest - Extra rules for imports from test files in the *_test package.

Rules in each group are processed in the order they appear in the .godepcop
file.  The transitive closure of the following imports are checked for each
package P:
  P.Imports                              - check pkg rules
  P.Imports+P.TestImports                - check test and pkg rules
  P.Imports+P.TestImports+P.XTestImports - check xtest, test and pkg rules
`}

func runCheck(env *cmdline.Env, args []string) error {
	// Gather packages specified in args.
	paths, err := listPackagePaths(env, args...)
	if err != nil {
		return err
	}
	var pkgs []*build.Package
	for _, path := range paths {
		pkg, err := importPackage(path)
		if err != nil {
			return err
		}
		pkgs = append(pkgs, pkg)
	}
	// Check each package.
	var violations []violation
	for _, pkg := range pkgs {
		v, err := checkDeps(pkg)
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

Elides $GOROOT packages by default; set the -goroot flag to include packages in
$GOROOT.  If any of the given <packages> are $GOROOT packages, list behaves as
if -goroot were set to true.

Lists each imported package exactly once when using the default -style=set.  See
the -style flag for alternate output styles.
`}

func runList(env *cmdline.Env, args []string) error {
	// Gather packages specified in args.
	paths, err := listPackagePaths(env, args...)
	if err != nil {
		return err
	}
	var pkgs []*build.Package
	opts := depOptsFromFlags()
	for _, path := range paths {
		pkg, err := importPackage(path)
		if err != nil {
			return err
		}
		if pkg.Goroot {
			// If any package in args is a GOROOT package, always include GOROOT deps.
			opts.IncludeGoroot = true
		}
		pkgs = append(pkgs, pkg)
	}
	switch flagStyle {
	case styleIndent:
		// Print indented deps for each package.
		for _, pkg := range pkgs {
			if err := opts.PrintIndent(env.Stdout, pkg); err != nil {
				return err
			}
		}
	case styleDot:
		if err := printDot(env.Stdout, pkgs, opts); err != nil {
			return err
		}
	default:
		// Print deps for all combined packages.
		deps := make(map[string]*build.Package)
		for _, pkg := range pkgs {
			if err := opts.Deps(pkg, deps); err != nil {
				return err
			}
		}
		for _, dep := range sortPackages(deps) {
			fmt.Fprintln(env.Stdout, dep.ImportPath)
		}
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

Elides $GOROOT packages by default; set the -goroot flag to include importers in
$GOROOT.  If any of the given <packages> are $GOROOT packages, list-importers
behaves as if -goroot were set to true.

Lists each importer package exactly once.
`}

func runListImporters(env *cmdline.Env, args []string) error {
	// Gather target packages specified in args.
	targetPaths, err := listPackagePaths(env, args...)
	if err != nil {
		return err
	}
	targets := make(map[string]*build.Package)
	opts := depOptsFromFlags()
	for _, path := range targetPaths {
		pkg, err := importPackage(path)
		if err != nil {
			return err
		}
		if pkg.Goroot {
			// If any package in args is a GOROOT package, always include GOROOT deps.
			opts.IncludeGoroot = true
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
		if err := opts.Deps(pkg, deps); err != nil {
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
	Long:   "Print version of the godepcop tool.",
}

func runVersion(env *cmdline.Env, _ []string) error {
	fmt.Fprintf(env.Stdout, "godepcop tool version %v\n", tool.Version)
	return nil
}
