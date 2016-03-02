// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go -env=CMDLINE_PREFIX=jiri .

package main

import (
	"v.io/jiri"
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
	"v.io/x/devtools/jiri-profile-v23/terraform_profile"
)

// commandLineDriver implements the command line for the 'profile-v23'
// subcommand.
var commandLineDriver = &cmdline.Command{
	Name:  "profile-v23",
	Short: "Manage v23 profiles",
	Long:  profilescmdline.HelpMsg(),
}

func main() {
	android_profile.Register("v23", "android")
	base_profile.Register("v23", "base")
	dart_profile.Register("v23", "dart")
	go_profile.Register("v23", "go")
	java_profile.Register("v23", "java")
	mojo_profile.Register("v23", "mojo")
	mojo_dev_profile.Register("v23", "mojodev")
	nacl_profile.Register("v23", "nacl")
	nodejs_profile.Register("v23", "nodejs")
	syncbase_profile.Register("v23", "syncbase")
	terraform_profile.Register("v23", "terraform")

	profilescmdline.RegisterManagementCommands(commandLineDriver, true, "v23", jiri.ProfilesDBDir, jiri.ProfilesRootDir)
	tool.InitializeRunFlags(&commandLineDriver.Flags)
	cmdline.Main(commandLineDriver)
}
