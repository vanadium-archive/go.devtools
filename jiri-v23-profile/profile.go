// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go -env=CMDLINE_PREFIX=jiri .

package main

import (
	"v.io/jiri/profiles/commandline"
	"v.io/jiri/tool"
	"v.io/x/devtools/jiri-v23-profile/v23_profile"
	"v.io/x/lib/cmdline"

	// Add profile manager implementations here.
	_ "v.io/x/devtools/jiri-v23-profile/android"
	_ "v.io/x/devtools/jiri-v23-profile/base"
	_ "v.io/x/devtools/jiri-v23-profile/dart"
	_ "v.io/x/devtools/jiri-v23-profile/go"
	_ "v.io/x/devtools/jiri-v23-profile/java"
	_ "v.io/x/devtools/jiri-v23-profile/mojo"
	_ "v.io/x/devtools/jiri-v23-profile/mojo-dev"
	_ "v.io/x/devtools/jiri-v23-profile/nacl"
	_ "v.io/x/devtools/jiri-v23-profile/nodejs"
	_ "v.io/x/devtools/jiri-v23-profile/syncbase"
)

// commandLineDriver implements the command line for the 'v23-profile'
// subcommand.
var commandLineDriver = &cmdline.Command{
	Name:  "v23-profile",
	Short: "Manage profiles",
	Long:  commandline.HelpMsg,
}

func main() {
	commandline.RegisterManagementCommands(commandLineDriver, v23_profile.DefaultDBFilename)
	commandline.RegisterReaderCommands(commandLineDriver, v23_profile.DefaultDBFilename)
	tool.InitializeRunFlags(&commandLineDriver.Flags)
	cmdline.Main(commandLineDriver)
}
