// Copyright 2016 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"v.io/jiri"
	"v.io/jiri/collect"
	"v.io/x/devtools/internal/test"
)

const (
	projectEnvVar       = "VKUBE_TEST_PROJECT"
	bucketNamePrefix    = "gs://vkube-test-"
	namespaceNamePrefix = "namespace/vkube-test-"
	timeFormat          = "20060102-150405"
)

func vanadiumVkubeIntegrationTest(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	project := os.Getenv(projectEnvVar)
	if project == "" {
		return nil, newInternalError(fmt.Errorf("project not defined in %s environment variable", projectEnvVar), "Env")
	}
	s := jirix.NewSeq()
	if err := s.Last("docker", "info"); err != nil {
		return nil, newInternalError(errors.New("this test requires docker"), err.Error())
	}
	if err := cleanUpBuckets(jirix, project); err != nil {
		return nil, newInternalError(errors.New("failed to clean up old buckets"), err.Error())
	}
	if err := cleanUpNamespaces(jirix); err != nil {
		return nil, newInternalError(errors.New("failed to clean up old namespaces"), err.Error())
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

func cleanUpBuckets(jirix *jiri.X, project string) error {
	s := jirix.NewSeq()
	var output bytes.Buffer
	if err := s.Capture(&output, nil).Last("gsutil", "ls", "-p", project); err != nil {
		return err
	}
	for _, b := range strings.Split(output.String(), "\n") {
		if !strings.HasPrefix(b, bucketNamePrefix) {
			continue
		}
		ts := strings.TrimPrefix(b, bucketNamePrefix)
		if len(ts) < len(timeFormat) {
			fmt.Fprintf(jirix.Stderr(), "failed to parse timestamp %s\n", ts)
			continue
		}
		ts = ts[:len(timeFormat)]
		t, err := time.Parse(timeFormat, ts)
		if err != nil {
			fmt.Fprintf(jirix.Stderr(), "failed to parse timestamp in %s\n", b)
			continue
		}
		if time.Since(t) > 2*time.Hour {
			fmt.Fprintf(jirix.Stdout(), "Deleting old bucket %q\n", b)
			if err := s.Last("gsutil", "-m", "rm", "-r", b); err != nil {
				fmt.Fprintf(jirix.Stderr(), "failed to delete bucket %q: %v\n", b, err)
			}
		}
	}
	return nil
}

func cleanUpNamespaces(jirix *jiri.X) error {
	s := jirix.NewSeq()
	var output bytes.Buffer
	if err := s.Capture(&output, nil).Last("kubectl", "get", "namespace", "-o", "name"); err != nil {
		return err
	}
	for _, ns := range strings.Split(output.String(), "\n") {
		if !strings.HasPrefix(ns, namespaceNamePrefix) {
			continue
		}
		ts := strings.TrimPrefix(ns, namespaceNamePrefix)
		if len(ts) < len(timeFormat) {
			fmt.Fprintf(jirix.Stderr(), "failed to parse timestamp %s\n", ts)
			continue
		}
		ts = ts[:len(timeFormat)]
		t, err := time.Parse(timeFormat, ts)
		if err != nil {
			fmt.Fprintf(jirix.Stderr(), "failed to parse timestamp in %s\n", ns)
			continue
		}
		if time.Since(t) > 2*time.Hour {
			fmt.Fprintf(jirix.Stdout(), "Deleting old namespace %q\n", ns)
			if err := s.Last("kubectl", "delete", ns); err != nil {
				fmt.Fprintf(jirix.Stderr(), "failed to delete %q: %v\n", ns, err)
			}
		}
	}
	return nil
}
