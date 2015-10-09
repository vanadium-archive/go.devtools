// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "syscall"

// fileGid returns the group id of the provided filename.
//
// The returned value is intended for use with the -u flag to 'docker run'.
func fileGid(filename string) (uint32, bool) {
	var res syscall.Stat_t
	syscall.Stat(filename, &res)
	return res.Gid, true
}
