// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "v.io/x/lib/cmdline"

const (
	bucket = "gs://vanadium-oncall/data"
)

var (
	colorFlag   bool
	dryrunFlag  bool
	verboseFlag bool
)

func init() {
	cmdRoot.Flags.BoolVar(&colorFlag, "color", true, "Use color to format output.")
	cmdRoot.Flags.BoolVar(&dryrunFlag, "n", false, "Show what commands will run, but do not execute them.")
	cmdRoot.Flags.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
}

// root returns a command that represents the root of the oncall tool.
func root() *cmdline.Command {
	return cmdRoot
}

// cmdRoot represents the root of the oncall tool.
var cmdRoot = &cmdline.Command{
	Name:     "oncall",
	Short:    "Command oncall implements oncall specific utilities used by Vanadium team",
	Long:     "Command oncall implements oncall specific utilities used by Vanadium team.",
	Children: []*cmdline.Command{cmdCollect, cmdServe},
}
