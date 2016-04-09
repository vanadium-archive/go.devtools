// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go -env=CMDLINE_PREFIX=jiri .

package main

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"v.io/jiri"
	"v.io/jiri/profiles/profilescmdline"
	"v.io/jiri/profiles/profilesreader"
	"v.io/jiri/tool"
	"v.io/x/lib/cmdline"
)

var (
	envFlag     bool
	readerFlags profilescmdline.ReaderFlagValues
)

func init() {
	profilescmdline.RegisterReaderFlags(&cmdGoExt.Flags, &readerFlags, "v23:base", jiri.ProfilesDBDir)
	flag.BoolVar(&envFlag, "print-run-env", false, "print detailed info on environment variables and the command line used")
	tool.InitializeRunFlags(&cmdGoExt.Flags)
}

// cmdGoExt represents the "jiri goext" command.
var cmdGoExt = &cmdline.Command{
	Name:     "goext",
	Short:    "Vanadium extensions of the go tool",
	Long:     "Vanadium extensions of the go tool.",
	Children: []*cmdline.Command{cmdGoExtDistClean},
}

// cmdGoExtDistClean represents the "jiri goext distclean" command.
var cmdGoExtDistClean = &cmdline.Command{
	Runner: jiri.RunnerFunc(runGoExtDistClean),
	Name:   "distclean",
	Short:  "Restore the vanadium Go workspaces to their pristine state",
	Long: `
Unlike the 'go clean' command, which only removes object files for
packages in the source tree, the 'goext disclean' command removes all
object files from vanadium Go workspaces. This functionality is needed
to avoid accidental use of stale object files that correspond to
packages that no longer exist in the source tree.
`,
}

func runGoExtDistClean(jirix *jiri.X, _ []string) error {
	rd, err := profilesreader.NewReader(jirix, readerFlags.ProfilesMode, readerFlags.DBFilename)
	if err != nil {
		return err
	}
	rd.MergeEnvFromProfiles(readerFlags.MergePolicies, readerFlags.Target, "jiri")
	failed := false
	if envFlag {
		fmt.Fprintf(jirix.Stdout(), "GOPATH:\n%s\n", strings.Join(rd.GetTokens("GOPATH", ":"), "\n"))
		fmt.Fprintf(jirix.Stdout(), "Jiri Root: %v\n", jirix.Root)
	}
	for _, workspace := range rd.GetTokens("GOPATH", ":") {
		if !strings.HasPrefix(workspace, jirix.Root) {
			continue
		}
		for _, name := range []string{"bin", "pkg"} {
			dir := filepath.Join(workspace, name)
			if envFlag {
				fmt.Fprintf(jirix.Stdout(), "removing: %s\n", dir)
			}
			if err := jirix.NewSeq().RemoveAll(dir).Done(); err != nil {
				failed = true
			}
		}
	}
	if failed {
		return cmdline.ErrExitCode(2)
	}
	return nil
}

func main() {
	cmdline.Main(cmdGoExt)
}
