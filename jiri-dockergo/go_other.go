// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !linux

package main

// fileGid is supposed to return the group id of the provided filename.
// However, this implementation always returns 0.
//
// The returned value is used to set the -u flag for "docker run". When docker
// runs inside a virtual machine (as will be the case if the host is not
// Linux), then the group id on the host is meaningless to the virtual machine,
// so there is no point in extracting a group id from filename that is only
// valid on the host.
func fileGid(filename string) (uint32, bool) {
	return 0, false
}
