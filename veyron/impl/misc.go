package impl

import (
	"fmt"
	"os/exec"
	"syscall"

	"tools/lib/cmdline"
	"tools/lib/envutil"
	"tools/lib/util"
)

// translateExitCode translates errors from the "os/exec" package that contain
// exit codes into cmdline.ErrExitCode errors.
func translateExitCode(err error) error {
	if exit, ok := err.(*exec.ExitError); ok {
		if wait, ok := exit.Sys().(syscall.WaitStatus); ok {
			if status := wait.ExitStatus(); wait.Exited() && status != 0 {
				return cmdline.ErrExitCode(status)
			}
		}
	}
	return err
}

// cmdEnv represents the 'env' command of the veyron tool.
var cmdEnv = &cmdline.Command{
	Run:   runEnv,
	Name:  "env",
	Short: "Print veyron environment variables",
	Long: `
Print veyron environment variables.

If no arguments are given, prints all variables in NAME="VALUE" format,
each on a separate line ordered by name.  This format makes it easy to set
all vars by running the following bash command (or similar for other shells):
   eval $(veyron env)

If arguments are given, prints only the value of each named variable,
each on a separate line in the same order as the arguments.
`,
	ArgsName: "[name ...]",
	ArgsLong: "[name ...] is an optional list of variable names.",
}

func runEnv(command *cmdline.Command, args []string) error {
	platform, err := util.ParsePlatform(platformFlag)
	if err != nil {
		return err
	}
	env, err := util.VeyronEnvironment(platform)
	if err != nil {
		return err
	}
	if len(args) > 0 {
		for _, name := range args {
			fmt.Fprintln(command.Stdout(), env.Get(name))
		}
		return nil
	}
	for _, entry := range envutil.ToQuotedSlice(env.DeltaMap()) {
		fmt.Fprintln(command.Stdout(), entry)
	}
	return nil
}

// cmdRun represents the 'run' command of the veyron tool.
var cmdRun = &cmdline.Command{
	Run:      runRun,
	Name:     "run",
	Short:    "Run an executable using the veyron environment",
	Long:     "Run an executable using the veyron environment.",
	ArgsName: "<executable> [arg ...]",
	ArgsLong: `
<executable> [arg ...] is the executable to run and any arguments to pass
verbatim to the executable.
`,
}

func runRun(command *cmdline.Command, args []string) error {
	if len(args) == 0 {
		return command.UsageErrorf("no command to run")
	}
	env, err := util.VeyronEnvironment(util.HostPlatform())
	if err != nil {
		return err
	}
	// For certain commands, veyron uses specialized wrappers that do
	// more than just set up the veyron environment. If the user is
	// trying to run any of these commands using the 'run' command,
	// inform the user that they should use the specialized wrapper.
	switch args[0] {
	case "go":
		return fmt.Errorf(`use "veyron go" instead of "veyron run go"`)
	}
	execCmd := exec.Command(args[0], args[1:]...)
	execCmd.Stdout = command.Stdout()
	execCmd.Stderr = command.Stderr()
	execCmd.Env = env.Slice()
	return translateExitCode(execCmd.Run())
}
