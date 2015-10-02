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

	"v.io/jiri/profiles"
	"v.io/jiri/tool"
	"v.io/x/lib/cmdline"
	"v.io/x/lib/envvar"
)

func init() {
	tool.InitializeRunFlags(&cmdEnv.Flags)
}

// cmdEnv represents the "jiri env" command.
var cmdEnv = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runEnv),
	Name:   "env",
	Short:  "Print vanadium environment variables",
	Long: `
Print vanadium environment variables.

If no arguments are given, prints all variables in NAME="VALUE" format,
each on a separate line ordered by name.  This format makes it easy to set
all vars by running the following bash command (or similar for other shells):
   eval $(jiri env)

If arguments are given, prints only the value of each named variable,
each on a separate line in the same order as the arguments.
`,
	ArgsName: "[name ...]",
	ArgsLong: "[name ...] is an optional list of variable names.",
}

var (
	manifestFlag, profilesFlag string
	targetFlag                 profiles.Target
	systemGoFlag               bool
)

func init() {
	profiles.RegisterProfileFlags(&cmdEnv.Flags, &manifestFlag, &profilesFlag, &targetFlag)
	flag.BoolVar(&systemGoFlag, "system-go", false, "use the version of go found in $PATH rather than that built by the go profile")
}

func runEnv(cmdlineEnv *cmdline.Env, args []string) error {
	ctx := tool.NewContextFromEnv(cmdlineEnv)
	ch, err := profiles.NewConfigHelper(ctx, profiles.DefaultManifestFilename)
	if err != nil {
		return err
	}
	if !ch.LegacyProfiles() {
		if err := profiles.ValidateRequestedProfilesAndTarget(strings.Split(profilesFlag, ","), targetFlag); err != nil {
			return err
		}
	}
	ch.SetGoPath()
	ch.SetVDLPath()
	ch.SetEnvFromProfiles(profiles.CommonConcatVariables(), profilesFlag, targetFlag)
	if len(args) > 0 {
		for _, name := range args {
			fmt.Fprintln(cmdlineEnv.Stdout, ch.Get(name))
		}
		return nil
	}
	for key, delta := range ch.Deltas() {
		var value string
		if delta != nil {
			value = `"` + strings.Replace(*delta, `"`, `\"`, -1) + `"`
		}
		fmt.Fprintln(cmdlineEnv.Stdout, envvar.JoinKeyValue(key, value))
	}
	return nil
}

func main() {
	cmdline.Main(cmdEnv)
}
