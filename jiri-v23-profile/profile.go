// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go -env=CMDLINE_PREFIX=jiri,JIRI_ROOT=/ .

package main

import (
	"v.io/jiri/profiles/commandline"
	"v.io/x/devtools/jiri-v23-profile/v23_profile"

	// Add profile manager implementations here.
	_ "v.io/x/devtools/jiri-v23-profile/android"
	_ "v.io/x/devtools/jiri-v23-profile/base"
	_ "v.io/x/devtools/jiri-v23-profile/go"
	_ "v.io/x/devtools/jiri-v23-profile/java"
	_ "v.io/x/devtools/jiri-v23-profile/nacl"
	_ "v.io/x/devtools/jiri-v23-profile/nodejs"
	_ "v.io/x/devtools/jiri-v23-profile/syncbase"
)

func init() {
	commandline.Init(v23_profile.DefaultManifestFilename)
}

func main() {
	commandline.Main()
}
