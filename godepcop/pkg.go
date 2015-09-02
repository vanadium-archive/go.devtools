// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"go/build"
	"io"
	"sort"
	"strings"

	"v.io/jiri/lib/tool"
	"v.io/x/devtools/internal/goutil"
	"v.io/x/lib/cmdline"
	"v.io/x/lib/set"
)

var (
	pseudoPackageC      = &build.Package{ImportPath: "C", Goroot: true}
	pseudoPackageUnsafe = &build.Package{ImportPath: "unsafe", Goroot: true}
	pkgCache            = map[string]*build.Package{"C": pseudoPackageC, "unsafe": pseudoPackageUnsafe}
)

func isPseudoPackage(p *build.Package) bool {
	return p == pseudoPackageUnsafe || p == pseudoPackageC
}

func listPackagePaths(env *cmdline.Env, args ...string) ([]string, error) {
	return goutil.List(tool.NewContextFromEnv(env), args...)
}

// importPackage loads and returns the package with the given package path.
func importPackage(path string) (*build.Package, error) {
	if p, ok := pkgCache[path]; ok {
		return p, nil
	}
	p, err := build.Import(path, "", build.AllowBinary)
	if err != nil {
		return nil, err
	}
	pkgCache[path] = p
	return p, nil
}

// depOpts holds options for computing package dependencies.
type depOpts struct {
	DirectOnly    bool // Only compute direct (rather than transitive) deps.
	IncludeGoroot bool // Include $GOROOT packages.
	IncludeTest   bool // Also include TestImports
	IncludeXTest  bool // Also include TestImports and XTestImports.
}

// Paths returns the initial package paths to use when computing dependencies.
func (x depOpts) Paths(pkg *build.Package) []string {
	uniq := map[string]struct{}{}
	set.String.Union(uniq, set.String.FromSlice(pkg.Imports))
	if x.IncludeTest || x.IncludeXTest {
		set.String.Union(uniq, set.String.FromSlice(pkg.TestImports))
	}
	if x.IncludeXTest {
		set.String.Union(uniq, set.String.FromSlice(pkg.XTestImports))
	}
	// Don't include the package itself; it was added by XTestImports.
	delete(uniq, pkg.ImportPath)
	paths := set.String.ToSlice(uniq)
	sort.Strings(paths)
	return paths
}

// PrintIdent prints pkg and its dependencies to w, with fancy indentation.
func (x depOpts) PrintIndent(w io.Writer, pkg *build.Package) error {
	fmt.Fprintln(w, "#"+pkg.ImportPath)
	return x.printIndentHelper(w, x.Paths(pkg), 0)
}

func (x depOpts) printIndentHelper(w io.Writer, paths []string, depth int) error {
	for _, path := range paths {
		pkg, err := importPackage(path)
		if err != nil {
			return err
		}
		if !x.IncludeGoroot && pkg.Goroot {
			continue
		}
		fmt.Fprintln(w, strings.Repeat(" │", depth)+" ├─"+pkg.ImportPath)
		if !x.DirectOnly {
			if err := x.printIndentHelper(w, pkg.Imports, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

// Deps fills deps with the dependencies of pkg.  If directOnly is true, only
// direct dependencies are printed, not transitive dependencies.
func (x depOpts) Deps(pkg *build.Package, deps map[string]*build.Package) error {
	return x.depsHelper(x.Paths(pkg), deps)
}

func (x depOpts) depsHelper(paths []string, deps map[string]*build.Package) error {
	for _, path := range paths {
		if deps[path] != nil {
			continue
		}
		pkg, err := importPackage(path)
		if err != nil {
			return err
		}
		if !x.IncludeGoroot && pkg.Goroot {
			continue
		}
		deps[path] = pkg
		if !x.DirectOnly {
			if err := x.depsHelper(pkg.Imports, deps); err != nil {
				return err
			}
		}
	}
	return nil
}

func hasOverlap(a, b map[string]*build.Package) bool {
	if len(a) > len(b) {
		a, b = b, a
	}
	for key, _ := range a {
		if b[key] != nil {
			return true
		}
	}
	return false
}

type pkgSorter []*build.Package

func (s pkgSorter) Len() int           { return len(s) }
func (s pkgSorter) Less(i, j int) bool { return s[i].ImportPath < s[j].ImportPath }
func (s pkgSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// sortPackages returns the packages in pkgs, sorting all GOROOT packages first,
// followed by sorted non-GOROOT packages.
func sortPackages(pkgs map[string]*build.Package) []*build.Package {
	var roots, nonroots pkgSorter
	for _, pkg := range pkgs {
		if pkg.Goroot {
			roots = append(roots, pkg)
		} else {
			nonroots = append(nonroots, pkg)
		}
	}
	sort.Sort(roots)
	sort.Sort(nonroots)
	var result []*build.Package
	result = append(result, roots...)
	result = append(result, nonroots...)
	return result
}
