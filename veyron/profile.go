package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	"tools/lib/cmdline"
	"tools/lib/runutil"
	"tools/lib/util"
)

// cmdProfile represents the 'profile' command of the veyron tool.
var cmdProfile = &cmdline.Command{
	Name:  "profile",
	Short: "Manage veyron profiles",
	Long: `
To facilitate development across different platforms, veyron defines
platform-independent profiles that map different platforms to a set
of libraries and tools that can be used for a factor of veyron
development.
`,
	Children: []*cmdline.Command{cmdProfileList, cmdProfileSetup},
}

// cmdProfileList represents the 'list' sub-command of the
// 'profile' command of the veyron tool.
var cmdProfileList = &cmdline.Command{
	Run:   runProfileList,
	Name:  "list",
	Short: "List supported veyron profiles",
	Long:  "Inspect the host platform and list supported profiles.",
}

func runProfileList(command *cmdline.Command, _ []string) error {
	root, err := util.VeyronRoot()
	if err != nil {
		return err
	}
	dir := filepath.Join(root, "scripts", "setup", runtime.GOOS)
	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("could not read %s", dir)
	}
	description := fmt.Sprintf("Supported profiles:\n")
	for _, entry := range entries {
		file := filepath.Join(dir, entry.Name(), "DESCRIPTION")
		bytes, err := ioutil.ReadFile(file)
		if err != nil {
			return fmt.Errorf("could not read %s", file)
		}
		description += fmt.Sprintf("  %s: %s", entry.Name(), string(bytes))
	}
	fmt.Fprintf(command.Stdout(), "%s", description)
	return nil
}

// cmdProfileSetup represents the 'setup' sub-command of the 'profile'
// command of the veyron tool.
var cmdProfileSetup = &cmdline.Command{
	Run:      runProfileSetup,
	Name:     "setup",
	Short:    "Set up the given veyron profiles",
	Long:     "Set up the given veyron profiles.",
	ArgsName: "<profiles>",
	ArgsLong: "<profiles> is a list of profiles to set up.",
}

func runProfileSetup(command *cmdline.Command, args []string) error {
	root, err := util.VeyronRoot()
	if err != nil {
		return err
	}
	// Check that the profiles to be set up exist.
	for _, arg := range args {
		script := filepath.Join(root, "scripts", "setup", runtime.GOOS, arg, "setup.sh")
		if _, err := os.Lstat(script); err != nil {
			return command.UsageErrorf("profile %v does not exist", arg)
		}
	}
	// Always log the output of 'veyron profile setup'
	// irrespective of the value of the verbose flag.
	run := runutil.New(true, command.Stdout())
	// Setup the profiles.
	for _, arg := range args {
		script := filepath.Join(root, "scripts", "setup", runtime.GOOS, arg, "setup.sh")
		setupFn := func() error {
			var stderr bytes.Buffer
			if err := run.Command(ioutil.Discard, &stderr, nil, script); err != nil {
				return fmt.Errorf("profile %q setup failed: %v\n%v", arg, err, stderr.String())
			}
			return nil
		}
		if err := run.Function(setupFn, "Setting up profile "+arg); err != nil {
			return err
		}
	}
	return nil
}
