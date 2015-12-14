// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"path/filepath"
	"time"

	"v.io/jiri/jiri"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/xunit"
)

const (
	defaultProjectTestTimeout = 10 * time.Minute
)

func runMakefileTestWithNacl(jirix *jiri.X, testName, testDir, target string, env map[string]string, profiles []string, timeout time.Duration) (_ *test.Result, e error) {
	if err := installExtraDeps(jirix, testName, []string{"nacl"}, "amd64p32-nacl"); err != nil {
		return nil, err
	}
	return runMakefileTest(jirix, testName, testDir, target, env, profiles, timeout)
}

// vanadiumBrowserTest runs the tests for the Vanadium browser.
func vanadiumBrowserTest(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	env := map[string]string{
		"XUNIT_OUTPUT_FILE": xunit.ReportPath(testName),
	}
	testDir := filepath.Join(jirix.Root, "release", "projects", "browser")
	return runMakefileTestWithNacl(jirix, testName, testDir, "test", env, []string{"nodejs"}, defaultProjectTestTimeout)
}

// vanadiumBrowserTestWeb runs the ui tests for the Vanadium browser.
func vanadiumBrowserTestWeb(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "projects", "browser")
	return runMakefileTestWithNacl(jirix, testName, testDir, "test-ui", nil, []string{"nodejs"}, defaultProjectTestTimeout)
}

// vanadiumChatShellTest runs the tests for the chat shell client.
func vanadiumChatShellTest(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "projects", "chat")
	return runMakefileTest(jirix, testName, testDir, "test-shell", nil, nil, defaultProjectTestTimeout)
}

// vanadiumChatWebTest runs the tests for the chat web client.
func vanadiumChatWebTest(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "projects", "chat")
	return runMakefileTestWithNacl(jirix, testName, testDir, "test-web", nil, []string{"nodejs"}, defaultProjectTestTimeout)
}

// vanadiumChatWebUITest runs the ui tests for the chat web client.
func vanadiumChatWebUITest(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "projects", "chat")
	return runMakefileTestWithNacl(jirix, testName, testDir, "test-ui", nil, []string{"nodejs"}, defaultProjectTestTimeout)
}

// vanadiumPipe2BrowserTest runs the tests for pipe2browser.
func vanadiumPipe2BrowserTest(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "projects", "pipe2browser")
	return runMakefileTestWithNacl(jirix, testName, testDir, "test", nil, []string{"nodejs"}, defaultProjectTestTimeout)
}

// vanadiumCroupierTestUnit runs the unit tests for the croupier example application.
// Note: This test requires the "with_flutter" manifest, or a Flutter checkout in ${JIRI_ROOT}.
func vanadiumCroupierTestUnit(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "projects", "croupier")
	return runMakefileTest(jirix, testName, testDir, "test-unit", nil, []string{"dart"}, defaultProjectTestTimeout)
}

// vanadiumReaderTest runs the tests for the reader example application.
func vanadiumReaderTest(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "projects", "reader")
	return runMakefileTest(jirix, testName, testDir, "test", nil, []string{"nodejs"}, defaultProjectTestTimeout)
}

// vanadiumTravelTest runs the tests for the travel example application.
func vanadiumTravelTest(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "projects", "travel")
	return runMakefileTest(jirix, testName, testDir, "test", nil, []string{"nodejs"}, defaultProjectTestTimeout)
}
