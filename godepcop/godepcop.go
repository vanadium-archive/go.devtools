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

type result int

const (
	resultUndecided result = iota
	resultApproved
	resultRejected
)

func (r result) String() string {
	return []string{"undecided", "approved", "rejected"}[int(r)]
}

type violation struct {
	Src, Dst *build.Package
	Err      error
}

func enforceRule(r rule, pkg *build.Package) (result, error) {
	if r.Pattern() == "..." {
		switch {
		case pkg.Goroot:
			return resultUndecided, nil
		case r.IsDeny():
			return resultRejected, nil
		}
		return resultApproved, nil
	}

	re := regexp.QuoteMeta(r.Pattern())
	if strings.HasSuffix(re, `/\.\.\.`) {
		re = re[:len(re)-len(`/\.\.\.`)] + `(/.*)?`
	}

	switch matched, err := regexp.MatchString("^"+re+"$", pkg.ImportPath); {
	case err != nil:
		return resultUndecided, err
	case !matched:
		return resultUndecided, nil
	case r.IsDeny():
		return resultRejected, nil
	}
	return resultApproved, nil
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

func checkDep(pkg, dep *build.Package, mode checkMode) (*violation, error) {
	it := newConfigIter(pkg)
	for it.Advance() {
		// Collect the ordered rules from this config for the given mode.
		cfg := it.Value()
		var rules []rule
		switch mode {
		case modeImport:
			rules = cfg.ImportRules
		case modeTest:
			rules = append(cfg.TestRules, cfg.ImportRules...)
		case modeXTest:
			rules = append(cfg.XTestRules, cfg.TestRules...)
			rules = append(rules, cfg.ImportRules...)
		}
		// Enforce each rule in order.
		for _, rule := range rules {
			switch result, err := enforceRule(rule, dep); {
			case err != nil:
				return nil, err
			case result == resultApproved:
				return nil, nil
			case result == resultRejected:
				err := fmt.Errorf(`violates %s deny rule %q in %s`, mode, rule.Pattern(), cfg.Path)
				return &violation{pkg, dep, err}, nil
			}
		}
	}
	if err := it.Err(); err != nil {
		return nil, err
	}
	// All config files have been checked without an approved or rejected result;
	// treat this as an approved result.  This also handles the case where no
	// config files have been specified.
	return nil, nil
}

func checkDeps(pkg *build.Package) ([]violation, error) {
	var violations []violation
	// First check direct dependencies against the Go 1.5 internal package rule.
	optsDirect := depOpts{DirectOnly: true, IncludeGoroot: true, IncludeTest: true, IncludeXTest: true}
	depsDirect := make(map[string]*build.Package)
	if err := optsDirect.Deps(pkg, depsDirect); err != nil {
		return nil, err
	}
	for _, dep := range sortPackages(depsDirect) {
		if !verifyGo15InternalRule(pkg.ImportPath, dep.ImportPath) {
			violations = append(violations, violation{pkg, dep, errGo15Internal})
		}
	}
	// Now check transitive dependencies against the rules in .godepcop files.
	// Each mode is checked independently, since the .godepcop configuration rules
	// may be different.
	for _, mode := range []checkMode{modeImport, modeTest, modeXTest} {
		opts := depOpts{IncludeGoroot: true}
		switch mode {
		case modeTest:
			opts.IncludeTest = true
		case modeXTest:
			opts.IncludeTest = true
			opts.IncludeXTest = true
		}
		deps := make(map[string]*build.Package)
		if err := opts.Deps(pkg, deps); err != nil {
			return nil, err
		}
		for _, dep := range sortPackages(deps) {
			v, err := checkDep(pkg, dep, mode)
			if err != nil {
				return nil, err
			}
			if v != nil {
				violations = append(violations, *v)
			}
		}
	}
	return violations, nil
}

type checkMode int

const (
	modeImport checkMode = iota
	modeTest
	modeXTest
)

func (mode checkMode) String() string {
	return []string{"import", "test", "xtest"}[mode]
}
