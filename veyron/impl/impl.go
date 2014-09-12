package impl

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"tools/lib/cmd"
	"tools/lib/cmdline"
	"tools/lib/git"
	"tools/lib/tool"
)

const (
	ROOT_ENV = "VEYRON_ROOT"
)

var (
	gcFlag       bool
	manifestFlag string
	verboseFlag  bool
)

var (
	root = func() string {
		result := os.Getenv(ROOT_ENV)
		if result == "" {
			panic(fmt.Sprintf("%v is not set", ROOT_ENV))
		}
		return result
	}()
)

func init() {
	cmdRoot.Flags.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
	cmdSelfUpdate.Flags.StringVar(&manifestFlag, "manifest", "absolute", "Name of the project manifest.")
	cmdProjectUpdate.Flags.StringVar(&manifestFlag, "manifest", "absolute", "Name of the project manifest.")
	cmdProjectUpdate.Flags.BoolVar(&gcFlag, "gc", false, "Garbage collect obsolete repositories.")
}

// Root returns a command that represents the root of the veyron tool.
func Root() *cmdline.Command {
	return cmdRoot
}

// cmdRoot represents the root of the veyron tool.
var cmdRoot = &cmdline.Command{
	Name:     "veyron",
	Short:    "Command-line tool for managing veyron projects",
	Long:     "The veyron tool facilitates interaction with veyron projects.",
	Children: []*cmdline.Command{cmdProfile, cmdProject, cmdSelfUpdate, cmdVersion},
}

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
	Children: []*cmdline.Command{cmdProfileDescribe, cmdProfileSetup},
}

// cmdProfileDescribe represents the 'describe' sub-command of the
// 'profile' command of the veyron tool.
var cmdProfileDescribe = &cmdline.Command{
	Run:   runProfileDescribe,
	Name:  "describe",
	Short: "Describe supported veyron profiles",
	Long:  "Inspect the host platform and enumerate supported profiles.",
}

func runProfileDescribe(*cmdline.Command, []string) error {
	dir := filepath.Join(root, "environment/scripts/setup", runtime.GOOS)
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
	fmt.Printf("%s", description)
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

// cmdProject represents the 'project' command of the veyron tool.
var cmdProject = &cmdline.Command{
	Name:     "project",
	Short:    "Manage veyron projects",
	Long:     "Manage veyron projects.",
	Children: []*cmdline.Command{cmdProjectDescribe, cmdProjectUpdate},
}

// cmdProjectDescribe represents the 'describe' sub-command of the
// 'project' command of the veyron tool.
var cmdProjectDescribe = &cmdline.Command{
	Run:   runProjectDescribe,
	Name:  "describe",
	Short: "Describe supported veyron profiles",
	Long:  "Inspect the local filesystem and enumerate existing projects.",
}

// runProjectDescribe generates a human-readable description of
// existing projects.
func runProjectDescribe(*cmdline.Command, []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer os.Chdir(wd)
	git := git.New(verboseFlag)
	projects := map[string]string{}
	if err := findCurrentProjects(root, projects, git); err != nil {
		return fmt.Errorf("%v", err)
	}
	names := []string{}
	for name := range projects {
		names = append(names, name)
	}
	sort.Strings(names)
	description := fmt.Sprintf("Existing projects:\n")
	for _, name := range names {
		description += fmt.Sprintf("   %v (located in %v)\n", name, projects[name])
	}
	fmt.Printf("%s", description)
	return nil
}

// cmdProjectUpdate represents the 'update' sub-command of the 'project'
// command of the veyron tool.
var cmdProjectUpdate = &cmdline.Command{
	Run:   runProjectUpdate,
	Name:  "update",
	Short: "Update veyron projects",
	Long: `
Update the local master branch of veyron projects by pulling from
the remote master. The projects to be updated are specified as a list
of arguments. If no project is specified, the default behavior is to
update all projects.
`,
	ArgsName: "<projects>",
	ArgsLong: "<projects> is a list of projects to update.",
}

type project struct {
	Name string `xml:"name,attr"`
	Path string `xml:"path,attr"`
}

type manifest struct {
	Projects []project `xml:"project"`
}

type operationType int

const (
	// The order in which operation types are defined determines
	// the order in which operations are performed. For
	// correctness, the delete operations should happen before
	// move operations, which should happen before create
	// operations.
	deleteOperation operationType = iota
	moveOperation
	createOperation
	updateOperation
)

// operation represents a project operation.
type operation struct {
	// destination is the new project path.
	destination string
	// project is the name of the project.
	project string
	// source is the current project path.
	source string
	// ty is the type of the operation.
	ty operationType
}

// newOperation is the operation factory.
func newOperation(project, src, dst string, ty operationType) operation {
	return operation{
		destination: dst,
		source:      src,
		project:     project,
		ty:          ty,
	}
}

func (o operation) String() string {
	switch o.ty {
	case createOperation:
		return fmt.Sprintf("create project %v in %q", o.project, o.destination)
	case deleteOperation:
		return fmt.Sprintf("delete project %v from %q", o.project, o.source)
	case moveOperation:
		return fmt.Sprintf("move project %v from %q to %q and update it", o.project, o.source, o.destination)
	case updateOperation:
		return fmt.Sprintf("update project %v in %q", o.project, o.source)
	default:
		return fmt.Sprintf("unknown operation type: %v", o.ty)
	}
}

// operationList is a collection used for sorting operations.
type operationList []operation

// Len returns the length of the collection.
func (ol operationList) Len() int {
	return len(ol)
}

// Less defines the order of operations. Operations are ordered first
// by their type and then by their project name.
func (ol operationList) Less(i, j int) bool {
	if ol[i].ty != ol[j].ty {
		return ol[i].ty < ol[j].ty
	}
	return ol[i].project < ol[j].project
}

// Swap swaps two elements of the collection.
func (ol operationList) Swap(i, j int) {
	ol[i], ol[j] = ol[j], ol[i]
}

// computeOperations inputs a set of projects to update and the set of
// current and new projects (as defined by contents of the local file
// system and manifest file respectively) and outputs a collection of
// operations that describe the actions needed to update the target
// projects.
func computeOperations(updateProjects map[string]struct{}, currentProjects, newProjects map[string]string) (operationList, error) {
	result := operationList{}
	names := []string{}
	for name := range updateProjects {
		names = append(names, name)
	}
	for _, name := range names {
		if currentPath, ok := currentProjects[name]; ok {
			if newPath, ok := newProjects[name]; ok {
				if currentPath == newPath {
					result = append(result, newOperation(name, currentPath, newPath, updateOperation))
				} else {
					result = append(result, newOperation(name, currentPath, newPath, moveOperation))
				}
			} else if gcFlag {
				result = append(result, newOperation(name, currentPath, "", deleteOperation))
			}
		} else if newPath, ok := newProjects[name]; ok {
			result = append(result, newOperation(name, "", newPath, createOperation))
		} else {
			return nil, fmt.Errorf("project %v does not exist", name)
		}
	}
	sort.Sort(result)
	return result, nil
}

// findCurrentProjects scans the VEYRON_ROOT folder to identify
// existing projects.
func findCurrentProjects(path string, projects map[string]string, git *git.Git) error {
	if err := os.Chdir(path); err != nil {
		return fmt.Errorf("Chdir(%v) failed: %v", path, err)
	}
	name, err := git.RepoName()
	if err == nil {
		if existingPath, ok := projects[name]; ok {
			return fmt.Errorf("name conflict: both %v and %v contain the project %v", existingPath, path, name)
		}
		projects[name] = path
		return nil
	}
	ignoreSet, ignorePath := make(map[string]struct{}, 0), filepath.Join(path, ".veyronignore")
	file, err := os.Open(ignorePath)
	if err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			ignoreSet[scanner.Text()] = struct{}{}
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("Scan() failed: %v", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("Open(%v) failed: %v", ignorePath, err)
	}
	fis, err := ioutil.ReadDir(path)
	if err != nil {
		return fmt.Errorf("ReadDir(%v) failed: %v", path, err)
	}
	for _, fi := range fis {
		if _, ignore := ignoreSet[fi.Name()]; fi.IsDir() && !strings.HasPrefix(fi.Name(), ".") && !ignore {
			if err := findCurrentProjects(filepath.Join(path, fi.Name()), projects, git); err != nil {
				return err
			}
		}
	}
	return nil
}

// findNewProjects parses the most recent version fo the project
// manifest to identify the latest state of the project universe.
func findNewProjects(projects map[string]string, git *git.Git) error {
	// Update the manifest.
	path := filepath.Join(root, ".manifest")
	if err := updateProject(path, git); err != nil {
		return err
	}
	// Parse the manifest.
	path = filepath.Join(root, ".manifest", manifestFlag+".xml")
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("ReadFile(%v) failed: %v", path, err)
	}
	var m manifest
	if err := xml.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("Unmarshal(%v) failed: %v", string(data), err)
	}
	for _, project := range m.Projects {
		projects[project.Name] = filepath.Join(root, project.Path)
	}
	return nil
}

// preCommitHook is a git hook installed to all new projects. It
// prevents accidental commits to the local master branch.
const preCommitHook = `
#!/bin/bash

# Get the current branch name.
readonly BRANCH=$(git rev-parse --abbrev-ref HEAD)

if [ "${BRANCH}" == "master" ]
then
  echo "========================================================================="
  echo "Veyron code cannot be committed to master using the 'git commit' command."
  echo "Please make a feature branch and commit your code there."
  echo "========================================================================="
  exit 1
fi

exit 0
`

// prePushHook is a git hook installed to all new projects. It
// prevents accidental pushes to the remote master branch.
const prePushHook = `
#!/bin/bash

readonly REMOTE=$1

# Get the current branch name.
readonly BRANCH=$(git rev-parse --abbrev-ref HEAD)

if [ "${REMOTE}" == "origin" ] && [ "${BRANCH}" == "master" ]
then
  echo "======================================================================="
  echo "Veyron code cannot be pushed to master using the 'git push' command."
  echo "Use the 'git veyron review' command to follow the code review workflow."
  echo "======================================================================="
  exit 1
fi

exit 0
`

// runOperation executes the given operation.
//
// TODO(jsimsa): Decide what to do in case we would want to update the
// commit hooks for existing repositories. Overwriting the existing
// hooks is not a good idea as developers might have customized the
// hooks.
func runOperation(op operation, git *git.Git) error {
	switch op.ty {
	case createOperation:
		path, perm := filepath.Dir(op.destination), os.FileMode(0700)
		if err := os.MkdirAll(path, perm); err != nil {
			return fmt.Errorf("MkdirAll(%v, %v) failed: %v", path, perm, err)
		}
		if err := git.Clone(op.project, op.destination); err != nil {
			return err
		}
		file := filepath.Join(op.destination, ".git", "hooks", "commit-msg")
		url := "https://gerrit-review.googlesource.com/tools/hooks/commit-msg"
		args := []string{"-Lo", file, url}
		if _, errOut, err := cmd.RunOutput(false, "curl", args...); err != nil {
			return fmt.Errorf("download of Gerrit commit message git hook failed\n%s", strings.Join(errOut, "\n"))
		}
		if err := os.Chmod(file, perm); err != nil {
			return fmt.Errorf("Chmod(%v, %v) failed: %v", file, perm, err)
		}
		file = filepath.Join(op.destination, ".git", "hooks", "pre-commit")
		if err := ioutil.WriteFile(file, []byte(preCommitHook), perm); err != nil {
			return fmt.Errorf("WriteFile(%v, %v) failed: %v", file, perm, err)
		}
		file = filepath.Join(op.destination, ".git", "hooks", "pre-push")
		if err := ioutil.WriteFile(file, []byte(prePushHook), perm); err != nil {
			return fmt.Errorf("WriteFile(%v, %v) failed: %v", file, perm, err)
		}
	case deleteOperation:
		if err := os.RemoveAll(op.source); err != nil {
			return fmt.Errorf("RemoveAll(%v) failed: %v", op.source, err)
		}
	case moveOperation:
		path, perm := filepath.Dir(op.destination), os.FileMode(0700)
		if err := os.MkdirAll(path, perm); err != nil {
			return fmt.Errorf("MkdirAll(%v, %v) failed: %v", path, perm, err)
		}
		if err := os.Rename(op.source, op.destination); err != nil {
			return fmt.Errorf("Rename(%v, %v) failed: %v", op.source, op.destination, err)
		}
		if err := updateProject(op.destination, git); err != nil {
			return err
		}
	case updateOperation:
		if err := updateProject(op.destination, git); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%v", op)
	}
	return nil
}

// runProjectUpdate implements the update command of the veyron tool.
func runProjectUpdate(command *cmdline.Command, args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer os.Chdir(wd)
	git := git.New(verboseFlag)
	currentProjects := map[string]string{}
	if err := findCurrentProjects(root, currentProjects, git); err != nil {
		return err
	}
	newProjects := map[string]string{}
	if err := findNewProjects(newProjects, git); err != nil {
		return err
	}
	allProjects := map[string]struct{}{}
	for project, _ := range currentProjects {
		allProjects[project] = struct{}{}
	}
	for project, _ := range newProjects {
		allProjects[project] = struct{}{}
	}
	updateProjects := map[string]struct{}{}
	for _, arg := range args {
		if _, ok := allProjects[arg]; !ok {
			command.Errorf("project %v does not exist", arg)
			return cmdline.ErrUsage
		}
		updateProjects[arg] = struct{}{}
	}
	// If no projects were specified, the default behavior is to
	// update all projects.
	if len(updateProjects) == 0 {
		updateProjects = allProjects
	}
	ops, err := computeOperations(updateProjects, currentProjects, newProjects)
	if err != nil {
		return err
	}
	if err := testOperations(ops); err != nil {
		return err
	}
	failed := false
	for _, op := range ops {
		runFn := func() error { return runOperation(op, git) }
		if err := cmd.Log(runFn, "%v", op); err != nil {
			fmt.Fprintf(command.Stderr(), "%v\n", err)
			failed = true
		}
	}
	if failed {
		os.Exit(2)
	}
	return nil
}

// testOperations checks if the target set of operations can be
// carried out given the current state of the local file system.
func testOperations(ops operationList) error {
	for _, op := range ops {
		switch op.ty {
		case createOperation:
			// Check the local file system.
			if _, err := os.Stat(op.destination); err != nil {
				if !os.IsNotExist(err) {
					return fmt.Errorf("Stat(%v) failed: %v", op.destination, err)
				}
			} else {
				return fmt.Errorf("cannot create %q as it already exists", op.destination)
			}
		case deleteOperation:
			if _, err := os.Stat(op.source); err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("cannot delete %q as it does not exist", op.destination)
				}
				return fmt.Errorf("Stat(%v) failed: %v", op.destination, err)
			}
		case moveOperation:
			if _, err := os.Stat(op.source); err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("cannot move %q to %q as the source does not exist", op.source, op.destination)
				}
				return fmt.Errorf("Stat(%v) failed: %v", op.destination, err)
			}
			if _, err := os.Stat(op.destination); err != nil {
				if !os.IsNotExist(err) {
					return fmt.Errorf("Stat(%v) failed: %v", op.destination, err)
				}
			} else {
				return fmt.Errorf("cannot move %q to %q as the destination already exists", op.source, op.destination)
			}
		case updateOperation:
			continue
		default:
			return fmt.Errorf("%v", op)
		}
	}
	return nil
}

// updateProject advances the local master branch of the project
// identified by the given path.
func updateProject(project string, git *git.Git) error {
	if err := os.Chdir(project); err != nil {
		return fmt.Errorf("Chdir(%v) failed: %v", project, err)
	}
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
	if err := git.Fetch(); err != nil {
		return err
	}
	if err := git.Merge("FETCH_HEAD", false); err != nil {
		return err
	}
	return nil
}

// cmdSelfUpdate represents the 'selfupdate' command of the veyron tool.
var cmdSelfUpdate = &cmdline.Command{
	Run:   runSelfUpdate,
	Name:  "selfupdate",
	Short: "Update the veyron tool",
	Long:  "Download and install the latest version of the veyron tool.",
}

func runSelfUpdate(command *cmdline.Command, args []string) error {
	return tool.SelfUpdate(verboseFlag, manifestFlag, "veyron")
}

// cmdVersion represents the 'version' command of the veyron tool.
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
