package impl

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"sort"

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
		Children: []*cmdline.Command{cmdSetup},
	}
}

var (
	profiles = map[string]string{
		"android":           "Android veyron development",
		"cross-compilation": "cross-compilation for Linux/ARM",
		"developer":         "core veyron development",
	}
)

func profilesDescription() string {
	result := `
<profiles> is a list of profiles to set up. Currently, the veyron tool
supports the following profiles:
`
	sortedProfiles := make([]string, 0)
	maxLength := 0
	for profile, _ := range profiles {
		sortedProfiles = append(sortedProfiles, profile)
		if len(profile) > maxLength {
			maxLength = len(profile)
		}
	}
	sort.Strings(sortedProfiles)
	for _, profile := range sortedProfiles {
		result += fmt.Sprintf("  %*s: %s\n", maxLength, profile, profiles[profile])
	}
	return result
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
	// Check that the profiles to be set up exist.
	for _, arg := range args {
		if _, ok := profiles[arg]; !ok {
			cmd.Errorf("Unknown profile '%s'", arg)
			return cmdline.ErrUsage
		}
	}
	// Setup the profiles.
	root := os.Getenv("VEYRON_ROOT")
	script := path.Join(root, "environment/scripts/setup/machine/init.sh")
	for _, arg := range args {
		checkpoints := path.Join(root, ".checkpoints", arg)
		if err := os.Setenv("CHECKPOINT_DIR", checkpoints); err != nil {
			return errors.New("checkpoint setup failed")
		}
		if err := os.MkdirAll(checkpoints, 0777); err != nil {
			return errors.New("checkpoint setup failed")
		}
		cmd := exec.Command(script, "-p", arg)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return errors.New("profile setup failed")
		}
		if err := os.RemoveAll(checkpoints); err != nil {
			return errors.New("checkpoint setup failed")
		}
	}
	return nil
}
