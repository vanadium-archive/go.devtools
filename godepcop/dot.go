// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"go/build"
	"io"
	"strings"
)

func printDot(w io.Writer, pkgs []*build.Package, opts depOpts) error {
	fmt.Fprintf(w, `digraph {
  node[shape=record,style=solid]
  edge[arrowhead=vee]
  graph[rankdir=TB,splines=true]
`)
	// Print edges for each package in pkgs, possibly transitively.
	printed := make(map[*build.Package]bool)
	ids := make(map[*build.Package]int)
	for _, pkg := range pkgs {
		if err := printDotEdges(w, opts, printed, ids, pkg, opts.Paths(pkg)); err != nil {
			return err
		}
	}
	// Print nodes for each package in ids.
	idToPkg := make([]*build.Package, len(ids))
	for pkg, id := range ids {
		idToPkg[id] = pkg
	}
	for id := 0; id < len(ids); id++ {
		pkg := idToPkg[id]
		attrs := []string{fmt.Sprintf("label=%q", pkg.ImportPath)}
		if pkg.Goroot {
			attrs = append(attrs, "goroot=true")
		}
		fmt.Fprintf(w, "  %d[%s]\n", id, strings.Join(attrs, ","))
	}
	fmt.Fprintf(w, "}\n")
	return nil
}

func printDotEdges(w io.Writer, opts depOpts, printed map[*build.Package]bool, ids map[*build.Package]int, pkg *build.Package, paths []string) error {
	if printed[pkg] {
		return nil
	}
	if _, ok := ids[pkg]; !ok {
		ids[pkg] = len(ids)
	}
	var depIDs []string
	var deps []*build.Package
	for _, path := range paths {
		dep, err := importPackage(path)
		if err != nil {
			return err
		}
		if !opts.IncludeGoroot && dep.Goroot {
			continue
		}
		if _, ok := ids[dep]; !ok {
			ids[dep] = len(ids)
		}
		depIDs = append(depIDs, fmt.Sprintf("%d", ids[dep]))
		deps = append(deps, dep)
	}
	if len(depIDs) > 0 {
		fmt.Fprintf(w, "  %d->{%s}\n", ids[pkg], strings.Join(depIDs, " "))
	}
	printed[pkg] = true
	if !opts.DirectOnly {
		for _, dep := range deps {
			if err := printDotEdges(w, opts, printed, ids, dep, dep.Imports); err != nil {
				return err
			}
		}
	}
	return nil
}
