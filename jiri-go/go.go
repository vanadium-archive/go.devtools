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

	"v.io/jiri/jiri"
	"v.io/jiri/profiles/commandline"
	"v.io/jiri/profiles/reader"
	"v.io/jiri/runutil"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/golib"
	"v.io/x/devtools/jiri-v23-profile/v23_profile"
	"v.io/x/lib/cmdline"
)

// cmdGo represents the "jiri go" command.
var cmdGo = &cmdline.Command{
	Runner: jiri.RunnerFunc(runGo),
	Name:   "go",
	Short:  "Execute the go tool using the vanadium environment",
	Long: `
Wrapper around the 'go' tool that can be used for compilation of
vanadium Go sources. It takes care of vanadium-specific setup, such as
setting up the Go specific environment variables or making sure that
VDL generated files are regenerated before compilation.
`,
	ArgsName: "<arg ...>",
	ArgsLong: "<arg ...> is a list of arguments for the go tool.",
}

var (
	extraLDFlags string
	systemGoFlag bool
	envFlag      bool
	readerFlags  commandline.ReaderFlagValues
)

func init() {
	commandline.RegisterReaderFlags(&cmdGo.Flags, &readerFlags, v23_profile.DefaultDBFilename)
	flag.BoolVar(&systemGoFlag, "system-go", false, "use the version of go found in $PATH rather than that built by the go profile")
	flag.StringVar(&extraLDFlags, "extra-ldflags", "", golib.ExtraLDFlagsFlagDescription)
	flag.BoolVar(&envFlag, "print-run-env", false, "print detailed info on environment variables and the command line used")
	tool.InitializeRunFlags(&cmdGo.Flags)
}

func runGo(jirix *jiri.X, args []string) error {
	if len(args) == 0 {
		return jirix.UsageErrorf("not enough arguments")
	}
	rd, err := reader.NewReader(jirix, readerFlags.ProfilesMode, readerFlags.DBFilename)
	if err != nil {
		return err
	}
	profileNames := reader.InitProfilesFromFlag(readerFlags.Profiles, reader.AppendJiriProfile)
	if err := rd.ValidateRequestedProfilesAndTarget(profileNames, readerFlags.Target); err != nil {
		return err
	}
	rd.MergeEnvFromProfiles(readerFlags.MergePolicies, readerFlags.Target, profileNames...)
	if !systemGoFlag {
		if len(rd.Get("GOROOT")) > 0 {
			rd.PrependToPATH(filepath.Join(rd.Get("GOROOT"), "bin"))
		}
	}
	if envFlag {
		fmt.Fprintf(jirix.Stdout(), "Merged profiles: %v\n", profileNames)
		fmt.Fprintf(jirix.Stdout(), "Merge policies: %v\n", readerFlags.MergePolicies)
		fmt.Fprintf(jirix.Stdout(), "%v\n", strings.Join(rd.ToSlice(), "\n"))
	}
	envMap := rd.ToMap()
	var installSuffix string
	if readerFlags.Target.OS() == "fnl" {
		installSuffix = "musl"
	}
	if args, err = golib.PrepareGo(jirix, envMap, args, extraLDFlags, installSuffix); err != nil {
		return err
	}
	// Run the go tool.
	goBin, err := runutil.LookPath("go", envMap)
	if err != nil {
		return err
	}
	if envFlag {
		fmt.Fprintf(jirix.Stdout(), "\n%v %s\n", goBin, strings.Join(args, " "))
	}
	err = jirix.NewSeq().Env(envMap).Capture(jirix.Stdout(), jirix.Stderr()).Last(goBin, args...)
	return runutil.TranslateExitCode(err)
}

func main() {
	cmdline.Main(cmdGo)
}
