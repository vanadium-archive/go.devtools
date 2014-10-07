package impl

import (
	"fmt"

	"tools/lib/cmdline"
	"tools/lib/util"
)

var (
	branchesFlag bool
	gcFlag       bool
	manifestFlag string
	novdlFlag    bool
	platformFlag string
	verboseFlag  bool
)

func init() {
	cmdProjectList.Flags.BoolVar(&branchesFlag, "branches", false, "Show project branches.")
	cmdProjectUpdate.Flags.BoolVar(&gcFlag, "gc", false, "Garbage collect obsolete repositories.")
	cmdProjectUpdate.Flags.StringVar(&manifestFlag, "manifest", "absolute", "Name of the project manifest.")
	cmdSelfUpdate.Flags.StringVar(&manifestFlag, "manifest", "absolute", "Name of the project manifest.")
	cmdGo.Flags.BoolVar(&novdlFlag, "novdl", false, "Disable automatic generation of vdl files.")
	cmdXGo.Flags.BoolVar(&novdlFlag, "novdl", false, "Disable automatic generation of vdl files.")
	cmdEnv.Flags.StringVar(&platformFlag, "platform", "", "Target platform.")
	cmdRoot.Flags.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
}

// Root returns a command that represents the root of the veyron tool.
func Root() *cmdline.Command {
	return cmdRoot
}

// cmdRoot represents the root of the veyron tool.
var cmdRoot = &cmdline.Command{
	Name:  "veyron",
	Short: "Tool for managing veyron development",
	Long:  "The veyron tool helps manage veyron development.",
	Children: []*cmdline.Command{
		cmdProfile,
		cmdProject,
		cmdEnv,
		cmdRun,
		cmdGo,
		cmdGoExt,
		cmdXGo,
		cmdSelfUpdate,
		cmdVersion,
	},
}

// cmdSelfUpdate represents the 'selfupdate' command of the veyron tool.
var cmdSelfUpdate = &cmdline.Command{
	Run:   runSelfUpdate,
	Name:  "selfupdate",
	Short: "Update the veyron tool",
	Long:  "Download and install the latest version of the veyron tool.",
}

func runSelfUpdate(command *cmdline.Command, _ []string) error {
	return util.SelfUpdate(verboseFlag, command.Stdout(), manifestFlag, "veyron")
}

// cmdVersion represents the 'version' command of the veyron tool.
var cmdVersion = &cmdline.Command{
	Run:   runVersion,
	Name:  "version",
	Short: "Print version",
	Long:  "Print version of the veyron tool.",
}

// Version should be over-written during build:
//
// go build -ldflags "-X tools/veyron/impl.Version <version>" tools/veyron
var Version string = "manual-build"

func runVersion(command *cmdline.Command, _ []string) error {
	fmt.Fprintf(command.Stdout(), "veyron tool version %v\n", Version)
	return nil
}
