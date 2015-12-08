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
	"v.io/jiri/runutil"
	"v.io/x/devtools/internal/test"
)

const (
	defaultWWWTestTimeout = 10 * time.Minute
)

// Runs specified make target in WWW Makefile as a test.
func commonVanadiumWWW(jirix *jiri.X, testName, makeTarget string, timeout time.Duration, extraDeps []string) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(jirix, testName, append([]string{"base", "nodejs"}, extraDeps...))
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	cleanup2, err := initTestForTarget(jirix, testName, []string{"nacl"}, "amd64p32-nacl")
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup2() }, &e)

	s := jirix.NewSeq()
	wwwDir := filepath.Join(jirix.Root, "www")
	if err := s.Chdir(wwwDir).
		Run("make", "clean").
		Timeout(timeout).Last("make", makeTarget); err != nil {
		if runutil.IsTimeout(err) {
			return &test.Result{
				Status:       test.TimedOut,
				TimeoutValue: timeout,
			}, nil
		} else {
			return nil, newInternalError(err, "Make "+makeTarget)
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
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	dir := filepath.Join(jirix.Root, "infrastructure", "nginx")
	target := strings.Join([]string{"deploy", env}, "-")
	project := strings.Join([]string{"vanadium", env}, "-")
	if err := jirix.NewSeq().Chdir(dir).
		Run("make", target).
		Last("./restart.sh", project); err != nil {
		return &test.Result{Status: test.Failed}, err
	}
	return &test.Result{Status: test.Passed}, nil
}

func vanadiumWWWConfigDeployProduction(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumWWWConfigDeployHelper(jirix, testName, "production")
}
func vanadiumWWWConfigDeployStaging(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumWWWConfigDeployHelper(jirix, testName, "staging")
}
