// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"v.io/jiri/collect"
	"v.io/jiri/jiri"
	"v.io/jiri/retry"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/xunit"
)

// vanadiumBootstrap runs a test of Vanadium bootstrapping.
func vanadiumBootstrap(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(jirix, testName, nil)
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Create a new temporary JIRI_ROOT.
	oldRoot := os.Getenv("JIRI_ROOT")
	defer collect.Error(func() error { return os.Setenv("JIRI_ROOT", oldRoot) }, &e)
	tmpDir, err := jirix.Run().TempDir("", "")
	if err != nil {
		return nil, internalTestError{err, "TempDir"}
	}
	defer collect.Error(func() error { return jirix.Run().RemoveAll(tmpDir) }, &e)

	root := filepath.Join(tmpDir, "root")
	if err := os.Setenv("JIRI_ROOT", root); err != nil {
		return nil, internalTestError{err, "Setenv"}
	}

	// Run the setup script.
	var out bytes.Buffer
	opts := jirix.Run().Opts()
	opts.Stdout = io.MultiWriter(opts.Stdout, &out)
	opts.Stderr = io.MultiWriter(opts.Stderr, &out)
	// Find the PATH element containing the "jiri" binary and remove it.
	jiriPath, err := exec.LookPath("jiri")
	if err != nil {
		return nil, internalTestError{err, "LookPath"}
	}
	opts.Env["PATH"] = strings.Replace(os.Getenv("PATH"), filepath.Dir(jiriPath), "", -1)
	opts.Env["JIRI_ROOT"] = root
	fn := func() error {
		return jirix.Run().CommandWithOpts(opts, filepath.Join(oldRoot, "www", "public", "bootstrap"))
	}
	if err := retry.Function(jirix.Context, fn); err != nil {
		// Create xUnit report.
		if err := xunit.CreateFailureReport(jirix.Context, testName, "VanadiumGo", "bootstrap", "Vanadium bootstrapping failed", out.String()); err != nil {
			return nil, err
		}
		return &test.Result{Status: test.Failed}, nil
	}
	return &test.Result{Status: test.Passed}, nil
}
