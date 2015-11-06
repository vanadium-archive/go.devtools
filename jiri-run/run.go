// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go -env=CMDLINE_PREFIX=jiri . -help

package main

import (
	"flag"
	"fmt"
	"os/exec"
	"strings"

	"v.io/jiri/profiles"
	"v.io/jiri/tool"
	"v.io/jiri/util"
	"v.io/x/devtools/jiri-v23-profile/v23_profile"
	"v.io/x/lib/cmdline"
)

var (
	manifestFlag, profilesFlag string
	profilesModeFlag           profiles.ProfilesMode
	targetFlag                 profiles.Target
	mergePoliciesFlag          profiles.MergePolicies
	verboseFlag                bool
)

func init() {
	tool.InitializeRunFlags(&cmdRun.Flags)
	mergePoliciesFlag = profiles.JiriMergePolicies()
	profiles.RegisterProfileFlags(&cmdRun.Flags, &profilesModeFlag, &manifestFlag, &profilesFlag, v23_profile.DefaultManifestFilename, &mergePoliciesFlag, &targetFlag)
	flag.BoolVar(&verboseFlag, "v", false, "print verbose debugging information")
}

// cmdRun represents the "jiri run" command.
var cmdRun = &cmdline.Command{
	Runner:   cmdline.RunnerFunc(runRun),
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
func runRun(cmdlineEnv *cmdline.Env, args []string) error {
	if len(args) == 0 {
		return cmdlineEnv.UsageErrorf("no command to run")
	}
	ctx := tool.NewContextFromEnv(cmdlineEnv)
	ch, err := profiles.NewConfigHelper(ctx, profilesModeFlag, manifestFlag)
	if err != nil {
		return err
	}
	profileNames := profiles.InitProfilesFromFlag(profilesFlag, profiles.DoNotAppendJiriProfile)
	if err := ch.ValidateRequestedProfilesAndTarget(profileNames, targetFlag); err != nil {
		return err
	}
	ch.MergeEnvFromProfiles(mergePoliciesFlag, targetFlag, profileNames...)
	if verboseFlag {
		fmt.Fprintf(ctx.Stdout(), "Merged profiles: %v\n", profileNames)
		fmt.Fprintf(ctx.Stdout(), "Merge policies: %v\n", mergePoliciesFlag)
		fmt.Fprintf(ctx.Stdout(), "%v\n", strings.Join(ch.ToSlice(), "\n"))
	}
	execCmd := exec.Command(args[0], args[1:]...)
	execCmd.Stdout = cmdlineEnv.Stdout
	execCmd.Stderr = cmdlineEnv.Stderr
	execCmd.Env = ch.ToSlice()
	return util.TranslateExitCode(execCmd.Run())
}

func main() {
	cmdline.Main(cmdRun)
}
