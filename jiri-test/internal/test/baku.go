// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"v.io/jiri/jiri"
	"v.io/x/devtools/internal/test"
)

// bakuAndroidBuild tests that the Baku Toolkit for Android Java builds.
func bakuAndroidBuild(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	return runJavaTest(jirix, testName, []string{"release", "java", "baku-toolkit"}, []string{":lib:clean", ":lib:install"})
}

// bakuJavaTest runs Baku Toolkit Java tests.
func bakuJavaTest(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	return runJavaTest(jirix, testName, []string{"release", "java", "baku-toolkit"}, []string{":lib:clean", ":lib:test"})
}
