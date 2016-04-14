// Copyright 2016 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"errors"
	"fmt"
	"os"

	"v.io/jiri"
	"v.io/jiri/collect"
	"v.io/x/devtools/internal/test"
)

const projectEnvVar = "VKUBE_TEST_PROJECT"

func vanadiumVkubeIntegrationTest(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	project := os.Getenv(projectEnvVar)
	if project == "" {
		return nil, newInternalError(fmt.Errorf("project not defined in %s environment variable", projectEnvVar), "Env")
	}
	s := jirix.NewSeq()
	if err := s.Last("kubectl", "cluster-info"); err != nil {
		return nil, newInternalError(errors.New("this test requires kubectl"), err.Error())
	}
	if err := s.Last("gsutil", "ls", "-p", project); err != nil {
		return nil, newInternalError(errors.New("this test requires gsutil"), err.Error())
	}
	if err := s.Last("docker", "info"); err != nil {
		return nil, newInternalError(errors.New("this test requires docker"), err.Error())
	}

	cleanup, err := initTest(jirix, testName, []string{"v23:base"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(cleanup, &e)

	args := []string{
		"go", "test", "-v=1", "-run=TestV23Vkube", "-timeout=30m",
		"v.io/x/ref/services/cluster/vkube",
		"--v23.tests", "--project=" + project, "--get-credentials=false",
	}
	if err := s.Capture(jirix.Stdout(), jirix.Stderr()).Last("jiri", args...); err != nil {
		return nil, newInternalError(err, err.Error())
	}

	return &test.Result{Status: test.Passed}, nil
}
