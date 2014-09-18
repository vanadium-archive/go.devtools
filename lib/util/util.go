package util

import (
	"bufio"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"tools/lib/cmd"
	"tools/lib/git"
)

const (
	rootEnv = "VEYRON_ROOT"
)

type configType struct {
	GoRepos []string
}

var baseEnv map[string]string

func init() {
	// Initialize the baseEnv map with values of the environment
	// variables relevant to veyron.
	baseEnv = map[string]string{}
	vars := []string{"GOPATH"}
	for _, v := range vars {
		baseEnv[v] = os.Getenv(v)
	}
}

// LatestProjects parses the most recent version fo the project
// manifest to identify the latest projects.
func LatestProjects(manifest string, git *git.Git) (map[string]string, error) {
	projects := map[string]string{}
	if err := findLatestProjects(manifest, projects, git); err != nil {
		return nil, err
	}
	return projects, nil
}

// LocalProjects scans the local filesystem to identify existing
// projects.
func LocalProjects(git *git.Git) (map[string]string, error) {
	root, err := VeyronRoot()
	if err != nil {
		return nil, err
	}
	projects := map[string]string{}
	if err := findLocalProjects(root, projects, git); err != nil {
		return nil, err
	}
	return projects, nil
}

// SelfUpdate updates the given tool to the latest version.
func SelfUpdate(verbose bool, manifest, name string) error {
	updateFn := func() error { return selfUpdate(verbose, manifest, name) }
	return cmd.Log(updateFn, "Updating tool %q", name)
}

// SetupVeyronEnvironment sets up the environment variables used by
// the veyron setup.
func SetupVeyronEnvironment() error {
	environment, err := VeyronEnvironment()
	if err != nil {
		return err
	}
	for key, value := range environment {
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("Setenv(%v, %v) failed: %v", key, value, err)
		}
	}
	return nil
}

// UpdateProject advances the local master branch of the project
// identified by the given path.
func UpdateProject(project string, git *git.Git) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer os.Chdir(wd)
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

// VeyronEnvironment returns the environment variables setting for
// veyron. The util package captures the original state of the
// environment variables relevant to veyron when it is initialized,
// and every invocation of this function updates this original state
// according to the current configuration of the veyron tool.
func VeyronEnvironment() (map[string]string, error) {
	root, err := VeyronRoot()
	if err != nil {
		return nil, err
	}
	configPath := filepath.Join(root, "tools", "conf", "veyron")
	configBytes, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("ReadFile(%v) failed: %v", configPath, err)
	}
	var config configType
	if err := json.Unmarshal(configBytes, &config); err != nil {
		return nil, fmt.Errorf("Unmarshal(%v) failed: %v", string(configBytes), err)
	}
	workspaces := []string{}
	for _, repo := range config.GoRepos {
		workspaces = append(workspaces, filepath.Join(root, repo, "go"))
	}
	env := map[string]string{}
	env["GOPATH"] = strings.Join(append([]string{baseEnv["GOPATH"]}, workspaces...), ":")
	return env, nil
}

// VeyronRoot returns the root of the veyron universe.
func VeyronRoot() (string, error) {
	root := os.Getenv(rootEnv)
	if root == "" {
		return "", fmt.Errorf("%v is not set", rootEnv)
	}
	return root, nil
}

type project struct {
	Name string `xml:"name,attr"`
	Path string `xml:"path,attr"`
}

type manifest struct {
	Projects []project `xml:"project"`
}

func findLatestProjects(manifestFile string, projects map[string]string, git *git.Git) error {
	root, err := VeyronRoot()
	if err != nil {
		return err
	}
	// Update the manifest.
	path := filepath.Join(root, ".manifest")
	if err := UpdateProject(path, git); err != nil {
		return err
	}
	// Parse the manifest.
	path = filepath.Join(root, ".manifest", manifestFile+".xml")
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

func findLocalProjects(path string, projects map[string]string, git *git.Git) error {
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
			if err := findLocalProjects(filepath.Join(path, fi.Name()), projects, git); err != nil {
				return err
			}
		}
	}
	return nil
}

func selfUpdate(verbose bool, manifest, name string) error {
	root, err := VeyronRoot()
	if err != nil {
		return err
	}
	url := "https://veyron.googlesource.com/tools"
	args := []string{fmt.Sprintf("-v=%v", verbose), "project", "update", "-manifest=" + manifest, url}
	if _, errOut, err := cmd.RunOutput(true, "veyron", args...); err != nil {
		return fmt.Errorf("%s", strings.Join(errOut, "\n"))
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
	goScript := filepath.Join(root, "veyron", "scripts", "build", "go")
	git := git.New(verbose)
	count, err := git.CountCommits("HEAD", "")
	if err != nil {
		return err
	}
	output := filepath.Join(root, "bin", name)
	ldflags := fmt.Sprintf("-X tools/%v/impl.commitId %d", name, count)
	pkg := fmt.Sprintf("tools/%v", name)
	args = []string{"build", "-ldflags", ldflags, "-o", output, pkg}
	if _, errOut, err := cmd.RunOutput(true, goScript, args...); err != nil {
		return fmt.Errorf("%v tool update failed\n%v", name, strings.Join(errOut, "\n"))
	}
	return nil
}
