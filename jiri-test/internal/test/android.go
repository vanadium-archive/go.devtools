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
	"v.io/x/devtools/jiri-v23-profile/v23_profile"
)

// vanadiumAndroidBuild tests that the android files build.
func vanadiumAndroidBuild(ctx *tool.Context, testName string, opts ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTestForTarget(ctx, testName, []string{"android"}, "android=arm-android")
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	ch, err := profiles.NewConfigHelper(ctx, v23_profile.DefaultManifestFilename)
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	// Run tests.
	javaDir := filepath.Join(ch.Root(), "release", "java")
	if err := ctx.Run().Chdir(javaDir); err != nil {
		return nil, err
	}
	if err := ctx.Run().Command(filepath.Join(javaDir, "gradlew"), "--info", ":android-lib:assemble"); err != nil {
		return nil, err
	}
	return &test.Result{Status: test.Passed}, nil
}