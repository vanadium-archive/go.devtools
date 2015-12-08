// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"v.io/jiri/collect"
	"v.io/jiri/jiri"
	"v.io/jiri/profiles"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/xunit"
)

// vanadiumGoVDL checks that all VDL-based Go source files are
// up-to-date.
func vanadiumGoVDL(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	fmt.Fprintf(jirix.Stdout(), "NOTE: This test checks that all VDL-based Go source files are up-to-date.\nIf it fails, you probably just need to run 'jiri run vdl generate --lang=go all'.\n")

	cleanup, err := initTest(jirix, testName, []string{"base"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	s := jirix.NewSeq()
	// Install the vdl tool.
	if err := s.Last("jiri", "go", "install", "v.io/x/ref/cmd/vdl"); err != nil {
		return nil, newInternalError(err, "Install VDL")
	}

	// Check that "vdl audit --lang=go all" produces no output.
	ch, err := profiles.NewConfigHelper(jirix, profiles.UseProfiles, ManifestFilename)
	if err != nil {
		return nil, err
	}
	ch.MergeEnvFromProfiles(profiles.JiriMergePolicies(), profiles.NativeTarget(), "jiri")
	env := ch.ToMap()
	env["VDLROOT"] = filepath.Join(ch.Root(), "release", "go", "src", "v.io", "v23", "vdlroot")
	vdl := filepath.Join(ch.Root(), "release", "go", "bin", "vdl")
	var out bytes.Buffer
	err = s.Env(env).Capture(&out, &out).Last(vdl, "audit", "--lang=go", "all")
	output := strings.TrimSpace(out.String())
	if err != nil || len(output) != 0 {
		fmt.Fprintf(jirix.Stdout(), "%v\n", output)
		// Create xUnit report.
		files := strings.Split(output, "\n")
		suites := []xunit.TestSuite{}
		for _, file := range files {
			s := xunit.CreateTestSuiteWithFailure("VDLAudit", file, "VDL audit failure", "Outdated file:\n"+file, 0)
			suites = append(suites, *s)
		}
		if err := xunit.CreateReport(jirix, testName, suites); err != nil {
			return nil, err
		}
		return &test.Result{Status: test.Failed}, nil
	}
	return &test.Result{Status: test.Passed}, nil
}
