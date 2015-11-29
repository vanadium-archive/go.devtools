// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"v.io/jiri/collect"
	"v.io/jiri/gitutil"
	"v.io/jiri/jiri"
	"v.io/jiri/retry"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/test"
)

func vanadiumSignupProxy(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumSignupProxyHelper(jirix, "old_schema.go", testName)
}

func vanadiumSignupProxyNew(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumSignupProxyHelper(jirix, "new_schema.go", testName)
}

func vanadiumSignupProxyHelper(jirix *jiri.X, schema, testName string) (_ *test.Result, e error) {
	cleanup, err := initTest(jirix, testName, []string{"base"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Fetch emails addresses.
	credentials := os.Getenv("CREDENTIALS")
	sheetID := os.Getenv("SHEET_ID")

	data, err := fetchFieldValues(jirix, credentials, "email", schema, sheetID, false)
	if err != nil {
		return nil, newInternalError(err, "fetch")
	}

	// Create a feature branch in the infrastructure project.
	infraDir := tool.RootDirOpt(filepath.Join(jirix.Root, "infrastructure"))
	if err := jirix.Git(infraDir).CreateAndCheckoutBranch("update"); err != nil {
		return nil, newInternalError(err, "create")
	}
	defer collect.Error(func() error {
		if err := jirix.Git(infraDir).CheckoutBranch("master", gitutil.ForceOpt(true)); err != nil {
			return newInternalError(err, "checkout")
		}
		if err := jirix.Git(infraDir).DeleteBranch("update", gitutil.ForceOpt(true)); err != nil {
			return newInternalError(err, "delete")
		}
		return nil
	}, &e)

	// Update emails address whitelists.
	{
		whitelists := strings.Split(os.Getenv("WHITELISTS"), string(filepath.ListSeparator))
		mergeSrc := filepath.Join(jirix.Root, "infrastructure", "signup", "merge.go")
		for _, whitelist := range whitelists {
			opts := jirix.Run().Opts()
			opts.Stdin = bytes.NewReader(data)
			if err := jirix.Run().CommandWithOpts(opts, "jiri", "go", "run", mergeSrc, "-whitelist="+whitelist); err != nil {
				return nil, newInternalError(err, "merge")
			}
			if err := jirix.Git(infraDir).Add(whitelist); err != nil {
				return nil, newInternalError(err, "commit")
			}
		}
	}

	// Push changes (if any exist) to master.
	changed, err := jirix.Git(infraDir).HasUncommittedChanges()
	if err != nil {
		return nil, newInternalError(err, "changes")
	}
	if changed {
		if err := jirix.Git(infraDir).CommitWithMessage("updating list of emails"); err != nil {
			return nil, newInternalError(err, "commit")
		}
		if err := jirix.Git(infraDir).Push("origin", "update:master", gitutil.VerifyOpt(false)); err != nil {
			return nil, newInternalError(err, "push")
		}
	}

	return &test.Result{Status: test.Passed}, nil
}

func vanadiumSignupWelcomeStepOneNew(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	cleanup, err := initTest(jirix, testName, []string{"base"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	credentials := os.Getenv("CREDENTIALS")
	sheetID := os.Getenv("SHEET_ID")

	data, err := fetchFieldValues(jirix, credentials, "email", "new_schema.go", sheetID, false)
	if err != nil {
		return nil, newInternalError(err, "fetch")
	}

	var emails bytes.Buffer

	welcome := filepath.Join(jirix.Root, "infrastructure", "signup", "welcome.go")
	welcomeOpts := jirix.Run().Opts()
	welcomeOpts.Stdin = bytes.NewReader(data)
	welcomeOpts.Stdout = &emails
	sentlist := filepath.Join(jirix.Root, "infrastructure", "signup", "sentlist.json")
	if err := jirix.Run().CommandWithOpts(welcomeOpts, "jiri", "go", "run", welcome, "-sentlist="+sentlist); err != nil {
		return nil, newInternalError(err, "welcome")
	}

	// Convert the newline delimited output from the command above into a slice of
	// strings which can be written to a file in the format:
	//
	//   EMAILS = <email> <email> <email...>
	//
	output := []string{"EMAILS", "="}
	reader := bytes.NewReader(emails.Bytes())
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		email := scanner.Text()
		output = append(output, email)
	}

	if err := scanner.Err(); err != nil {
		return nil, newInternalError(err, "Scan")
	}

	// Join the array and convert it to bytes
	contents := strings.Join(output, " ")
	filename := filepath.Join(jirix.Root, ".vanadium_signup_weclome_properties")

	if err := jirix.Run().WriteFile(filename, []byte(contents), 0644); err != nil {
		return nil, newInternalError(err, "WriteFile")
	}

	// Create a feature branch in the infrastructure project.
	infraDir := tool.RootDirOpt(filepath.Join(jirix.Root, "infrastructure"))
	if err := jirix.Git(infraDir).CreateAndCheckoutBranch("update"); err != nil {
		return nil, newInternalError(err, "create")
	}
	defer collect.Error(func() error {
		if err := jirix.Git(infraDir).CheckoutBranch("master", gitutil.ForceOpt(true)); err != nil {
			return newInternalError(err, "checkout")
		}
		if err := jirix.Git(infraDir).DeleteBranch("update", gitutil.ForceOpt(true)); err != nil {
			return newInternalError(err, "delete")
		}
		return nil
	}, &e)

	if err := jirix.Git(infraDir).Add(sentlist); err != nil {
		return nil, newInternalError(err, "commit")
	}

	// Push changes (if any exist) to master.
	changed, err := jirix.Git(infraDir).HasUncommittedChanges()
	if err != nil {
		return nil, newInternalError(err, "changes")
	}
	if changed {
		if err := jirix.Git(infraDir).CommitWithMessage("infrastructure/signup: updating sentlist"); err != nil {
			return nil, newInternalError(err, "commit")
		}
		if err := jirix.Git(infraDir).Push("origin", "update:master", gitutil.VerifyOpt(false)); err != nil {
			return nil, newInternalError(err, "push")
		}
	}

	return &test.Result{Status: test.Passed}, nil
}

func vanadiumSignupWelcomeStepTwoNew(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	cleanup, err := initTest(jirix, testName, []string{"base"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	mailer := filepath.Join(jirix.Root, "release", "go", "src", "v.io", "x", "devtools", "mailer", "mailer.go")
	mailerFunc := func() error {
		return jirix.Run().Command("jiri", "go", "run", mailer)
	}
	if err := retry.Function(jirix.Context, mailerFunc); err != nil {
		return nil, newInternalError(err, "mailer")
	}

	return &test.Result{Status: test.Passed}, nil
}

func vanadiumSignupGithub(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumSignupGithubHelper(jirix, "old_schema.go", testName)
}

func vanadiumSignupGithubNew(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumSignupGithubHelper(jirix, "new_schema.go", testName)
}

func vanadiumSignupGithubHelper(jirix *jiri.X, schema, testName string) (_ *test.Result, e error) {
	cleanup, err := initTest(jirix, testName, []string{"base"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	credentials := os.Getenv("CREDENTIALS")
	sheetID := os.Getenv("SHEET_ID")

	data, err := fetchFieldValues(jirix, credentials, "github", schema, sheetID, false)
	if err != nil {
		return nil, newInternalError(err, "fetch")
	}

	// Add them to @vanadium/developers
	githubToken := os.Getenv("GITHUB_TOKEN")
	github := filepath.Join(jirix.Root, "infrastructure", "signup", "github.go")
	githubOpts := jirix.Run().Opts()
	githubOpts.Stdin = bytes.NewReader(data)
	if err := jirix.Run().CommandWithOpts(githubOpts, "jiri", "go", "run", github, "-token="+githubToken); err != nil {
		return nil, newInternalError(err, "github")
	}

	return &test.Result{Status: test.Passed}, nil
}

func vanadiumSignupGroup(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumSignupGroupHelper(jirix, "old_schema.go", testName, false)
}

func vanadiumSignupGroupNew(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumSignupGroupHelper(jirix, "new_schema.go", testName, false)
}

func vanadiumSignupDiscussNew(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumSignupGroupHelper(jirix, "new_schema.go", testName, true)
}

func vanadiumSignupGroupHelper(jirix *jiri.X, schema, testName string, discussOnly bool) (_ *test.Result, e error) {
	cleanup, err := initTest(jirix, testName, []string{"base"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Fetch emails addresses.
	credentials := os.Getenv("CREDENTIALS")
	sheetID := os.Getenv("SHEET_ID")

	data, err := fetchFieldValues(jirix, credentials, "email", schema, sheetID, discussOnly)
	if err != nil {
		return nil, newInternalError(err, "fetch")
	}

	// Add them to Google Group.
	groupEmail := os.Getenv("GROUP_EMAIL")
	opts := jirix.Run().Opts()
	opts.Stdin = bytes.NewReader(data)
	groupSrc := filepath.Join(jirix.Root, "infrastructure", "signup", "group.go")
	if err := jirix.Run().CommandWithOpts(opts, "jiri", "go", "run", groupSrc, "-credentials="+credentials, "-group-email="+groupEmail); err != nil {
		return nil, newInternalError(err, "group")
	}

	return &test.Result{Status: test.Passed}, nil
}

func fetchFieldValues(jirix *jiri.X, credentials, field, schema, sheetID string, discussOnly bool) ([]byte, error) {
	var buffer bytes.Buffer

	fetchSrc := filepath.Join(jirix.Root, "infrastructure", "signup", "fetch.go")
	schemaSrc := filepath.Join(jirix.Root, "infrastructure", "signup", schema)
	opts := jirix.Run().Opts()
	opts.Stdout = &buffer
	args := []string{"go", "run", fetchSrc, schemaSrc, "-credentials=" + credentials, "-field=" + field, "-sheet-id=" + sheetID}
	if discussOnly {
		args = append(args, "-discuss-only")
	}
	if err := jirix.Run().CommandWithOpts(opts, "jiri", args...); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
