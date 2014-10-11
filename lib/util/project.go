// Package util provides utility functions for veyron tools.
//
// TODO(jsimsa): Create a repoutil package that hides different
// version control systems behind a single interface.
package util

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"tools/lib/envutil"
	"tools/lib/gitutil"
	"tools/lib/hgutil"
	"tools/lib/runutil"
)

// Update represents an update of veyron projects as a map from
// project names to a collections of commits.
type Update map[string][]CL

// CL represents a changelist.
type CL struct {
	// Author identifies the author of the changelist.
	Author string
	// Email identifies the author's email.
	Email string
	// Description holds the description of the changelist.
	Description string
}

// Manifest represents a collection of veyron projects.
type Manifest struct {
	Projects []Project `xml:"project"`
}

// Project represents a veyron project.
type Project struct {
	// Name is the URL at which the project is hosted.
	Name string `xml:"name,attr"`
	// Path is the relative path used to store the project locally.
	Path string `xml:"path,attr"`
	// Protocol is the version control protocol used by the project.
	Protocol string `xml:"protocol,attr"`
}

// LatestProjects parses the most recent version of the project
// manifest to identify the latest projects.
func LatestProjects(manifest string, git *gitutil.Git) (map[string]Project, error) {
	projects := map[string]Project{}
	if err := findLatestProjects(manifest, projects, git); err != nil {
		return nil, err
	}
	return projects, nil
}

// LocalProjects scans the local filesystem to identify existing
// projects.
func LocalProjects(git *gitutil.Git, hg *hgutil.Hg) (map[string]Project, error) {
	root, err := VeyronRoot()
	if err != nil {
		return nil, err
	}
	projects := map[string]Project{}
	if err := findLocalProjects(root, projects, git, hg); err != nil {
		return nil, err
	}
	return projects, nil
}

// SelfUpdate updates the given tool to the latest version.
func SelfUpdate(verbose bool, stdout io.Writer, name string) error {
	run := runutil.New(verbose, stdout)
	git, hg := gitutil.New(run), hgutil.New(run)
	// Always log the output of selfupdate logic irrespective of
	// the value of the verbose flag.
	verboseRun := runutil.New(true, stdout)
	updateFn := func() error { return selfUpdate(git, hg, verboseRun, name) }
	return run.Function(updateFn, "Updating tool %q", name)
}

// UpdateLocalProject advances the master branch of a project that is
// expected to exist locally at project.Path.
func UpdateLocalProject(project Project, git *gitutil.Git, hg *hgutil.Hg) error {
	return applyToLocalProject(project, git, hg, func() error { return repoPull(git, hg, project.Protocol) })
}

// applyToLocalProject applies the operation expressed as the given
// function to the master branch of a project that is expected to
// exist locally at project.Path.
func applyToLocalProject(project Project, git *gitutil.Git, hg *hgutil.Hg, fn func() error) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer os.Chdir(wd)
	if err := os.Chdir(project.Path); err != nil {
		return fmt.Errorf("Chdir(%v) failed: %v", project.Path, err)
	}
	switch project.Protocol {
	case "git":
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
		if err := git.CheckoutBranch("master", !gitutil.Force); err != nil {
			return err
		}
		defer git.CheckoutBranch(branch, !gitutil.Force)
	case "hg":
		branch, err := hg.CurrentBranchName()
		if err != nil {
			return err
		}
		if err := hg.CheckoutBranch("default"); err != nil {
			return err
		}
		defer hg.CheckoutBranch(branch)
	default:
		return fmt.Errorf("unsupported protocol %v", project.Protocol)
	}
	return fn()
}

// findLatestProjects implements LatestProjects.
func findLatestProjects(manifestFile string, projects map[string]Project, git *gitutil.Git) error {
	root, err := VeyronRoot()
	if err != nil {
		return err
	}
	// Update the manifest.
	project := Project{
		Path:     filepath.Join(root, ".manifest"),
		Protocol: "git",
	}
	if err := UpdateLocalProject(project, git, nil); err != nil {
		return err
	}
	// Parse the manifest.
	path := filepath.Join(root, ".manifest", manifestFile+".xml")
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("ReadFile(%v) failed: %v", path, err)
	}
	var manifest Manifest
	if err := xml.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("Unmarshal(%v) failed: %v", string(data), err)
	}
	for _, project := range manifest.Projects {
		// git is the default protocol.
		if project.Protocol == "" {
			project.Protocol = "git"
		}
		project.Path = filepath.Join(root, project.Path)
		projects[project.Name] = project
	}
	return nil
}

// findLocalProjects implements LocalProjects.
func findLocalProjects(path string, projects map[string]Project, git *gitutil.Git, hg *hgutil.Hg) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer os.Chdir(wd)
	if err := os.Chdir(path); err != nil {
		return fmt.Errorf("Chdir(%v) failed: %v", path, err)
	}
	gitDir := filepath.Join(path, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		name, err := git.RepoName()
		if err != nil {
			return err
		}
		if project, ok := projects[name]; ok {
			return fmt.Errorf("name conflict: both %v and %v contain the project %v", project.Path, path, name)
		}
		projects[name] = Project{
			Name:     name,
			Path:     path,
			Protocol: "git",
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("Stat(%v) failed: %v", gitDir, err)
	}
	hgDir := filepath.Join(path, ".hg")
	if _, err := os.Stat(hgDir); err == nil {
		name, err := hg.RepoName()
		if err != nil {
			return err
		}
		if project, ok := projects[name]; ok {
			return fmt.Errorf("name conflict: both %v and %v contain the project %v", project.Path, path, name)
		}
		projects[name] = Project{
			Name:     name,
			Path:     path,
			Protocol: "hg",
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("Stat(%v) failed: %v", hgDir, err)
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
			if err := findLocalProjects(filepath.Join(path, fi.Name()), projects, git, hg); err != nil {
				return err
			}
		}
	}
	return nil
}

// findToolPackage finds the package path for the given tool.
func findToolPackage(name string) (string, error) {
	conf, err := VeyronConfig()
	if err != nil {
		return "", err
	}
	pkg, ok := conf.Tools[name]
	if !ok {
		return "", fmt.Errorf("could not find package for tool %v", name)
	}
	return pkg, nil
}

// repoPull invokes a pull operation in the current working directory
// using the given version control protocol.
func repoPull(git *gitutil.Git, hg *hgutil.Hg, protocol string) error {
	switch protocol {
	case "git":
		return git.Pull("origin", "master")
	case "hg":
		return hg.Pull()
	default:
		return fmt.Errorf("unsupported protocol %v", protocol)
	}
}

// selfUpdate is the implementation of SelfUpdate. The self-update
// logic uses three anchor points: the VEYRON_ROOT environment
// variable, URL of the tools repository, and the veyron tools
// configuration file location that maps stores a map from tool names
// to package paths.
//
// The reason for these anchor points is that executing the selfupdate
// logic is expected to retrieve the latest source files, build them,
// and install the resulting binary to the $VEYRON_ROOT/bin
// directory. But the tool that invokes the selfupdate logic may be
// arbitrarily old, so this only works if the source updating and
// binary building logic in an arbitrarily old tool still works.  Thus
// we define our anchor points, and require that these anchor points
// never change.
//
// Additionally, the self-update logic should not use os/exec to run
// the veyron tool to avoid executing an arbitrarilly old version of
// the veyron tool.
func selfUpdate(git *gitutil.Git, hg *hgutil.Hg, run *runutil.Run, name string) error {
	// Find where the tools repository exists locally and update it.
	projects, err := LocalProjects(git, hg)
	if err != nil {
		return err
	}
	const url = "https://veyron.googlesource.com/tools"
	project, ok := projects[url]
	if !ok {
		return fmt.Errorf("could not find project %v", url)
	}
	if expected, got := "git", project.Protocol; expected != got {
		return fmt.Errorf("unexpected protocol: expected %v, got %v", expected, got)
	}
	// Fetch the latest sources, and build the tool using the
	// veyron environment.
	buildFn := func() error {
		if err := repoPull(git, hg, project.Protocol); err != nil {
			return err
		}
		venv, err := VeyronEnvironment(HostPlatform())
		if err != nil {
			return err
		}
		env := envutil.ToMap(os.Environ())
		envutil.Replace(env, venv)
		root, err := VeyronRoot()
		if err != nil {
			return err
		}
		output := filepath.Join(root, "bin", name)
		count, err := git.CountCommits("HEAD", "")
		if err != nil {
			return err
		}
		pkg, err := findToolPackage(name)
		if err != nil {
			return err
		}
		ldflags := fmt.Sprintf("-X %v/impl.Version %d", pkg, count)
		args := []string{"build", "-ldflags", ldflags, "-o", output, pkg}
		var stderr bytes.Buffer
		if err := run.Command(ioutil.Discard, &stderr, env, "go", args...); err != nil {
			return fmt.Errorf("%v tool update failed\n%v", name, stderr.String())
		}
		return nil
	}
	return applyToLocalProject(project, git, hg, buildFn)
}
