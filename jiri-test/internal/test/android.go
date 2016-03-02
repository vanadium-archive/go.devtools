// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"v.io/jiri"
	"v.io/x/devtools/internal/test"
)

// vanadiumAndroidBuild tests that the android files build.
func vanadiumAndroidBuild(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	return runJavaTest(jirix, testName, []string{"release", "java"}, []string{":android-lib:clean", ":android-lib:assemble"})
}

func vanadiumMomentsTest(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	return runJavaTest(jirix, testName, []string{"release", "java", "projects", "moments"}, []string{"assembleDebug", "test"})
}
