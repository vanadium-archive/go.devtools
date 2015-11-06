// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go -env=CMDLINE_PREFIX=jiri . -help

package main

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"v.io/jiri/profiles"
	"v.io/jiri/runutil"
	"v.io/jiri/tool"
	"v.io/jiri/util"
	"v.io/x/devtools/internal/golib"
	"v.io/x/devtools/jiri-v23-profile/v23_profile"
	"v.io/x/lib/cmdline"
)

// cmdGo represents the "jiri go" command.
var cmdGo = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runGo),
	Name:   "go",
	Short:  "Execute the go tool using the vanadium environment",
	Long: `
Wrapper around the 'go' tool that can be used for compilation of
vanadium Go sources. It takes care of vanadium-specific setup, such as
setting up the Go specific environment variables or making sure that
VDL generated files are regenerated before compilation.

In particular, the tool invokes the following command before invoking
any go tool commands that compile vanadium Go code:

vdl generate -lang=go all
`,
	ArgsName: "<arg ...>",
	ArgsLong: "<arg ...> is a list of arguments for the go tool.",
}

var (
	manifestFlag, profilesFlag string
	systemGoFlag, verboseFlag  bool
	profilesModeFlag           profiles.ProfilesMode
	targetFlag                 profiles.Target
	extraLDFlags               string
	mergePoliciesFlag          profiles.MergePolicies
)

func init() {
	mergePoliciesFlag = profiles.JiriMergePolicies()
	profiles.RegisterProfileFlags(&cmdGo.Flags, &profilesModeFlag, &manifestFlag, &profilesFlag, v23_profile.DefaultManifestFilename, &mergePoliciesFlag, &targetFlag)
	flag.BoolVar(&systemGoFlag, "system-go", false, "use the version of go found in $PATH rather than that built by the go profile")
	flag.BoolVar(&verboseFlag, "v", false, "print verbose debugging information")
	flag.StringVar(&extraLDFlags, "extra-ldflags", "", golib.ExtraLDFlagsFlagDescription)
	tool.InitializeRunFlags(&cmdGo.Flags)
}

func runGo(cmdlineEnv *cmdline.Env, args []string) error {
	if len(args) == 0 {
		return cmdlineEnv.UsageErrorf("not enough arguments")
	}
	ctx := tool.NewContextFromEnv(cmdlineEnv)
	ch, err := profiles.NewConfigHelper(ctx, profilesModeFlag, manifestFlag)
	if err != nil {
		return err
	}
	profileNames := profiles.InitProfilesFromFlag(profilesFlag, profiles.AppendJiriProfile)
	if err := ch.ValidateRequestedProfilesAndTarget(profileNames, targetFlag); err != nil {
		return err
	}
	ch.MergeEnvFromProfiles(mergePoliciesFlag, targetFlag, profileNames...)
	if !systemGoFlag {
		if len(ch.Get("GOROOT")) > 0 {
			ch.PrependToPATH(filepath.Join(ch.Get("GOROOT"), "bin"))
		}
	}
	if verboseFlag {
		fmt.Fprintf(ctx.Stdout(), "Merged profiles: %v\n", profileNames)
		fmt.Fprintf(ctx.Stdout(), "Merge policies: %v\n", mergePoliciesFlag)
		fmt.Fprintf(ctx.Stdout(), "%v\n", strings.Join(ch.ToSlice(), "\n"))
	}
	envMap := ch.ToMap()
	var installSuffix string
	if targetFlag.OS() == "fnl" {
		installSuffix = "musl"
	}
	if args, err = golib.PrepareGo(ctx, envMap, args, extraLDFlags, installSuffix); err != nil {
		return err
	}
	// Run the go tool.
	goBin, err := runutil.LookPath("go", envMap)
	if err != nil {
		return err
	}
	if verboseFlag {
		fmt.Fprintf(ctx.Stdout(), "\n%v %s\n", goBin, strings.Join(args, " "))
	}
	opts := ctx.Run().Opts()
	opts.Env = envMap
	return util.TranslateExitCode(ctx.Run().CommandWithOpts(opts, goBin, args...))
}

func main() {
	cmdline.Main(cmdGo)
}
