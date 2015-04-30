// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"go/build"
	"regexp"
	"strings"
)

type importRule struct {
	IsDenyRule bool
	PkgExpr    string
}

type packageConfig struct {
	Path    string
	Imports []importRule
}

type importResult int

const (
	undecidedResult importResult = iota
	approvedResult
	rejectedResult
)

func (r importResult) String() string {
	return []string{"undecided", "approved", "rejected"}[int(r)]
}

type importViolation struct {
	Src, Dst *build.Package
	Err      error
}

func (r importRule) enforce(p *build.Package) (importResult, error) {
	if r.PkgExpr == "..." {
		switch {
		case p.Goroot:
			return undecidedResult, nil
		case r.IsDenyRule:
			return rejectedResult, nil
		}
		return approvedResult, nil
	}

	re := regexp.QuoteMeta(r.PkgExpr)
	if strings.HasSuffix(re, `/\.\.\.`) {
		re = re[:len(re)-len(`/\.\.\.`)] + `(/.*)?`
	}

	switch matched, err := regexp.MatchString("^"+re+"$", p.ImportPath); {
	case err != nil:
		return undecidedResult, err
	case !matched:
		return undecidedResult, nil
	case r.IsDenyRule:
		return rejectedResult, nil
	}
	return approvedResult, nil
}

// verifyGo15InternalRule implements support for the internal package rule,
// which is supposed to be enabled for GOPATH packages in Go 1.5.  This logic
// can be removed after Go 1.5 is released.
//
// https://docs.google.com/document/d/1e8kOo3r51b2BWtTs_1uADIA5djfXhPT36s6eHVRIvaU
func verifyGo15InternalRule(src, dst string) bool {
	// The rule is that package "a/b/c/internal/d/e/f" can only be imported by
	// code rooted at "a/b/c".  The doc above isn't clear, but if there are
	// multiple occurrences of "internal", we apply the rule to the last (deepest)
	// occurrence.
	//
	// The only tricky part is to ensure path components are matched correctly.
	// E.g. when looking for the last path component that is "internal", we don't
	// want to match "Xinternal" or "internalX".
	const internal = "/internal"
	var root string
	if strings.HasSuffix(dst, internal) {
		root = dst[:len(dst)-len(internal)]
	} else if index := strings.LastIndex(dst, internal+"/"); index != -1 {
		root = dst[:index]
	}
	return root == "" || src == root || strings.HasPrefix(src, root+"/")
}

var errGo15Internal = errors.New("violates Go 1.5 internal package rule")

func checkImport(pkg, dep *build.Package) (*importViolation, error) {
	it := newPackageConfigIterator(pkg)
	for it.Advance() {
		config := it.Value()
		for _, rule := range config.Imports {
			switch result, err := rule.enforce(dep); {
			case err != nil:
				return nil, err
			case result == approvedResult:
				return nil, nil
			case result == rejectedResult:
				err := fmt.Errorf(`violates rule {"deny": %q} in %s`, rule.PkgExpr, config.Path)
				return &importViolation{pkg, dep, err}, nil
			}
		}
	}
	if err := it.Err(); err != nil {
		return nil, err
	}
	return nil, nil
}

func checkImports(pkg *build.Package) ([]importViolation, error) {
	var violations []importViolation
	// First check direct dependencies against the Go 1.5 internal package rule.
	deps := make(map[string]*build.Package)
	if err := packageDeps(pkg, deps, true, true); err != nil {
		return nil, err
	}
	for _, dep := range sortPackages(deps) {
		if !verifyGo15InternalRule(pkg.ImportPath, dep.ImportPath) {
			violations = append(violations, importViolation{pkg, dep, errGo15Internal})
		}
	}
	// Now check all transitive dependencies against the GO.PACKAGE rules.
	if err := packageDeps(pkg, deps, false, true); err != nil {
		return nil, err
	}
	for _, dep := range sortPackages(deps) {
		v, err := checkImport(pkg, dep)
		if err != nil {
			return nil, err
		}
		if v != nil {
			violations = append(violations, *v)
		}
	}
	return violations, nil
}
