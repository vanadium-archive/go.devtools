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
	"v.io/jiri/profiles"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/xunit"
	"v.io/x/devtools/jiri-v23-profile/v23_profile"
)

// vanadiumGoVDL checks that all VDL-based Go source files are
// up-to-date.
func vanadiumGoVDL(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	fmt.Fprintf(ctx.Stdout(), "NOTE: This test checks that all VDL-based Go source files are up-to-date.\nIf it fails, you probably just need to run 'jiri run vdl generate --lang=go all'.\n")

	cleanup, err := initTest(ctx, testName, []string{"base"})
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Install the vdl tool.
	if err := ctx.Run().Command("jiri", "go", "install", "v.io/x/ref/cmd/vdl"); err != nil {
		return nil, internalTestError{err, "Install VDL"}
	}

	// Check that "vdl audit --lang=go all" produces no output.
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	ch, err := profiles.NewConfigHelper(ctx, v23_profile.DefaultManifestFilename)
	if err != nil {
		return nil, err
	}
	ch.SetGoPath()
	ch.SetVDLPath()
	opts.Env["VDLPATH"] = ch.Get("VDLPATH")
	vdl := filepath.Join(ch.Root(), "release", "go", "bin", "vdl")
	err = ctx.Run().CommandWithOpts(opts, vdl, "audit", "--lang=go", "all")
	output := strings.TrimSpace(out.String())
	if err != nil || len(output) != 0 {
		fmt.Fprintf(ctx.Stdout(), "%v\n", output)
		// Create xUnit report.
		files := strings.Split(output, "\n")
		suites := []xunit.TestSuite{}
		for _, file := range files {
			s := xunit.CreateTestSuiteWithFailure("VDLAudit", file, "VDL audit failure", "Outdated file:\n"+file, 0)
			suites = append(suites, *s)
		}
		if err := xunit.CreateReport(ctx, testName, suites); err != nil {
			return nil, err
		}
		return &test.Result{Status: test.Failed}, nil
	}
	return &test.Result{Status: test.Passed}, nil
}
