// Package util contains a variety of general purpose functions, such
// as the SelfUpdate() function, for writing tools.
package util

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"tools/lib/gitutil"
	"tools/lib/hgutil"
	"tools/lib/runutil"
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
	vars := []string{
		"CGO_ENABLED",
		"CGO_CFLAGS",
		"CGO_LDFLAGS",
		"GOPATH",
	}
	for _, v := range vars {
		baseEnv[v] = os.Getenv(v)
	}
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

// SetupVeyronEnvironment sets up the environment variables used by
// the veyron setup. Developers that wish to do the environment
// variable setup themselves, should set the VEYRON_ENV_SETUP
// environment variable to "none".
func SetupVeyronEnvironment() error {
	if os.Getenv("VEYRON_ENV_SETUP") != "none" {
		environment, err := VeyronEnvironment()
		if err != nil {
			return err
		}
		for key, value := range environment {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("Setenv(%v, %v) failed: %v", key, value, err)
			}
		}
	}
	return nil
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
	gopath := parseTokens(baseEnv["GOPATH"], ":")
	// Append an entry to gopath for each veyron go repo.
	for _, repo := range config.GoRepos {
		gopath = append(gopath, filepath.Join(root, repo, "go"))
	}
	env := map[string]string{}
	env["GOPATH"] = strings.Join(gopath, ":")
	// Set the CGO_* variables for the veyron proximity component.
	if runtime.GOOS == "linux" {
		env["CGO_ENABLED"] = "1"
		libs := []string{
			"dbus-1.6.14",
			"expat-2.1.0",
			"bluez-4.101",
			"libusb-1.0.16-rc10",
			"libusb-compat-0.1.5",
		}
		archCmd := exec.Command("uname", "-m")
		arch, err := archCmd.Output()
		if err != nil {
			return nil, fmt.Errorf("failed to get host architecture: %v\n%v\n%s", err, strings.Join(archCmd.Args, " "))
		}
		cflags := parseTokens(baseEnv["CGO_CFLAGS"], " ")
		ldflags := parseTokens(baseEnv["CGO_LDFLAGS"], " ")
		for _, lib := range libs {
			dir := filepath.Join(root, "environment", "cout", lib, strings.TrimSpace(string(arch)))
			if _, err := os.Stat(dir); err != nil {
				if !os.IsNotExist(err) {
					return nil, fmt.Errorf("Stat(%v) failed: %v", dir, err)
				}
			} else {
				if lib == "dbus-1.6.14" {
					cflags = append(cflags, filepath.Join("-I"+dir, "include", "dbus-1.0", "dbus"))
				} else {
					cflags = append(cflags, filepath.Join("-I"+dir, "include"))
				}
				ldflags = append(ldflags, filepath.Join("-L"+dir, "lib"), "-Wl,-rpath", filepath.Join(dir, "lib"))
			}
		}
		env["CGO_CFLAGS"] = strings.Join(cflags, " ")
		env["CGO_LDFLAGS"] = strings.Join(ldflags, " ")
	}
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

type Project struct {
	Name     string `xml:"name,attr"`
	Path     string `xml:"path,attr"`
	Protocol string `xml:"protocol,attr"`
}

type Manifest struct {
	Projects []Project `xml:"project"`
}

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
		if project.Name == "" {
			project.Name = "git"
		}
		project.Path = filepath.Join(root, project.Path)
		projects[project.Name] = project
	}
	return nil
}

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

func parseTokens(tokens, separator string) []string {
	result := []string{}
	for _, token := range strings.Split(tokens, separator) {
		if token != "" {
			result = append(result, token)
		}
	}
	return result
}

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
