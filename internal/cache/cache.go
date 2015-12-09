// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	"path/filepath"
	"strings"

	"v.io/jiri/collect"
	"v.io/jiri/jiri"
	"v.io/jiri/runutil"
)

// StoreGoogleStorageFile reads the given file from the given Google Storage
// location and stores it in the given cache location. It returns the cached
// file path.
func StoreGoogleStorageFile(jirix *jiri.X, cacheRoot, bucketRoot, filename string) (_ string, e error) {
	s := jirix.NewSeq()
	cachedFile := filepath.Join(cacheRoot, filename)
	if _, err := s.Stat(cachedFile); err != nil {
		if !runutil.IsNotExist(err) {
			return "", err
		}
		// To avoid interference between concurrent requests, download data to a
		// tmp dir, and move it to the final location.
		tmpDir, err := s.TempDir(cacheRoot, "")
		if err != nil {
			return "", err
		}
		defer collect.Error(func() error { return jirix.NewSeq().RemoveAll(tmpDir).Done() }, &e)
		if err := s.Last("gsutil", "-m", "-q", "cp", "-r", bucketRoot+"/"+filename, tmpDir); err != nil {
			return "", err
		}
		if err := s.Rename(filepath.Join(tmpDir, filename), cachedFile).Done(); err != nil {
			// If the target directory already exists, it must have been created by
			// a concurrent request.
			if !strings.Contains(err.Error(), "directory not empty") {
				return "", err
			}
		}
	}
	return cachedFile, nil
}
