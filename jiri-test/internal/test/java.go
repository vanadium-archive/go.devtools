// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"path/filepath"

	"v.io/jiri"
	"v.io/jiri/collect"
	"v.io/jiri/profiles"
	"v.io/jiri/profiles/profilesreader"
	"v.io/x/devtools/internal/test"
	"v.io/x/lib/envvar"
)

// runJavaTest includes common run logic for Java tests.
func runJavaTest(jirix *jiri.X, testName string, cwd []string, tasks []string) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(jirix, testName, []string{"v23:java"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	rd, err := profilesreader.NewReader(jirix, profilesreader.UseProfiles, ProfilesDBFilename)
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	target := profiles.NativeTarget()
	rd.MergeEnvFromProfiles(profilesreader.JiriMergePolicies(), target, "java")
	env := envvar.VarsFromOS()
	env.Set("JAVA_HOME", rd.Get("JAVA_HOME"))
	// Run tests.
	javaDir := filepath.Join(append([]string{jirix.Root}, cwd...)...)
	args := []string{"--info"}
	args = append(args, tasks...)
	if err := jirix.NewSeq().
		Pushd(javaDir).
		Env(env.ToMap()).
		Last(filepath.Join(javaDir, "gradlew"), args...); err != nil {
		return nil, err
	}
	return &test.Result{Status: test.Passed}, nil
}

// vanadiumJavaTest runs all Java tests.
func vanadiumJavaTest(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	return runJavaTest(jirix, testName, []string{"release", "java"}, []string{":lib:clean", ":lib:check"})
}

// vanadiumJavaSyncbaseTest runs all Java Syncbase high-level API unit tests.
func vanadiumJavaSyncbaseTest(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	return runJavaTest(jirix, testName, []string{"release", "java", "syncbase"}, []string{"clean", "test"})
}
