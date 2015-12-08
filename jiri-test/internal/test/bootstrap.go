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
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	s := jirix.NewSeq()

	// Create a new temporary JIRI_ROOT.
	oldRoot := os.Getenv("JIRI_ROOT")
	defer collect.Error(func() error { return os.Setenv("JIRI_ROOT", oldRoot) }, &e)
	tmpDir, err := s.TempDir("", "")
	if err != nil {
		return nil, newInternalError(err, "TempDir")
	}
	defer collect.Error(func() error { return jirix.NewSeq().RemoveAll(tmpDir).Done() }, &e)

	root := filepath.Join(tmpDir, "root")
	if err := os.Setenv("JIRI_ROOT", root); err != nil {
		return nil, newInternalError(err, "Setenv")
	}

	// Run the setup script.
	var out bytes.Buffer
	stdout := io.MultiWriter(jirix.Stdout(), &out)
	stderr := io.MultiWriter(jirix.Stderr(), &out)
	// Find the PATH element containing the "jiri" binary and remove it.
	jiriPath, err := exec.LookPath("jiri")
	if err != nil {
		return nil, newInternalError(err, "LookPath")
	}
	env := jirix.Env()
	env["PATH"] = strings.Replace(os.Getenv("PATH"), filepath.Dir(jiriPath), "", -1)
	env["JIRI_ROOT"] = root
	fn := func() error {
		return s.Env(env).Capture(stdout, stderr).Last(filepath.Join(oldRoot, "www", "public", "bootstrap"))
	}
	if err := retry.Function(jirix.Context, fn); err != nil {
		// Create xUnit report.
		if err := xunit.CreateFailureReport(jirix, testName, "VanadiumGo", "bootstrap", "Vanadium bootstrapping failed", out.String()); err != nil {
			return nil, err
		}
		return &test.Result{Status: test.Failed}, nil
	}
	return &test.Result{Status: test.Passed}, nil
}
