package impl

import (
	"encoding/xml"
	"fmt"
	"go/build"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"tools/lib/cmd"
	"tools/lib/cmdline"
	"tools/lib/git"

	"veyron2/rt"
	vbuild "veyron2/services/mgmt/build"
)

const (
	ROOT_ENV = "VEYRON_ROOT"
)

var (
	arch, opsys string
	root        = func() string {
		result := os.Getenv(ROOT_ENV)
		if result == "" {
			panic(fmt.Sprintf("%v is not set", ROOT_ENV))
		}
		return result
	}()
	verbose bool
)

func init() {
	cmdBuild.Flags.StringVar(&arch, "arch", runtime.GOARCH, "Target architecture.")
	cmdBuild.Flags.StringVar(&opsys, "os", runtime.GOOS, "Target operating system.")
	cmdRoot.Flags.BoolVar(&verbose, "v", false, "Print verbose output.")
}

var cmdRoot = &cmdline.Command{
	Name:  "veyron",
	Short: "Command-line tool for managing the veyron project",
	Long: `
The veyron tool facilitates interaction with the veyron project.
In particular, it can be used to install different veyron profiles.
`,
	Children: []*cmdline.Command{cmdBuild, cmdSelfUpdate, cmdSetup, cmdUpdate, cmdVersion},
}

// Root returns a command that represents the root of the veyron tool.
func Root() *cmdline.Command {
	return cmdRoot
}

var cmdBuild = &cmdline.Command{
	Run:   runBuild,
	Name:  "build",
	Short: "Build veyron Go packages",
	Long: `
Build veyron Go packages using a remote build server. The command
collects all source code files that are not part of the Go standard
library that the target packages depend on, sends them to a build
server, and receives the built binaries.
`,
	ArgsName: "<name> <packages>",
	ArgsLong: `
<name> is a veyron object name of a build server
<packages> is a list of packages to build
`,
}

func importPackages(path string, pkgMap map[string]*build.Package) error {
	if _, ok := pkgMap[path]; ok {
		return nil
	}
	srcDir, mode := "", build.ImportMode(0)
	pkg, err := build.Import(path, srcDir, mode)
	if err != nil {
		return fmt.Errorf("Import(%q,%q,%v) failed: %v", path, srcDir, mode, err)
	}
	if pkg.Goroot {
		return nil
	}
	pkgMap[path] = pkg
	for _, path := range pkg.Imports {
		if err := importPackages(path, pkgMap); err != nil {
			return err
		}
	}
	return nil
}

// TODO(jsimsa): Avoid reading all source files into memory at the
// same time by returning a channel that can be used to bring source
// code files into memory one by one when streaming them to the build
// server.
func getSources(pkgMap map[string]*build.Package) ([]vbuild.File, error) {
	result := []vbuild.File{}
	for _, pkg := range pkgMap {
		for _, files := range [][]string{pkg.GoFiles, pkg.SFiles} {
			for _, file := range files {
				path := filepath.Join(pkg.Dir, file)
				bytes, err := ioutil.ReadFile(path)
				if err != nil {
					return nil, fmt.Errorf("ReadFile(%v) failed: %v", path, err)
				}
				result = append(result, vbuild.File{Contents: bytes, Name: filepath.Join(pkg.ImportPath, file)})
			}
		}
	}
	return result, nil
}

func invokeBuild(name string, files []vbuild.File) ([]byte, []vbuild.File, error) {
	rt.Init()
	client, err := vbuild.BindBuild(name)
	if err != nil {
		return nil, nil, fmt.Errorf("BindBuild(%v) failed: %v", name, err)
	}
	stream, err := client.Build(rt.R().NewContext(), vbuild.Architecture(arch), vbuild.OperatingSystem(opsys))
	if err != nil {
		return nil, nil, fmt.Errorf("Build() failed: %v", err)
	}
	for _, file := range files {
		if err := stream.Send(file); err != nil {
			stream.Cancel()
			return nil, nil, fmt.Errorf("Send() failed: %v", err)
		}
	}
	if err := stream.CloseSend(); err != nil {
		return nil, nil, fmt.Errorf("CloseSend() failed: %v", err)
	}
	bins := []vbuild.File{}
	for stream.Advance() {
		bins = append(bins, stream.Value())
	}
	if err := stream.Err(); err != nil {
		return nil, nil, fmt.Errorf("Advance() failed: %v", err)
	}
	output, err := stream.Finish()
	if err != nil {
		return nil, nil, fmt.Errorf("Finish() failed: %v", err)
	}
	return output, bins, nil
}

func runBuild(command *cmdline.Command, args []string) error {
	name, path := args[0], args[1]
	pkgMap := map[string]*build.Package{}
	if err := importPackages(path, pkgMap); err != nil {
		return err
	}
	files, err := getSources(pkgMap)
	if err != nil {
		return err
	}
	_, bins, err := invokeBuild(name, files)
	if err != nil {
		return err
	}
	if expected, got := 1, len(bins); expected != got {
		return fmt.Errorf("Unexpected number of binaries: expected %v, got %v", expected, got)
	}
	pkg, _ := pkgMap[path]
	binPath, perm := filepath.Join(pkg.BinDir, filepath.Base(pkg.Dir)), os.FileMode(0755)
	fmt.Printf("Generated binary %v\n", binPath)
	if err := ioutil.WriteFile(binPath, bins[0].Contents, perm); err != nil {
		return fmt.Errorf("WriteFile(%v, %v) failed: %v", binPath, perm, err)
	}
	return nil
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
	git := git.New(verbose)
	tool := "veyron"
	return cmd.Log(fmt.Sprintf("Updating tool %q", tool), func() error { return git.SelfUpdate(tool) })
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
		cmd.LogStart(fmt.Sprintf("Setting up profile %q", arg))
		if _, errOut, err := cmd.RunOutput(true, script); err != nil {
			cmd.LogEnd(false)
			return fmt.Errorf("profile %q setup failed with\n%v", arg, strings.Join(errOut, "\n"))
		}
		cmd.LogEnd(true)
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
	git := git.New(verbose)
	path := filepath.Join(root, ".repo", "manifest.xml")
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("ReadFile(%v) failed: %v", path, err)
	}
	var m manifest
	if err := xml.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("Unmarshal() failed: %v", err)
	}
	projects := map[string]string{}
	for _, project := range m.Projects {
		projects[project.Name] = project.Path
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
		if err := cmd.Log(fmt.Sprintf("Updating repository %q", path),
			func() error { return updateRepository(filepath.Join(root, path), git) }); err != nil {
			return err
		}
	}
	return nil
}

func updateRepository(repo string, git *git.Git) error {
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
