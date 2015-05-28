// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "v.io/x/lib/cmdline"

var (
	colorFlag     bool
	dryRunFlag    bool
	exportDirFlag string
	verboseFlag   bool
)

func init() {
	cmdRoot.Flags.BoolVar(&colorFlag, "color", true, "Use color to format output.")
	cmdRoot.Flags.BoolVar(&dryRunFlag, "n", false, "Show what commands will run, but do not execute them.")
	cmdRoot.Flags.StringVar(&exportDirFlag, "export-dir", "gs://vanadium-kiosk",
		"Directory for storing/retrieving screenshots. Dirs that start with 'gs://' point to Google Storage buckets.")
	cmdRoot.Flags.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
}

func root() *cmdline.Command {
	return cmdRoot
}

var cmdRoot = &cmdline.Command{
	Name:  "vkiosk",
	Short: "takes and shows screenshots of a given url",
	Long: `
Command vkiosk runs Chrome in a virtual X11 environtment for a given url, takes
its screenshots periodically, exports them to Google Storage, and serves them
in a http server.

This tool is only tested in Debian/Ubuntu.
`,
	Children: []*cmdline.Command{
		cmdCollect,
		cmdServe,
	},
}
