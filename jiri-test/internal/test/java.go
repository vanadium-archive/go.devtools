// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"path/filepath"

	"v.io/jiri/collect"
	"v.io/jiri/profiles"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/test"
	"v.io/x/lib/envvar"
)

// vanadiumJavaTest runs all Java tests.
func vanadiumJavaTest(ctx *tool.Context, testName string, opts ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(ctx, testName, []string{"java"})
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	ch, err := profiles.NewConfigHelper(ctx, profiles.UseProfiles, ManifestFilename)
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	target := profiles.NativeTarget()
	ch.MergeEnvFromProfiles(profiles.JiriMergePolicies(), target, "java")
	env := envvar.VarsFromOS()
	env.Set("JAVA_HOME", ch.Get("JAVA_HOME"))
	// Run tests.
	javaDir := filepath.Join(ch.Root(), "release", "java")
	if err := ctx.Run().Chdir(javaDir); err != nil {
		return nil, err
	}
	runOpts := ctx.Run().Opts()
	runOpts.Env = env.ToMap()
	if err := ctx.Run().CommandWithOpts(runOpts, filepath.Join(javaDir, "gradlew"), "--info", ":lib:test"); err != nil {
		return nil, err
	}
	// Run Gradle plugin tests.
	gradlePluginDir := filepath.Join(javaDir, "gradle-plugin")
	if err := ctx.Run().Chdir(gradlePluginDir); err != nil {
		return nil, err
	}
	if err := ctx.Run().CommandWithOpts(runOpts, filepath.Join(gradlePluginDir, "gradlew"), "--info", "test"); err != nil {
		return nil, err
	}
	return &test.Result{Status: test.Passed}, nil
}
