package testutil

import (
	"v.io/tools/lib/collect"
	"v.io/tools/lib/util"
)

// vanadiumPostsubmitPoll polls for new changes in all projects' master branches,
// and starts the corresponding Jenkins targets based on the changes.
func vanadiumPostsubmitPoll(ctx *util.Context, testName string) (_ *TestResult, e error) {
	// Initialize the test.
	cleanup, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Run the "postsubmit poll" command.
	args := []string{}
	if ctx.Verbose() {
		args = append(args, "-v")
	}
	args = append(args,
		"-host", jenkinsHost,
		"-token", jenkinsToken,
		"poll",
		"-manifest", "all-mirror",
	)
	if err := ctx.Run().Command("postsubmit", args...); err != nil {
		return nil, err
	}

	return &TestResult{Status: TestPassed}, nil
}
