// Copyright 2016 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"v.io/jiri"
	"v.io/x/devtools/internal/test"
)

// madbGoTest runs Go tests for the madb project.
func madbGoTest(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	pkgs := []string{"madb/..."}
	opts = append(opts, DefaultPkgsOpt(pkgs))
	return vanadiumGoTest(jirix, testName, opts...)
}

// madbGoFormat runs Go format check for the madb project.
func madbGoFormat(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	pkgs := []string{"madb/..."}
	opts = append(opts, DefaultPkgsOpt(pkgs))
	return vanadiumGoFormat(jirix, testName, opts...)
}

// madbGoGenerate checks that files created by 'go generate' are
// up-to-date.
func madbGoGenerate(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	pkgs := []string{"madb/..."}
	opts = append(opts, DefaultPkgsOpt(pkgs))
	return vanadiumGoGenerate(jirix, testName, opts...)
}
