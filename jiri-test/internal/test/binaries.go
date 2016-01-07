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
	"v.io/jiri/jiri"
	"v.io/x/devtools/internal/test"
)

// vanadiumGoBinaries uploads Vanadium binaries to Google Storage.
func vanadiumGoBinaries(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(jirix, testName, []string{"base"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	s := jirix.NewSeq()

	// Fetch the latest stable Go snapshot.
	snapshotArgs := []string{"update", "-manifest=snapshot/stable-go"}
	// The "leveldb" tag is needed to compile the levelDB-based storage
	// engine for the groups service. See v.io/i/632 for more details.
	installArgs := []string{"go", "install", "-tags=leveldb", "v.io/..."}
	err = s.Run("jiri", snapshotArgs...).
		Fprintf(jirix.Stdout(), "jiri %s: success\n", snapshotArgs).
		Run("jiri", installArgs...).
		Fprintf(jirix.Stdout(), "jiri %s: success\n", installArgs).
		Done()
	if err != nil {
		return nil, newInternalError(err, "Update & Install")
	}

	// TODO(nlacasse): Are we going to continue storing snapshots here?  Maybe
	// we need some configuation to tell us where these should be, so we don't
	// need to hard-code this path.
	manifestDir := filepath.Join(jirix.Root, ".manifest", "v2")
	snapshotDir := filepath.Join(manifestDir, "snapshot")

	// Compute the timestamp for the build snapshot.
	labelFile := filepath.Join(snapshotDir, "stable-go")
	snapshotFile, err := filepath.EvalSymlinks(labelFile)
	if err != nil {
		return nil, newInternalError(err, "EvalSymlinks")
	}
	timestamp := filepath.Base(snapshotFile)

	// Upload all v.io binaries to Google Storage.
	bucket := fmt.Sprintf("gs://vanadium-binaries/%s_%s/", runtime.GOOS, runtime.GOARCH)
	binaries := filepath.Join(jirix.Root, "release", "go", "bin", "*")

	// Upload binaries and two files: 1) a file that identifies the directory
	// containing the latest set of binaries and 2) a file that
	// indicates that the upload of binaries succeeded.
	tmpDir, err := s.TempDir("", "")
	if err != nil {
		return nil, newInternalError(err, "TempDir")
	}
	defer collect.Error(func() error { return jirix.NewSeq().RemoveAll(tmpDir).Done() }, &e)
	uploadArgs := []string{"-m", "-q", "cp", binaries, bucket + timestamp}
	doneFile := filepath.Join(tmpDir, ".done")
	latestFile := filepath.Join(tmpDir, "latest")
	doneArgs := []string{"-q", "cp", doneFile, bucket + timestamp}
	latestArgs := []string{"-q", "cp", latestFile, bucket}
	err = s.Run("ls", filepath.Dir(binaries)).
		Fprintf(jirix.Stdout(), "gsutil %s ......\n", uploadArgs).
		Run("gsutil", uploadArgs...).
		Fprintf(jirix.Stdout(), "gsutil %s: success\n", uploadArgs).
		WriteFile(doneFile, nil, os.FileMode(0600)).
		Fprintf(jirix.Stdout(), "Created %s: succcess\n", doneFile).
		Run("gsutil", doneArgs...).
		Fprintf(jirix.Stdout(), "gsutil %s: success\n", doneArgs).
		WriteFile(latestFile, []byte(timestamp), os.FileMode(0600)).
		Fprintf(jirix.Stdout(), "Created %s: succcess\n", latestFile).
		Run("gsutil", latestArgs...).
		Fprintf(jirix.Stdout(), "gsutil %s: success\n", latestArgs).
		Done()
	if err != nil {
		return nil, newInternalError(err, "Upload Binaries, Done & Latest Files")
	}
	return &test.Result{Status: test.Passed}, nil
}
