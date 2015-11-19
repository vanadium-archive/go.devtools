// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go -env=CMDLINE_PREFIX=jiri . -help

package main

import (
	"fmt"
	"os"

	"v.io/x/lib/cmdline"
)

// cmdProfile represents the "jiri profile" command.
var cmdProfile = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runProfile),
	Name:   "profile",
	Short:  "Manage vanadium profiles (deprecated: use jiri v23-profile instead)",
	Long: `
NOTE: this command is deprecated, please use jiri v23-profile instead.`,
}

func runProfile(cmdlineEnv *cmdline.Env, args []string) error {
	fmt.Fprintf(os.Stdout, "jiri profile is deprecated - please use jiri v23-profile instead. This tool will be deleted before thanksgiving.\n")
	return nil
}

func main() {
	cmdline.Main(cmdProfile)
}
