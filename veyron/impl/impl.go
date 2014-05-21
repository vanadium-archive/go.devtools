package impl

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"runtime"

	"veyron/lib/cmdline"
)

// Root returns a command that represents the root of the veyron tool.
func Root() *cmdline.Command {
	return &cmdline.Command{
		Name:  "veyron",
		Short: "Command-line tool for managing the veyron project",
		Long: `
The veyron tool facilitates interaction with the veyron project.
In particular, it can be used to install different veyron profiles.
`,
		Children: []*cmdline.Command{cmdSetup, cmdVersion},
	}
}

func profilesDescription() string {
	result := "<profiles> is a list of profiles to set up. Supported profiles are:\n"
	root := os.Getenv("VEYRON_ROOT")
	if root == "" {
		panic("VEYRON_ROOT is not set.")
	}
	dir := path.Join(root, "environment/scripts/setup", runtime.GOOS)
	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		panic(fmt.Sprintf("Could not read %s.", dir))
	}
	for _, entry := range entries {
		file := path.Join(dir, entry.Name(), "DESCRIPTION")
		description, err := ioutil.ReadFile(file)
		if err != nil {
			panic(fmt.Sprintf("Could not read %s.", file))
		}
		result += fmt.Sprintf("  %s: %s", entry.Name(), string(description))
	}
	return result
}

// cmdVersion represent the 'version' command of the veyron tool.
var cmdVersion = &cmdline.Command{
	Run:   runVersion,
	Name:  "version",
	Short: "Print version.",
	Long:  "Print version and commit hash used to build the veyron tool.",
}

const version string = "0.1.0"

// commitId should be over-written during build:
// go build -ldflags "-X tools/veyron/impl.commitId <commitId>" tools/veyron
var commitId string = "test-build"

func runVersion(cmd *cmdline.Command, args []string) error {
	fmt.Printf("%v (%v)\n", version, commitId)
	return nil
}

// cmdSetup represents the 'setup' command of the veyron tool.
var cmdSetup = &cmdline.Command{
	Run:   runSetup,
	Name:  "setup",
	Short: "Set up the given veyron profiles",
	Long: `
To facilitate development across different platforms, veyron defines
platform-independent profiles that map different platforms to a set
of libraries and tools that can be used for a factor of veyron
development. The "setup" command can be used to install the libraries
and tools identified by the combination of the given profiles and
the host platform.
`,
	ArgsName: "<profiles>",
	ArgsLong: profilesDescription(),
}

func runSetup(cmd *cmdline.Command, args []string) error {
	root := os.Getenv("VEYRON_ROOT")
	if root == "" {
		cmd.Errorf("VEYRON_ROOT is not set.")
	}
	// Check that the profiles to be set up exist.
	for _, arg := range args {
		script := path.Join(root, "environment/scripts/setup", runtime.GOOS, arg, "setup.sh")
		if _, err := os.Lstat(script); err != nil {
			cmd.Errorf("Unknown profile '%s'", arg)
			return cmdline.ErrUsage
		}
	}
	// Setup the profiles.
	for _, arg := range args {
		script := path.Join(root, "environment/scripts/setup", runtime.GOOS, arg, "setup.sh")
		cmd := exec.Command(script)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return errors.New("profile setup failed")
		}
	}
	return nil
}
