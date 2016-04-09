// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go -env=CMDLINE_PREFIX=jiri . -help

package main

import (
	"flag"
	"fmt"
	"strings"

	"v.io/jiri"
	"v.io/jiri/profiles/profilescmdline"
	"v.io/jiri/profiles/profilesreader"
	"v.io/jiri/runutil"
	"v.io/jiri/tool"
	"v.io/x/devtools/tooldata"
	"v.io/x/lib/cmdline"
)

var (
	readerFlags profilescmdline.ReaderFlagValues
	envFlag     bool
)

func init() {
	tool.InitializeRunFlags(&cmdRun.Flags)
	profilescmdline.RegisterReaderFlags(&cmdRun.Flags, &readerFlags, "v23:base", jiri.ProfilesDBDir)
	flag.BoolVar(&envFlag, "print-run-env", false, "print detailed info on environment variables and the command line used")
}

// cmdRun represents the "jiri run" command.
var cmdRun = &cmdline.Command{
	Runner:   jiri.RunnerFunc(runRun),
	Name:     "run",
	Short:    "Run an executable using the specified profile and target's environment",
	Long:     "Run an executable using the specified profile and target's environment.",
	ArgsName: "<executable> [arg ...]",
	ArgsLong: `
<executable> [arg ...] is the executable to run and any arguments to pass
verbatim to the executable.
`,
}

// TODO(cnicolaou,nlacasse): consider moving run into the core
// jiri tool since there really dones't need to be anything
// project specific in it.
func runRun(jirix *jiri.X, args []string) error {
	if len(args) == 0 {
		return jirix.UsageErrorf("no command to run")
	}
	config, err := tooldata.LoadConfig(jirix)
	if err != nil {
		return err
	}
	rd, err := profilesreader.NewReader(jirix, readerFlags.ProfilesMode, readerFlags.DBFilename)
	if err != nil {
		return err
	}
	profileNames := strings.Split(readerFlags.Profiles, ",")
	if err := rd.ValidateRequestedProfilesAndTarget(profileNames, readerFlags.Target); err != nil {
		return err
	}
	rd.MergeEnvFromProfiles(readerFlags.MergePolicies, readerFlags.Target, profileNames...)
	mp := profilesreader.MergePolicies{
		"GOPATH":  profilesreader.PrependPath,
		"VDLPATH": profilesreader.PrependPath,
	}
	profilesreader.MergeEnv(mp, rd.Vars, []string{config.GoPath(jirix), config.VDLPath(jirix)})
	if envFlag {
		fmt.Fprintf(jirix.Stdout(), "Merged profiles: %v\n", profileNames)
		fmt.Fprintf(jirix.Stdout(), "Merge policies: %v\n", readerFlags.MergePolicies)
		fmt.Fprintf(jirix.Stdout(), "%v\n", strings.Join(rd.ToSlice(), "\n"))
		fmt.Fprintf(jirix.Stdout(), "%s\n", strings.Join(args, " "))
	}
	err = jirix.NewSeq().Env(rd.ToMap()).Capture(jirix.Stdout(), jirix.Stderr()).Last(args[0], args[1:]...)
	return runutil.TranslateExitCode(err)
}

func main() {
	cmdline.Main(cmdRun)
}
