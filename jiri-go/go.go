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
)

func init() {
	tool.InitializeRunFlags(&cmdGo.Flags)
	profiles.RegisterProfileFlags(&cmdGo.Flags, &profilesModeFlag, &manifestFlag, &profilesFlag, v23_profile.DefaultManifestFilename, &targetFlag)
	flag.BoolVar(&systemGoFlag, "system-go", false, "use the version of go found in $PATH rather than that built by the go profile")
	flag.BoolVar(&verboseFlag, "v", false, "print verbose debugging information")
	flag.StringVar(&extraLDFlags, "extra-ldflags", "", golib.ExtraLDFlagsFlagDescription)
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
	ch.SetGoPath()
	ch.SetVDLPath()
	if err := ch.ValidateRequestedProfilesAndTarget(strings.Split(profilesFlag, ","), targetFlag); err != nil {
		return err
	}

	/*
		// TODO(cnicolaou): there is a CL in flight to handle merge environment
		// variables in a more principled and consistent manner. Clean this up
		// when it's checked in.
		ignoredVariables := map[string]bool{"GOPATH": true} // never use GOPATH from a profile
		// If we're run from the presubmit test, then prefer the
		// env vars in the profile...
		if os.Getenv("TEST") == "" {
			for k, _ := range profiles.CommonIgnoreVariables() {
				if ch.Get(k) != "" {
					// Prefer variables from the environment.
					ignoredVariables[k] = true
				}
			}
		}
	*/
	ch.SetEnvFromProfiles(profiles.CommonConcatVariables(), profiles.CommonIgnoreVariables(), profilesFlag, targetFlag)
	if !systemGoFlag {
		if len(ch.Get("GOROOT")) > 0 {
			ch.PrependToPATH(filepath.Join(ch.Get("GOROOT"), "bin"))
		}
	}
	if verboseFlag {
		fmt.Fprintf(ctx.Stdout(), "Environment: %v\n", strings.Join(ch.ToSlice(), "\n"))
	}
	envMap := ch.ToMap()
	if args, err = golib.PrepareGo(ctx, envMap, args, extraLDFlags); err != nil {
		return err
	}
	// Run the go tool.
	goBin, err := runutil.LookPath("go", envMap)
	if err != nil {
		return err
	}
	opts := ctx.Run().Opts()
	opts.Env = envMap
	return util.TranslateExitCode(ctx.Run().CommandWithOpts(opts, goBin, args...))
}

func main() {
	cmdline.Main(cmdGo)
}
