// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"v.io/jiri/collect"
	"v.io/jiri/project"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/test"
)

var (
	jenkinsHost = "http://localhost:8001/jenkins"
)

// requireEnv makes sure that the given environment variables are set.
func requireEnv(names []string) error {
	for _, name := range names {
		if os.Getenv(name) == "" {
			return fmt.Errorf("environment variable %q is not set", name)
		}
	}
	return nil
}

// vanadiumPresubmitPoll polls vanadium projects for new patchsets for
// which to run presubmit tests.
func vanadiumPresubmitPoll(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	root, err := project.JiriRoot()
	if err != nil {
		return nil, err
	}

	// Initialize the test.
	cleanup, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Use the "presubmit query" command to poll for new changes.
	logfile := filepath.Join(root, ".presubmit_log")
	args := []string{}
	if ctx.Verbose() {
		args = append(args, "-v")
	} else {
		// append this for testing this CL only - remove on checkin.
		args = append(args, "-v")
	}
	args = append(args,
		"-host", jenkinsHost,
		"query",
		"-log-file", logfile,
		"-manifest", "tools",
	)
	if err := ctx.Run().Command("presubmit", args...); err != nil {
		return nil, err
	}

	return &test.Result{Status: test.Passed}, nil
}

func removeProfiles(ctx *tool.Context) {
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out

	removals := []string{}
	fmt.Fprintf(ctx.Stdout(), "presubmit: removeProfiles: %s\n", removals)
	cmds := append([]string{"list"}, removals...)
	cmds = append(cmds, "list")
	for _, args := range cmds {
		clargs := append([]string{"v23-profile"}, strings.Split(args, " ")...)
		err := ctx.Run().CommandWithOpts(opts, "jiri", clargs...)
		fmt.Fprintf(ctx.Stdout(), "jiri %v: %v [[\n", strings.Join(clargs, " "), err)
		fmt.Fprintf(ctx.Stdout(), "%s]]\n", out.String())
		out.Reset()
	}
}

func displayProfiles(ctx *tool.Context, msg string) {
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	fmt.Fprintf(ctx.Stdout(), "%s: installed profiles:\n", msg)
	err := ctx.Run().CommandWithOpts(opts, "jiri", "v23-profile", "list", "--v")
	if err != nil {
		fmt.Fprintf(ctx.Stdout(), " %v\n", err)
		return
	}
	fmt.Fprintf(ctx.Stdout(), "\n%s\n", out.String())
	out.Reset()
	fmt.Fprintf(ctx.Stdout(), "recreate profiles with:\n")
	err = ctx.Run().CommandWithOpts(opts, "jiri", "v23-profile", "recreate")
	if err != nil {
		fmt.Fprintf(ctx.Stdout(), " %v\n", err)
		return
	}
	fmt.Fprintf(ctx.Stdout(), "\n%s\n", out.String())
}

// vanadiumPresubmitTest runs presubmit tests for a given project specified
// in TEST environment variable.
func vanadiumPresubmitTest(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	if err := requireEnv([]string{"BUILD_NUMBER", "REFS", "PROJECTS", "TEST", "WORKSPACE"}); err != nil {
		return nil, err
	}

	// Initialize the test.
	cleanup, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	if isCI() {
		removeProfiles(ctx)
		displayProfiles(ctx, "presubmit")
	}

	// Use the "presubmit test" command to run the presubmit test.
	args := []string{}
	if ctx.Verbose() {
		args = append(args, "-v")
	}
	name := os.Getenv("TEST")
	args = append(args,
		"-host", jenkinsHost,
		"test",
		"-build-number", os.Getenv("BUILD_NUMBER"),
		"-manifest", "tools",
		"-projects", os.Getenv("PROJECTS"),
		"-refs", os.Getenv("REFS"),
		"-test", name,
	)
	if err := ctx.Run().Command("presubmit", args...); err != nil {
		return nil, internalTestError{err, "Presubmit"}
	}

	// Remove any test result files that are empty.
	testResultFiles, err := findTestResultFiles(ctx, name)
	if err != nil {
		return nil, err
	}
	for _, file := range testResultFiles {
		fileInfo, err := ctx.Run().Stat(file)
		if err != nil {
			return nil, err
		}
		if fileInfo.Size() == 0 {
			if err := ctx.Run().RemoveAll(file); err != nil {
				return nil, err
			}
		}
	}

	return &test.Result{Status: test.Passed}, nil
}

// vanadiumPresubmitResult runs "presubmit result" command to process and post test results.
func vanadiumPresubmitResult(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	if err := requireEnv([]string{"BUILD_NUMBER", "REFS", "PROJECTS", "WORKSPACE"}); err != nil {
		return nil, err
	}

	// Initialize the test.
	cleanup, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Run "presubmit result".
	args := []string{}
	if ctx.Verbose() {
		args = append(args, "-v")
	}
	args = append(args,
		"-host", jenkinsHost,
		"result",
		"-build-number", os.Getenv("BUILD_NUMBER"),
		"-manifest", "tools",
		"-refs", os.Getenv("REFS"),
		"-projects", os.Getenv("PROJECTS"),
	)
	if err := ctx.Run().Command("presubmit", args...); err != nil {
		return nil, err
	}

	return &test.Result{Status: test.Passed}, nil
}
