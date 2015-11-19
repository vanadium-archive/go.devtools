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

// cmdEnv represents the "jiri env" command.
var cmdEnv = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runEnv),
	Name:   "env",
	Short:  "Print vanadium environment variables (deprecated: use jiri v23-profile env instead)",
	Long: `
NOTE: this command is deprecated, please use jiri v23-profile env instead.`,
}

func runEnv(cmdlineEnv *cmdline.Env, args []string) error {
	fmt.Fprintf(os.Stdout, "jiri env is deprecated - please use jiri v23-profile env instead. This tool will be deleted before thanksgiving.\n")
	return nil
}

func main() {
	cmdline.Main(cmdEnv)
}
