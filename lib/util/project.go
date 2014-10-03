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

// LatestProjects parses the most recent version fo the project
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
func SelfUpdate(verbose bool, stdout io.Writer, manifest, name string) error {
	git := gitutil.New(runutil.New(verbose, stdout))
	// Always log the output of selfupdate logic irrespective of
	// the value of the verbose flag.
	run := runutil.New(true, stdout)
	updateFn := func() error { return selfUpdate(git, run, manifest, name) }
	return run.Function(updateFn, "Updating tool %q", name)
}

// UpdateProject advances the local master branch of the project
// identified by the given path.
func UpdateProject(path string, git *gitutil.Git, hg *hgutil.Hg) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer os.Chdir(wd)
	if err := os.Chdir(path); err != nil {
		return fmt.Errorf("Chdir(%v) failed: %v", path, err)
	}
	projects := map[string]Project{}
	if err := findLocalProjects(path, projects, git, hg); err != nil {
		return err
	}
	if expected, got := 1, len(projects); expected != got {
		return fmt.Errorf("unexpected length of %v: expected %v, got %v", projects, expected, got)
	}
	for _, project := range projects {
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
			if err := git.CheckoutBranch("master"); err != nil {
				return err
			}
			defer git.CheckoutBranch(branch)
			if err := git.Pull("origin", "master"); err != nil {
				return err
			}
		case "hg":
			branch, err := hg.CurrentBranchName()
			if err != nil {
				return err
			}
			if err := hg.CheckoutBranch("default"); err != nil {
				return err
			}
			defer hg.CheckoutBranch(branch)
			if err := hg.Pull(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported protocol %v", project.Protocol)
		}
	}
	return nil
}

// findLatestProjects implements FindLocalProjects.
func findLatestProjects(manifestFile string, projects map[string]Project, git *gitutil.Git) error {
	root, err := VeyronRoot()
	if err != nil {
		return err
	}
	// Update the manifest.
	path := filepath.Join(root, ".manifest")
	if err := UpdateProject(path, git, nil); err != nil {
		return err
	}
	// Parse the manifest.
	path = filepath.Join(root, ".manifest", manifestFile+".xml")
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

// findLocalProjects implements FindLocalProjects.
func findLocalProjects(path string, projects map[string]Project, git *gitutil.Git, hg *hgutil.Hg) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer os.Chdir(wd)
	if err := os.Chdir(path); err != nil {
		return fmt.Errorf("Chdir(%v) failed: %v", path, err)
	}
	name, err := git.RepoName()
	if err == nil {
		if project, ok := projects[name]; ok {
			return fmt.Errorf("name conflict: both %v and %v contain the project %v", project.Path, path, name)
		}
		projects[name] = Project{
			Name:     name,
			Path:     path,
			Protocol: "git",
		}
		return nil
	}
	name, err = hg.RepoName()
	if err == nil {
		if project, ok := projects[name]; ok {
			return fmt.Errorf("name conflict: both %v and %v contain the project %v", project.Path, path, name)
		}
		projects[name] = Project{
			Name:     name,
			Path:     path,
			Protocol: "hg",
		}
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
			if err := findLocalProjects(filepath.Join(path, fi.Name()), projects, git, hg); err != nil {
				return err
			}
		}
	}
	return nil
}

// selfUpdate is the implementation of SelfUpdate.
func selfUpdate(git *gitutil.Git, run *runutil.Run, manifest, name string) error {
	root, err := VeyronRoot()
	if err != nil {
		return err
	}
	url := "https://veyron.googlesource.com/tools"
	args := []string{fmt.Sprintf("-v=%v", run.Verbose), "project", "update", "-manifest=" + manifest, url}
	var stderr bytes.Buffer
	if err := run.Command(ioutil.Discard, &stderr, "veyron", args...); err != nil {
		return fmt.Errorf("%v", stderr.String())
	}
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer os.Chdir(wd)
	repo := filepath.Join(root, "tools")
	if err := os.Chdir(repo); err != nil {
		return fmt.Errorf("Chdir(%v) failed: %v", repo, err)
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
	count, err := git.CountCommits("HEAD", "")
	if err != nil {
		return err
	}
	output := filepath.Join(root, "bin", name)
	ldflags := fmt.Sprintf("-X tools/%v/impl.commitId %d", name, count)
	pkg := fmt.Sprintf("tools/%v", name)
	args = []string{"go", "build", "-ldflags", ldflags, "-o", output, pkg}
	stderr.Reset()
	if err := run.Command(ioutil.Discard, &stderr, "veyron", args...); err != nil {
		return fmt.Errorf("%v tool update failed\n%v", name, stderr.String())
	}
	return nil
}
