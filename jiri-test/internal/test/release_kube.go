// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"fmt"
	"os"
	"path/filepath"

	"v.io/jiri"
	"v.io/jiri/collect"
	"v.io/x/devtools/internal/test"
)

func vanadiumReleaseKubeStaging(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	version := os.Getenv("SNAPSHOT_MANIFEST")
	if version == "" {
		return nil, fmt.Errorf("SNAPSHOT_MANIFEST environment variable not set")
	}
	return vanadiumReleaseKubeCommon(jirix, testName, "staging", version)
}

func vanadiumReleaseKubeProduction(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	return vanadiumReleaseKubeCommon(jirix, testName, "production", "")
}

func vanadiumReleaseKubeCommon(jirix *jiri.X, testName, updateType, version string) (_ *test.Result, e error) {
	cleanup, err := initTest(jirix, testName, []string{"v23:base"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Build and run vprodupdater.
	s := jirix.NewSeq()
	if err := s.Last("jiri", "go", "install", "v.io/infrastructure/vprodupdater/"); err != nil {
		return nil, newInternalError(err, "Build vprodupdater")
	}
	vprodupdaterBin := filepath.Join(jirix.Root, "infrastructure", "go", "bin", "vprodupdater")
	args := []string{
		fmt.Sprintf("-type=%s", updateType),
	}
	if version != "" {
		args = append(args, fmt.Sprintf("-tag=%s", version))
	}
	if err := s.Last(vprodupdaterBin, args...); err != nil {
		return nil, newInternalError(err, "Run vprodupdater")
	}
	return &test.Result{Status: test.Passed}, nil
}
