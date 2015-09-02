// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $V23_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go . -help

package main

import (
	"fmt"
	"os/exec"

	"v.io/jiri/lib/tool"
	"v.io/jiri/lib/util"
	"v.io/x/lib/cmdline"
)

func init() {
	tool.InitializeRunFlags(&cmdRun.Flags)
}

// cmdRun represents the "v23 run" command.
var cmdRun = &cmdline.Command{
	Runner:   cmdline.RunnerFunc(runRun),
	Name:     "run",
	Short:    "Run an executable using the vanadium environment",
	Long:     "Run an executable using the vanadium environment.",
	ArgsName: "<executable> [arg ...]",
	ArgsLong: `
<executable> [arg ...] is the executable to run and any arguments to pass
verbatim to the executable.
`,
}

func runRun(cmdlineEnv *cmdline.Env, args []string) error {
	if len(args) == 0 {
		return cmdlineEnv.UsageErrorf("no command to run")
	}
	ctx := tool.NewContextFromEnv(cmdlineEnv)
	env, err := util.VanadiumEnvironment(ctx)
	if err != nil {
		return err
	}
	// For certain commands, vanadium uses specialized wrappers that do
	// more than just set up the vanadium environment. If the user is
	// trying to run any of these commands using the 'run' command,
	// warn the user that they might want to use the specialized wrapper.
	switch args[0] {
	case "go":
		fmt.Fprintln(cmdlineEnv.Stderr, `WARNING: using "v23 run go" instead of "v23 go" skips vdl generation`)
	}
	execCmd := exec.Command(args[0], args[1:]...)
	execCmd.Stdout = cmdlineEnv.Stdout
	execCmd.Stderr = cmdlineEnv.Stderr
	execCmd.Env = env.ToSlice()
	return util.TranslateExitCode(execCmd.Run())
}

func main() {
	cmdline.Main(cmdRun)
}
