// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"fmt"
	"path/filepath"
	"time"

	"v.io/jiri"
	"v.io/jiri/collect"
	"v.io/jiri/runutil"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/xunit"
	"v.io/x/lib/envvar"
)

const (
	defaultJSTestTimeout = 15 * time.Minute
)

// runJSTest is a harness for executing javascript tests.
func runJSTest(jirix *jiri.X, testName, testDir, target string, cleanFn func() error, env map[string]string) (_ *test.Result, e error) {
	// Initialize the test.
	deps := []string{"v23:base", "v23:nodejs"}
	cleanup, err := initTest(jirix, testName, deps)
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	s := jirix.NewSeq()

	// Set up the environment
	merged := envvar.MergeMaps(jirix.Env(), env)

	cleanCallFunc := func() error {
		if cleanFn != nil {
			return cleanFn()
		}
		return nil
	}

	// Navigate to the target directory and run make clean.
	err = s.Pushd(testDir).
		Env(merged).Run("make", "clean").
		Call(cleanCallFunc, "cleanFn: %p", cleanFn).
		Timeout(defaultJSTestTimeout).Env(merged).Last("make", target)
	if err != nil {
		if runutil.IsTimeout(err) {
			return &test.Result{
				Status:       test.TimedOut,
				TimeoutValue: defaultJSTestTimeout,
			}, nil
		} else {
			return nil, newInternalError(err, "Make "+target)
		}
	}

	return &test.Result{Status: test.Passed}, nil
}

func runJSTestWithNacl(jirix *jiri.X, testName, testDir, target string, cleanFn func() error, env map[string]string) (_ *test.Result, e error) {
	if err := installExtraDeps(jirix, testName, []string{"v23:nacl"}, "amd64p32-nacl"); err != nil {
		return nil, err
	}
	return runJSTest(jirix, testName, testDir, target, cleanFn, env)
}

func installExtraDeps(jirix *jiri.X, testName string, deps []string, target string) (e error) {
	cleanup2, err := initTestForTarget(jirix, testName, deps, target)
	if err != nil {
		return newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup2() }, &e)
	return nil
}

// vanadiumJSBuildExtension tests the vanadium javascript build extension.
func vanadiumJSBuildExtension(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "javascript", "core")
	target := "extension/vanadium.zip"
	return runJSTestWithNacl(jirix, testName, testDir, target, nil, nil)
}

// vanadiumJSDoc (re)generates the content of the vanadium core javascript
// documentation server.
func vanadiumJSDoc(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "javascript", "core")
	target := "docs"

	result, err := runJSTest(jirix, testName, testDir, target, nil, nil)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// vanadiumJSDocSyncbase (re)generates the content of the vanadium syncbase
// javascript documentation server.
func vanadiumJSDocSyncbase(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "javascript", "syncbase")
	result, err := runJSTest(jirix, testName, testDir, "docs", nil, nil)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// vanadiumJSDocDeploy (re)generates core jsdocs and deploys them to staging
// and production.
func vanadiumJSDocDeploy(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	return jsDocDeployHelper(jirix, testName, "core")
}

// vanadiumJSDocSyncbaseDeploy (re)generates syncbase jsdocs and deploys them to
// staging and production.
func vanadiumJSDocSyncbaseDeploy(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	return jsDocDeployHelper(jirix, testName, "syncbase")
}

func jsDocDeployHelper(jirix *jiri.X, testName, projectName string) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(jirix, testName, []string{"v23:nodejs"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	s := jirix.NewSeq()

	testDir := filepath.Join(jirix.Root, "release", "javascript", projectName)
	if err := s.Chdir(testDir).Done(); err != nil {
		return nil, err
	}

	for _, target := range []string{"deploy-docs-staging", "deploy-docs-production"} {
		if err := s.Timeout(defaultJSTestTimeout).Last("make", target); err != nil {
			if runutil.IsTimeout(err) {
				return &test.Result{
					Status:       test.TimedOut,
					TimeoutValue: defaultJSTestTimeout,
				}, nil
			} else {
				return nil, newInternalError(err, "Make "+target)
			}
		}
	}

	return &test.Result{Status: test.Passed}, nil
}

// vanadiumJSBrowserIntegration runs the vanadium javascript integration test in a browser environment using nacl plugin.
func vanadiumJSBrowserIntegration(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "javascript", "core")
	target := "test-integration-browser"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["BROWSER_OUTPUT"] = xunit.ReportPath(testName)
	res, err := runJSTestWithNacl(jirix, testName, testDir, target, nil, env)
	// TODO(nlacasse): This test is occasionally timing out on Jenkins with no
	// output after prova launches.  In the event of a timeout, the following
	// lines will print chrome's log, which will hopefully give us some useful
	// info.  Remove this line once the timeout has been fixed.
	// See https://github.com/vanadium/issues/issues/1182
	if res != nil && res.Status == test.TimedOut {
		if err := jirix.NewSeq().Last("cat", filepath.Join(jirix.Root, "release", "javascript", "core", "tmp", "chrome.log")); err != nil {
			fmt.Printf("error catting chrome.log: %v\n", err)
		}
	}
	return res, err
}

// vanadiumJSNodeIntegration runs the vanadium javascript integration test in NodeJS environment using wspr.
func vanadiumJSNodeIntegration(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "javascript", "core")
	target := "test-integration-node"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["NODE_OUTPUT"] = xunit.ReportPath(testName)
	return runJSTest(jirix, testName, testDir, target, nil, env)
}

// vanadiumJSUnit runs the vanadium javascript unit test.
func vanadiumJSUnit(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "javascript", "core")
	target := "test-unit"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["NODE_OUTPUT"] = xunit.ReportPath(testName)
	return runJSTestWithNacl(jirix, testName, testDir, target, nil, env)
}

// vanadiumJSVdl runs the vanadium javascript vdl test.
func vanadiumJSVdl(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "javascript", "core")
	target := "test-vdl"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["NODE_OUTPUT"] = xunit.ReportPath(testName)
	return runJSTestWithNacl(jirix, testName, testDir, target, nil, env)
}

// vanadiumJSVDLAudit checks that all VDL-based JS source files are up-to-date.
func vanadiumJSVdlAudit(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	testDir := filepath.Join(jirix.Root, "release", "javascript", "core")
	target := "test-vdl-audit"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["NODE_OUTPUT"] = xunit.ReportPath(testName)
	return runJSTest(jirix, testName, testDir, target, nil, env)
}

// vanadiumJSVom runs the vanadium javascript vom test.
func vanadiumJSVom(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "javascript", "core")
	target := "test-vom"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["NODE_OUTPUT"] = xunit.ReportPath(testName)
	return runJSTestWithNacl(jirix, testName, testDir, target, nil, env)
}

// vanadiumJSSyncbaseBrowser runs the vanadium javascript syncbase test in a browser.
func vanadiumJSSyncbaseBrowser(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "javascript", "syncbase")
	target := "test-integration-browser"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["BROWSER_OUTPUT"] = xunit.ReportPath(testName)
	return runJSTestWithNacl(jirix, testName, testDir, target, nil, env)
}

// vanadiumJSSyncbaseNode runs the vanadium javascript syncbase test in nodejs.
func vanadiumJSSyncbaseNode(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	testDir := filepath.Join(jirix.Root, "release", "javascript", "syncbase")
	target := "test-integration-node"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["NODE_OUTPUT"] = xunit.ReportPath(testName)
	return runJSTest(jirix, testName, testDir, target, nil, env)
}

func setCommonJSEnv(env map[string]string) {
	env["XUNIT"] = "true"
}
