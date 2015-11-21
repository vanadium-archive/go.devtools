// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"path/filepath"
	"strings"
	"time"

	"v.io/jiri/collect"
	"v.io/jiri/jiri"
	"v.io/jiri/project"
	"v.io/jiri/runutil"
	"v.io/x/devtools/internal/test"
)

const (
	defaultWWWTestTimeout = 10 * time.Minute
)

// Runs specified make target in WWW Makefile as a test.
func commonVanadiumWWW(jirix *jiri.X, testName, makeTarget string, timeout time.Duration, extraDeps []string) (_ *test.Result, e error) {
	root, err := project.JiriRoot()
	if err != nil {
		return nil, err
	}

	// Initialize the test.
	cleanup, err := initTest(jirix, testName, append([]string{"base", "nodejs"}, extraDeps...))
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	cleanup2, err := initTestForTarget(jirix, testName, []string{"nacl"}, "amd64p32-nacl")
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup2() }, &e)

	wwwDir := filepath.Join(root, "www")
	if err := jirix.Run().Chdir(wwwDir); err != nil {
		return nil, err
	}

	if err := jirix.Run().Command("make", "clean"); err != nil {
		return nil, err
	}

	// Invoke the make target.
	if err := jirix.Run().TimedCommand(timeout, "make", makeTarget); err != nil {
		if err == runutil.CommandTimedOutErr {
			return &test.Result{
				Status:       test.TimedOut,
				TimeoutValue: timeout,
			}, nil
		} else {
			return nil, internalTestError{err, "Make " + makeTarget}
		}
	}

	return &test.Result{Status: test.Passed}, nil
}

func vanadiumWWWSite(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	return commonVanadiumWWW(jirix, testName, "test-site", defaultWWWTestTimeout, nil)
}

func vanadiumWWWTutorialsCore(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	return commonVanadiumWWW(jirix, testName, "test-tutorials-core", defaultWWWTestTimeout, nil)
}

func vanadiumWWWTutorialsExternal(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	return commonVanadiumWWW(jirix, testName, "test-tutorials-external", defaultWWWTestTimeout, nil)
}

func vanadiumWWWTutorialsJava(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	return commonVanadiumWWW(jirix, testName, "test-tutorials-java", defaultWWWTestTimeout, []string{"java"})
}

func vanadiumWWWTutorialsJSNode(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	return commonVanadiumWWW(jirix, testName, "test-tutorials-js-node", defaultWWWTestTimeout, nil)
}

func vanadiumWWWTutorialsJSWeb(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	return commonVanadiumWWW(jirix, testName, "test-tutorials-js-web", defaultWWWTestTimeout, nil)
}

func vanadiumWWWDeployStaging(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	return commonVanadiumWWW(jirix, testName, "deploy-staging", defaultWWWTestTimeout, nil)
}

func vanadiumWWWDeployProduction(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	return commonVanadiumWWW(jirix, testName, "deploy-production", defaultWWWTestTimeout, nil)
}

// vanadiumWWWConfigDeployHelper updates remote instance configuration and restarts remote nginx, auth, and proxy services.
func vanadiumWWWConfigDeployHelper(jirix *jiri.X, testName string, env string, _ ...Opt) (_ *test.Result, e error) {
	cleanup, err := initTest(jirix, testName, nil)
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Change dir to infrastructure/nginx.
	root, err := project.JiriRoot()
	if err != nil {
		return nil, internalTestError{err, "JiriRoot"}
	}

	dir := filepath.Join(root, "infrastructure", "nginx")
	if err := jirix.Run().Chdir(dir); err != nil {
		return nil, internalTestError{err, "Chdir"}
	}

	// Update configuration.
	target := strings.Join([]string{"deploy", env}, "-")
	if err := jirix.Run().Command("make", target); err != nil {
		return &test.Result{Status: test.Failed}, nil
	}

	// Restart remote services.
	project := strings.Join([]string{"vanadium", env}, "-")
	if err := jirix.Run().Command("./restart.sh", project); err != nil {
		return &test.Result{Status: test.Failed}, nil
	}

	return &test.Result{Status: test.Passed}, nil
}

func vanadiumWWWConfigDeployProduction(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumWWWConfigDeployHelper(jirix, testName, "production")
}
func vanadiumWWWConfigDeployStaging(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumWWWConfigDeployHelper(jirix, testName, "staging")
}
