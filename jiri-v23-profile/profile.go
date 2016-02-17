// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go -env=CMDLINE_PREFIX=jiri .

package main

import (
	"v.io/jiri/profiles/profilescmdline"
	"v.io/jiri/tool"
	"v.io/x/lib/cmdline"

	// Add profile manager implementations here.
	"v.io/x/devtools/jiri-profile-v23/android_profile"
	"v.io/x/devtools/jiri-profile-v23/base_profile"
	"v.io/x/devtools/jiri-profile-v23/dart_profile"
	"v.io/x/devtools/jiri-profile-v23/go_profile"
	"v.io/x/devtools/jiri-profile-v23/java_profile"
	"v.io/x/devtools/jiri-profile-v23/mojo_dev_profile"
	"v.io/x/devtools/jiri-profile-v23/mojo_profile"
	"v.io/x/devtools/jiri-profile-v23/nacl_profile"
	"v.io/x/devtools/jiri-profile-v23/nodejs_profile"
	"v.io/x/devtools/jiri-profile-v23/syncbase_profile"
)

// commandLineDriver implements the command line for the 'v23-profile'
// subcommand.
var commandLineDriver = &cmdline.Command{
	Name:  "v23-profile",
	Short: "Manage profiles",
	Long:  profilescmdline.HelpMsg(),
}

// DefaultDBFilename is the default filename used for the profiles database
// by the jiri-v23-profile subcommand.
const DefaultDBFilename = ".jiri_v23_profiles"

func main() {
	android_profile.Register("", "android")
	base_profile.Register("", "base")
	dart_profile.Register("", "dart")
	go_profile.Register("", "go")
	java_profile.Register("", "java")
	mojo_profile.Register("", "mojo")
	mojo_dev_profile.Register("", "mojodev")
	nacl_profile.Register("", "nacl")
	nodejs_profile.Register("", "nodejs")
	syncbase_profile.Register("", "syncbase")

	profilescmdline.RegisterManagementCommands(commandLineDriver, false, "", DefaultDBFilename, "profiles")
	profilescmdline.RegisterReaderCommands(commandLineDriver, DefaultDBFilename)
	tool.InitializeRunFlags(&commandLineDriver.Flags)
	cmdline.Main(commandLineDriver)
}
