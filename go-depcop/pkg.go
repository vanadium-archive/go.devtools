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

	"v.io/x/devtools/internal/goutil"
	"v.io/x/devtools/internal/tool"
	"v.io/x/lib/cmdline"
)

var (
	pseudoPackageC      = &build.Package{ImportPath: "C", Goroot: true}
	pseudoPackageUnsafe = &build.Package{ImportPath: "unsafe", Goroot: true}
	pkgCache            = map[string]*build.Package{"C": pseudoPackageC, "unsafe": pseudoPackageUnsafe}
)

func isPseudoPackage(p *build.Package) bool {
	return p == pseudoPackageUnsafe || p == pseudoPackageC
}

func listPackagePaths(cmd *cmdline.Command, args ...string) ([]string, error) {
	ctx := tool.NewContextFromCommand(cmd, tool.ContextOpts{Verbose: new(bool)})
	return goutil.List(ctx, args...)
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

// printIndentedDeps prints pkg and its dependencies to w, with fancy
// indentation.  If directOnly is true, only direct dependencies are printed,
// not transitive dependencies.
func printIndentedDeps(w io.Writer, pkg *build.Package, depth int, directOnly, showGoroot bool) error {
	header := "#"
	if depth > 0 {
		header = strings.Repeat(" │", depth-1) + " ├─"
	}
	fmt.Fprintln(w, header+pkg.ImportPath)
	if depth >= 1 && directOnly {
		return nil
	}
	for _, importPath := range append(pkg.Imports, pkg.TestImports...) {
		dep, err := importPackage(importPath)
		if err != nil {
			return err
		}
		if pkg.Goroot && !showGoroot {
			continue
		}
		if err := printIndentedDeps(w, dep, depth+1, directOnly, showGoroot); err != nil {
			return err
		}
	}
	return nil
}

// packageDeps fills deps with the dependencies of pkg.  If directOnly is true,
// only direct dependencies are printed, not transitive dependencies.
func packageDeps(pkg *build.Package, deps map[string]*build.Package, directOnly, showGoroot bool) error {
	for _, importPath := range append(pkg.Imports, pkg.TestImports...) {
		if deps[importPath] != nil {
			continue
		}
		dep, err := importPackage(importPath)
		if err != nil {
			return err
		}
		if dep.Goroot && !showGoroot {
			continue
		}
		deps[importPath] = dep
		if directOnly {
			continue
		}
		if err := packageDeps(dep, deps, directOnly, showGoroot); err != nil {
			return err
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
