// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"v.io/jiri/collect"
	"v.io/jiri/project"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/test"
)

// vanadiumGoBinaries uploads Vanadium binaries to Google Storage.
func vanadiumGoBinaries(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(ctx, testName, []string{"base"})
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	args := []string{"update", "-manifest=snapshot/stable-go"}
	// Fetch the latest stable Go snapshot.
	if err := ctx.Run().Command("jiri", args...); err != nil {
		return nil, internalTestError{err, "Update"}
	}
	fmt.Fprintf(ctx.Stdout(), "jiri %s: success\n", args)

	// Build all v.io binaries.
	//
	// The "leveldb" tag is needed to compile the levelDB-based storage
	// engine for the groups service. See v.io/i/632 for more details.
	args = []string{"go", "install", "-tags=leveldb", "v.io/..."}
	if err := ctx.Run().Command("jiri", args...); err != nil {
		return nil, internalTestError{err, "Install"}
	}

	fmt.Fprintf(ctx.Stdout(), "jiri %s: success\n", args)

	// Compute the timestamp for the build snapshot.
	labelFile, err := project.ManifestFile("snapshot/stable-go")
	if err != nil {
		return nil, internalTestError{err, "ManifestFile"}
	}
	snapshotFile, err := filepath.EvalSymlinks(labelFile)
	if err != nil {
		return nil, internalTestError{err, "EvalSymlinks"}
	}
	timestamp := filepath.Base(snapshotFile)

	// Upload all v.io binaries to Google Storage.
	bucket := fmt.Sprintf("gs://vanadium-binaries/%s_%s/", runtime.GOOS, runtime.GOARCH)
	root, err := project.JiriRoot()
	if err != nil {
		return nil, internalTestError{err, "JiriRoot"}
	}
	binaries := filepath.Join(root, "release", "go", "bin", "*")

	ctx.Run().Command("ls", filepath.Dir(binaries))

	args = []string{"-m", "-q", "cp", binaries, bucket + timestamp}
	fmt.Fprintf(ctx.Stdout(), "gsutil %s ......\n", args)
	if err := ctx.Run().Command("gsutil", args...); err != nil {
		return nil, internalTestError{err, "Upload"}
	}
	fmt.Fprintf(ctx.Stdout(), "gsutil %s: success\n", args)

	// Upload two files: 1) a file that identifies the directory
	// containing the latest set of binaries and 2) a file that
	// indicates that the upload of binaries succeeded.
	tmpDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		return nil, internalTestError{err, "TempDir"}
	}
	defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpDir) }, &e)
	doneFile := filepath.Join(tmpDir, ".done")
	if err := ctx.Run().WriteFile(doneFile, nil, os.FileMode(0600)); err != nil {
		return nil, internalTestError{err, "WriteFile"}
	}
	fmt.Fprintf(ctx.Stdout(), "Created %s: succcess\n", doneFile)
	args = []string{"-q", "cp", doneFile, bucket + timestamp}
	if err := ctx.Run().Command("gsutil", args...); err != nil {
		return nil, internalTestError{err, "Upload"}
	}
	fmt.Fprintf(ctx.Stdout(), "gsutil %s: success\n", args)

	latestFile := filepath.Join(tmpDir, "latest")
	if err := ctx.Run().WriteFile(latestFile, []byte(timestamp), os.FileMode(0600)); err != nil {
		return nil, internalTestError{err, "WriteFile"}
	}
	fmt.Fprintf(ctx.Stdout(), "Created %s: succcess\n", latestFile)
	args = []string{"-q", "cp", latestFile, bucket}
	if err := ctx.Run().Command("gsutil", args...); err != nil {
		return nil, internalTestError{err, "Upload"}
	}
	fmt.Fprintf(ctx.Stdout(), "gsutil %s: success\n", args)

	return &test.Result{Status: test.Passed}, nil
}
