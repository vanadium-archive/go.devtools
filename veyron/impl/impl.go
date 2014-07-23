package impl

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	"tools/lib/cmd"
	"tools/lib/cmdline"
	"tools/lib/git"
)

const (
	ROOT_ENV = "VEYRON_ROOT"
)

var (
	root = func() string {
		result := os.Getenv(ROOT_ENV)
		if result == "" {
			panic(fmt.Sprintf("%v is not set", ROOT_ENV))
		}
		return result
	}()
	verbose bool
)

func init() {
	cmdRoot.Flags.BoolVar(&verbose, "v", false, "Print verbose output.")
}

var cmdRoot = &cmdline.Command{
	Name:  "veyron",
	Short: "Command-line tool for managing the veyron project",
	Long: `
The veyron tool facilitates interaction with the veyron project.
In particular, it can be used to install different veyron profiles.
`,
	Children: []*cmdline.Command{cmdSelfUpdate, cmdSetup, cmdUpdate, cmdVersion},
}

// Root returns a command that represents the root of the veyron tool.
func Root() *cmdline.Command {
	return cmdRoot
}

// cmdSelfUpdate represents the 'selfupdate' command of the veyron
// tool.
var cmdSelfUpdate = &cmdline.Command{
	Run:   runSelfUpdate,
	Name:  "selfupdate",
	Short: "Update the veyron tool",
	Long:  "Download and install the latest version of the veyron tool.",
}

func runSelfUpdate(command *cmdline.Command, args []string) error {
	cmd.SetVerbose(verbose)
	return git.SelfUpdate("veyron")
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

func profilesDescription() string {
	result := "<profiles> is a list of profiles to set up. Supported profiles are:\n"
	dir := filepath.Join(root, "environment/scripts/setup", runtime.GOOS)
	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		panic(fmt.Sprintf("could not read %s", dir))
	}
	for _, entry := range entries {
		file := filepath.Join(dir, entry.Name(), "DESCRIPTION")
		description, err := ioutil.ReadFile(file)
		if err != nil {
			panic(fmt.Sprintf("could not read %s", file))
		}
		result += fmt.Sprintf("  %s: %s", entry.Name(), string(description))
	}
	return result
}

func runSetup(command *cmdline.Command, args []string) error {
	cmd.SetVerbose(verbose)
	// Check that the profiles to be set up exist.
	for _, arg := range args {
		script := filepath.Join(root, "environment/scripts/setup", runtime.GOOS, arg, "setup.sh")
		if _, err := os.Lstat(script); err != nil {
			return command.Errorf("profile %v does not exist", arg)
		}
	}
	// Setup the profiles.
	for _, arg := range args {
		script := filepath.Join(root, "environment/scripts/setup", runtime.GOOS, arg, "setup.sh")
		if _, err := cmd.RunErrorOutput(script); err != nil {
			return fmt.Errorf("profile %v setup failed: %v", arg, err)
		}
	}
	return nil
}

// cmdUpdate represents the 'update' command of the veyron tool.
var cmdUpdate = &cmdline.Command{
	Run:   runUpdate,
	Name:  "update",
	Short: "Update local veyron repositories",
	Long: `
Update the local master branch of veyron git repositories by pulling
from the remote master. The repositories to be updated are specified
as a list of arguments. If no repositories are specified, the default
behavior is to update all repositories.
`,
	ArgsName: "<repos>",
	ArgsLong: reposDescription(),
}

type project struct {
	Name string `xml:"name,attr"`
	Path string `xml:"path,attr"`
}

type manifest struct {
	Projects []project `xml:"project"`
}

func reposDescription() string {
	result := "<repos> is a list of repositories to update. Existing repositories are:\n"
	path := filepath.Join(root, ".repo", "manifest.xml")
	data, err := ioutil.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("ReadFile(%v) failed: %v", path, err))
	}
	var m manifest
	if err := xml.Unmarshal(data, &m); err != nil {
		panic(fmt.Sprintf("Unmarshal() failed: %v", err))
	}
	for _, project := range m.Projects {
		result += fmt.Sprintf("   %s (located in %s)\n", project.Name, filepath.Join(root, project.Path))
	}
	return result
}

func runUpdate(command *cmdline.Command, args []string) error {
	cmd.SetVerbose(verbose)
	path := filepath.Join(root, ".repo", "manifest.xml")
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("ReadFile(%v) failed: %v", path, err)
	}
	var m manifest
	if err := xml.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("Unmarshal() failed: %v", err)
	}
	projects := make(map[string]string)
	for _, project := range m.Projects {
		projects[project.Name] = projects[project.Path]
	}
	if len(args) == 0 {
		// The default behavior is to update all repositories.
		for name, _ := range projects {
			args = append(args, name)
		}
	}
	// Check that the repositories to be updated exist.
	for _, arg := range args {
		if _, ok := projects[arg]; !ok {
			command.Errorf("repository %v does not exist", arg)
			return cmdline.ErrUsage
		}
	}
	// Update the repositories.
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer os.Chdir(wd)
	for _, arg := range args {
		path := projects[arg]
		if err := updateRepository(path); err != nil {
			return err
		}
	}
	return nil
}

func updateRepository(repo string) error {
	os.Chdir(repo)
	branch, err := git.CurrentBranchName()
	if err != nil {
		return err
	}
	stashed, err := git.Stash()
	if err != nil {
		return err
	}
	if stashed {
		defer git.StashPop()
	}
	if err := git.CheckoutBranch("master"); err != nil {
		return err
	}
	defer git.CheckoutBranch(branch)
	if err := git.Pull("origin", "master"); err != nil {
		return err
	}
	return nil
}

// cmdVersion represent the 'version' command of the veyron tool.
var cmdVersion = &cmdline.Command{
	Run:   runVersion,
	Name:  "version",
	Short: "Print version",
	Long:  "Print version of the veyron tool.",
}

const version string = "0.3.0"

// commitId should be over-written during build:
// go build -ldflags "-X tools/veyron/impl.commitId <commitId>" tools/veyron
var commitId string = "test-build"

func runVersion(cmd *cmdline.Command, args []string) error {
	fmt.Printf("veyron tool version %v (build %v)\n", version, commitId)
	return nil
}
