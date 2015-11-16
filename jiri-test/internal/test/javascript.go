// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"path/filepath"
	"time"

	"v.io/jiri/collect"
	"v.io/jiri/project"
	"v.io/jiri/runutil"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/xunit"
)

const (
	defaultJSTestTimeout = 15 * time.Minute
)

// runJSTest is a harness for executing javascript tests.
func runJSTest(ctx *tool.Context, testName, testDir, target string, cleanFn func() error, env map[string]string) (_ *test.Result, e error) {
	// Initialize the test.
	deps := []string{"base", "nodejs"}
	cleanup, err := initTest(ctx, testName, deps)
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Navigate to the target directory.
	if err := ctx.Run().Chdir(testDir); err != nil {
		return nil, err
	}

	// Set up the environment
	opts := ctx.Run().Opts()
	for key, value := range env {
		opts.Env[key] = value
	}

	// Clean up after previous instances of the test.
	if err := ctx.Run().CommandWithOpts(opts, "make", "clean"); err != nil {
		return nil, err
	}
	if cleanFn != nil {
		if err := cleanFn(); err != nil {
			return nil, err
		}
	}

	// Run the test target.
	if err := ctx.Run().TimedCommandWithOpts(defaultJSTestTimeout, opts, "make", target); err != nil {
		if err == runutil.CommandTimedOutErr {
			return &test.Result{
				Status:       test.TimedOut,
				TimeoutValue: defaultJSTestTimeout,
			}, nil
		} else {
			return nil, internalTestError{err, "Make " + target}
		}
	}

	return &test.Result{Status: test.Passed}, nil
}

func runJSTestWithNacl(ctx *tool.Context, testName, testDir, target string, cleanFn func() error, env map[string]string) (_ *test.Result, e error) {
	if err := installExtraDeps(ctx, testName, []string{"nacl"}, "amd64p32-nacl"); err != nil {
		return nil, err
	}
	return runJSTest(ctx, testName, testDir, target, cleanFn, env)
}

func installExtraDeps(ctx *tool.Context, testName string, deps []string, target string) (e error) {
	cleanup2, err := initTestForTarget(ctx, testName, deps, target)
	if err != nil {
		return internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup2() }, &e)
	return nil
}

// vanadiumJSBuildExtension tests the vanadium javascript build extension.
func vanadiumJSBuildExtension(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	root, err := project.JiriRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "core")
	target := "extension/vanadium.zip"
	return runJSTestWithNacl(ctx, testName, testDir, target, nil, nil)
}

// vanadiumJSDoc (re)generates the content of the vanadium core javascript
// documentation server.
func vanadiumJSDoc(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	root, err := project.JiriRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "core")
	target := "docs"

	result, err := runJSTest(ctx, testName, testDir, target, nil, nil)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// vanadiumJSDocSyncbase (re)generates the content of the vanadium syncbase
// javascript documentation server.
func vanadiumJSDocSyncbase(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	root, err := project.JiriRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "syncbase")
	result, err := runJSTest(ctx, testName, testDir, "docs", nil, nil)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// vanadiumJSDocDeploy (re)generates core jsdocs and deploys them to staging
// and production.
func vanadiumJSDocDeploy(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	return jsDocDeployHelper(ctx, testName, "core")
}

// vanadiumJSDocSyncbaseDeploy (re)generates syncbase jsdocs and deploys them to
// staging and production.
func vanadiumJSDocSyncbaseDeploy(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	return jsDocDeployHelper(ctx, testName, "syncbase")
}

func jsDocDeployHelper(ctx *tool.Context, testName, projectName string) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(ctx, testName, []string{"nodejs"})
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	root, err := project.JiriRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", projectName)
	if err := ctx.Run().Chdir(testDir); err != nil {
		return nil, err
	}

	for _, target := range []string{"deploy-docs-staging", "deploy-docs-production"} {
		if err := ctx.Run().TimedCommand(defaultJSTestTimeout, "make", target); err != nil {
			if err == runutil.CommandTimedOutErr {
				return &test.Result{
					Status:       test.TimedOut,
					TimeoutValue: defaultJSTestTimeout,
				}, nil
			} else {
				return nil, internalTestError{err, "Make " + target}
			}
		}
	}

	return &test.Result{Status: test.Passed}, nil
}

// vanadiumJSBrowserIntegration runs the vanadium javascript integration test in a browser environment using nacl plugin.
func vanadiumJSBrowserIntegration(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	root, err := project.JiriRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "core")
	target := "test-integration-browser"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["BROWSER_OUTPUT"] = xunit.ReportPath(testName)
	return runJSTestWithNacl(ctx, testName, testDir, target, nil, env)
}

// vanadiumJSNodeIntegration runs the vanadium javascript integration test in NodeJS environment using wspr.
func vanadiumJSNodeIntegration(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	root, err := project.JiriRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "core")
	target := "test-integration-node"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["NODE_OUTPUT"] = xunit.ReportPath(testName)
	return runJSTest(ctx, testName, testDir, target, nil, env)
}

// vanadiumJSUnit runs the vanadium javascript unit test.
func vanadiumJSUnit(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	root, err := project.JiriRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "core")
	target := "test-unit"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["NODE_OUTPUT"] = xunit.ReportPath(testName)
	return runJSTestWithNacl(ctx, testName, testDir, target, nil, env)
}

// vanadiumJSVdl runs the vanadium javascript vdl test.
func vanadiumJSVdl(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	root, err := project.JiriRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "core")
	target := "test-vdl"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["NODE_OUTPUT"] = xunit.ReportPath(testName)
	return runJSTestWithNacl(ctx, testName, testDir, target, nil, env)
}

// vanadiumJSVDLAudit checks that all VDL-based JS source files are up-to-date.
func vanadiumJSVdlAudit(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	root, err := project.JiriRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "core")
	target := "test-vdl-audit"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["NODE_OUTPUT"] = xunit.ReportPath(testName)
	return runJSTest(ctx, testName, testDir, target, nil, env)
}

// vanadiumJSVom runs the vanadium javascript vom test.
func vanadiumJSVom(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	root, err := project.JiriRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "core")
	target := "test-vom"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["NODE_OUTPUT"] = xunit.ReportPath(testName)
	return runJSTestWithNacl(ctx, testName, testDir, target, nil, env)
}

// vanadiumJSSyncbaseBrowser runs the vanadium javascript syncbase test in a browser.
func vanadiumJSSyncbaseBrowser(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	root, err := project.JiriRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "syncbase")
	target := "test-integration-browser"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["BROWSER_OUTPUT"] = xunit.ReportPath(testName)
	return runJSTestWithNacl(ctx, testName, testDir, target, nil, env)
}

// vanadiumJSSyncbaseNode runs the vanadium javascript syncbase test in nodejs.
func vanadiumJSSyncbaseNode(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	root, err := project.JiriRoot()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "syncbase")
	target := "test-integration-node"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["NODE_OUTPUT"] = xunit.ReportPath(testName)
	return runJSTest(ctx, testName, testDir, target, nil, env)
}

func setCommonJSEnv(env map[string]string) {
	env["XUNIT"] = "true"
}
