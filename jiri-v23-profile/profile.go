// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go -env=CMDLINE_PREFIX=jiri,JIRI_ROOT=/ .

package main

import (
	"v.io/jiri/profiles/driver"
	"v.io/x/lib/cmdline"

	// Add profile manager implementations here.
	_ "v.io/x/devtools/jiri-xprofile/android"
	_ "v.io/x/devtools/jiri-xprofile/base"
	_ "v.io/x/devtools/jiri-xprofile/go"
	_ "v.io/x/devtools/jiri-xprofile/java"
	_ "v.io/x/devtools/jiri-xprofile/nacl"
	_ "v.io/x/devtools/jiri-xprofile/nodejs"
	_ "v.io/x/devtools/jiri-xprofile/syncbase"
)

func main() {
	cmdline.Main(driver.CommandLineDriver)
}
