package impl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"tools/lib/cmdline"
	"tools/lib/gitutil"
	"tools/lib/hgutil"
	"tools/lib/runutil"
	"tools/lib/util"
)

// cmdProject represents the 'project' command of the veyron tool.
var cmdProject = &cmdline.Command{
	Name:     "project",
	Short:    "Manage veyron projects",
	Long:     "Manage veyron projects.",
	Children: []*cmdline.Command{cmdProjectList, cmdProjectPoll, cmdProjectUpdate},
}

// cmdProjectList represents the 'list' sub-command of the
// 'project' command of the veyron tool.
var cmdProjectList = &cmdline.Command{
	Run:   runProjectList,
	Name:  "list",
	Short: "List existing veyron projects",
	Long:  "Inspect the local filesystem and list the existing projects.",
}

// runProjectList generates a human-readable description of
// existing projects.
func runProjectList(command *cmdline.Command, _ []string) error {
	run := runutil.New(verboseFlag, command.Stdout())
	git, hg := gitutil.New(run), hgutil.New(run)
	projects, err := util.LocalProjects(git, hg)
	if err != nil {
		return err
	}
	names := []string{}
	for name := range projects {
		names = append(names, name)
	}
	sort.Strings(names)
	description := fmt.Sprintf("Existing projects:\n")
	for _, name := range names {
		project := projects[name]
		description += fmt.Sprintf("  %q in %q\n", filepath.Base(name), project.Path)
		if branchesFlag {
			if err := os.Chdir(project.Path); err != nil {
				return fmt.Errorf("Chdir(%v) failed: %v", project.Path, err)
			}
			branches, current := []string{}, ""
			switch project.Protocol {
			case "git":
				branches, current, err = git.GetBranches()
				if err != nil {
					return err
				}
			case "hg":
				branches, current, err = hg.GetBranches()
				if err != nil {
					return err
				}
			default:
				return fmt.Errorf("unsupported protocol %v", project.Protocol)
			}
			for _, branch := range branches {
				if branch == current {
					description += fmt.Sprintf("    * %v\n", branch)
				} else {
					description += fmt.Sprintf("    %v\n", branch)
				}
			}
		}
	}
	fmt.Fprintf(command.Stdout(), "%s", description)
	return nil
}

// cmdProjectPoll represents the 'poll' sub-command of the 'project'
// command of the veyron tool.
var cmdProjectPoll = &cmdline.Command{
	Run:   runProjectPoll,
	Name:  "poll",
	Short: "Poll existing veyron projects",
	Long: `
Poll existing veyron projects and report whether any new changes exist.
`,
}

// runProjectPoll generates a description of the new changes.
func runProjectPoll(command *cmdline.Command, _ []string) error {
	run := runutil.New(verboseFlag, command.Stdout())
	git, hg := gitutil.New(run), hgutil.New(run)
	currentProjects, err := util.LocalProjects(git, hg)
	if err != nil {
		return err
	}
	newProjects, err := util.LatestProjects(manifestFlag, git)
	if err != nil {
		return err
	}
	allProjects := map[string]struct{}{}
	for project, _ := range currentProjects {
		allProjects[project] = struct{}{}
	}
	for project, _ := range newProjects {
		allProjects[project] = struct{}{}
	}
	ops, err := computeOperations(allProjects, currentProjects, newProjects)
	if err != nil {
		return err
	}
	update, err := computeUpdate(git, ops)
	if err != nil {
		return err
	}
	bytes, err := json.MarshalIndent(update, "", "  ")
	if err != nil {
		return fmt.Errorf("MarshalIndent() failed: %v", err)
	}
	fmt.Fprintf(command.Stdout(), "%s\n", bytes)
	return nil
}

func computeUpdate(git *gitutil.Git, ops operationList) (util.Update, error) {
	update := util.Update{}
	for _, op := range ops {
		cls := []util.CL{}
		if op.ty == updateOperation {
			if err := os.Chdir(op.destination); err != nil {
				return nil, fmt.Errorf("Chdir(%v) failed: %v", op.destination, err)
			}
			if err := git.Fetch("origin", "master"); err != nil {
				return nil, err
			}
			commitsText, err := git.Log("FETCH_HEAD", "master", "%an%n%ae%n%B")
			if err != nil {
				return nil, err
			}
			for _, commitText := range commitsText {
				if expected, got := 3, len(commitText); got < expected {
					return nil, fmt.Errorf("Unexpected length of %v: expected at least %v, got %v", commitText, expected, got)
				}
				cls = append(cls, util.CL{
					Author:      commitText[0],
					Email:       commitText[1],
					Description: strings.Join(commitText[2:], "\n"),
				})
			}
		}
		update[op.project.Name] = cls
	}
	return update, nil
}

// cmdProjectUpdate represents the 'update' sub-command of the 'project'
// command of the veyron tool.
var cmdProjectUpdate = &cmdline.Command{
	Run:   runProjectUpdate,
	Name:  "update",
	Short: "Update veyron projects",
	Long: `
Update the local projects to match the state of the remote projects
identified by a project manifest. The projects to be updated are
specified as a list of arguments. If no project is specified, the
default behavior is to update all projects.
`,
	ArgsName: "<projects>",
	ArgsLong: "<projects> is a list of projects to update.",
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
	// project holds information about the project such as its
	// name, local path, and the protocol it uses for version
	// control.
	project util.Project
	// destination is the new project path.
	destination string
	// source is the current project path.
	source string
	// ty is the type of the operation.
	ty operationType
}

// newOperation is the operation factory.
func newOperation(project util.Project, src, dst string, ty operationType) operation {
	return operation{
		project:     project,
		destination: dst,
		source:      src,
		ty:          ty,
	}
}

func (o operation) String() string {
	name := filepath.Base(o.project.Name)
	switch o.ty {
	case createOperation:
		return fmt.Sprintf("create project %q in %q", name, o.destination)
	case deleteOperation:
		return fmt.Sprintf("delete project %q from %q", name, o.source)
	case moveOperation:
		return fmt.Sprintf("move project %q from %q to %q and update it", name, o.source, o.destination)
	case updateOperation:
		return fmt.Sprintf("update project %q in %q", name, o.source)
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
	return ol[i].project.Name < ol[j].project.Name
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
func computeOperations(updateProjects map[string]struct{}, currentProjects, newProjects map[string]util.Project) (operationList, error) {
	result := operationList{}
	names := []string{}
	for name := range updateProjects {
		names = append(names, name)
	}
	for _, name := range names {
		if currentProject, ok := currentProjects[name]; ok {
			if newProject, ok := newProjects[name]; ok {
				if currentProject.Path == newProject.Path {
					result = append(result, newOperation(currentProject, currentProject.Path, newProject.Path, updateOperation))
				} else {
					result = append(result, newOperation(currentProject, currentProject.Path, newProject.Path, moveOperation))
				}
			} else if gcFlag {
				result = append(result, newOperation(currentProject, currentProject.Path, "", deleteOperation))
			}
		} else if newProject, ok := newProjects[name]; ok {
			result = append(result, newOperation(newProject, "", newProject.Path, createOperation))
		} else {
			return nil, fmt.Errorf("project %v does not exist", name)
		}
	}
	sort.Sort(result)
	return result, nil
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
func runOperation(run *runutil.Run, git *gitutil.Git, hg *hgutil.Hg, op operation) error {
	switch op.ty {
	case createOperation:
		path, perm := filepath.Dir(op.destination), os.FileMode(0755)
		if err := os.MkdirAll(path, perm); err != nil {
			return fmt.Errorf("MkdirAll(%v, %v) failed: %v", path, perm, err)
		}
		switch op.project.Protocol {
		case "git":
			if err := git.Clone(op.project.Name, op.destination); err != nil {
				return err
			}
			if strings.HasPrefix(op.project.Name, "https://veyron.googlesource.com/") {
				// Setup the repository for Gerrit code reviews.
				file := filepath.Join(op.destination, ".git", "hooks", "commit-msg")
				url := "https://gerrit-review.googlesource.com/tools/hooks/commit-msg"
				args := []string{"-Lo", file, url}
				var stderr bytes.Buffer
				if err := run.Command(ioutil.Discard, &stderr, nil, "curl", args...); err != nil {
					return fmt.Errorf("failed to download commit message hook: %v\n%v", err, stderr.String())
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
			}
		case "hg":
			if err := hg.Clone(op.project.Name, op.destination); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported protocol %v", op.project.Protocol)
		}
	case deleteOperation:
		if err := os.RemoveAll(op.source); err != nil {
			return fmt.Errorf("RemoveAll(%v) failed: %v", op.source, err)
		}
	case moveOperation:
		path, perm := filepath.Dir(op.destination), os.FileMode(0755)
		if err := os.MkdirAll(path, perm); err != nil {
			return fmt.Errorf("MkdirAll(%v, %v) failed: %v", path, perm, err)
		}
		if err := os.Rename(op.source, op.destination); err != nil {
			return fmt.Errorf("Rename(%v, %v) failed: %v", op.source, op.destination, err)
		}
		if err := util.UpdateProject(op.destination, git, hg); err != nil {
			return err
		}
	case updateOperation:
		if err := util.UpdateProject(op.destination, git, hg); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%v", op)
	}
	return nil
}

// runProjectUpdate implements the update command of the veyron tool.
func runProjectUpdate(command *cmdline.Command, args []string) error {
	run := runutil.New(verboseFlag, command.Stdout())
	git, hg := gitutil.New(run), hgutil.New(run)
	currentProjects, err := util.LocalProjects(git, hg)
	if err != nil {
		return err
	}
	newProjects, err := util.LatestProjects(manifestFlag, git)
	if err != nil {
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
			return command.UsageErrorf("project %v does not exist", arg)
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
		runFn := func() error { return runOperation(run, git, hg, op) }
		// Always log the output of 'veyron project update'
		// irrespective of the value of the verbose flag.
		run := runutil.New(true, command.Stdout())
		if err := run.Function(runFn, "%v", op); err != nil {
			fmt.Fprintf(command.Stderr(), "%v\n", err)
			failed = true
		}
	}
	if failed {
		return cmdline.ErrExitCode(2)
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
