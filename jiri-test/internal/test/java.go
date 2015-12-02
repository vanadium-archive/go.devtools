// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"path/filepath"

	"v.io/jiri/collect"
	"v.io/jiri/jiri"
	"v.io/jiri/profiles"
	"v.io/x/devtools/internal/test"
	"v.io/x/lib/envvar"
)

// runJavaTest includes common run logic for Java tests.
func runJavaTest(jirix *jiri.X, testName string, cwd []string, task string) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(jirix, testName, []string{"java"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	ch, err := profiles.NewConfigHelper(jirix, profiles.UseProfiles, ManifestFilename)
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	target := profiles.NativeTarget()
	ch.MergeEnvFromProfiles(profiles.JiriMergePolicies(), target, "java")
	env := envvar.VarsFromOS()
	env.Set("JAVA_HOME", ch.Get("JAVA_HOME"))
	// Run tests.
	javaDir := filepath.Join(append([]string{ch.Root()}, cwd...)...)
	if err := jirix.Run().Chdir(javaDir); err != nil {
		return nil, err
	}
	runOpts := jirix.Run().Opts()
	runOpts.Env = env.ToMap()
	if err := jirix.Run().CommandWithOpts(runOpts, filepath.Join(javaDir, "gradlew"), "--info", task); err != nil {
		return nil, err
	}
	return &test.Result{Status: test.Passed}, nil
}

// vanadiumJavaTest runs all Java tests.
func vanadiumJavaTest(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	return runJavaTest(jirix, testName, []string{"release", "java"}, ":lib:check")
}
