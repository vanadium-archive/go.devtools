// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"v.io/jiri/collect"
	"v.io/jiri/jiri"
	"v.io/x/devtools/internal/test"
)

// vanadiumPostsubmitPoll polls for new changes in all projects' master branches,
// and starts the corresponding Jenkins targets based on the changes.
func vanadiumPostsubmitPoll(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTestImpl(jirix, false, false, testName, []string{"base"}, "")
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Run the "postsubmit poll" command.
	args := []string{}
	if jirix.Verbose() {
		args = append(args, "-v")
	}
	args = append(args,
		"-host", jenkinsHost,
		"poll",
		"-manifest", "mirror/tools",
	)
	if err := jirix.Run().Command("postsubmit", args...); err != nil {
		return nil, err
	}

	return &test.Result{Status: test.Passed}, nil
}
