// Package util provides utility functions for vanadium tools.
//
// TODO(jsimsa): Create a repoutil package that hides different
// version control systems behind a single interface.
package util

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"v.io/lib/cmdline"
	"v.io/tools/lib/collect"
	"v.io/tools/lib/gitutil"
	"v.io/tools/lib/runutil"
)

// CL represents a changelist.
type CL struct {
	// Author identifies the author of the changelist.
	Author string
	// Email identifies the author's email.
	Email string
	// Description holds the description of the changelist.
	Description string
}

// Manifest represents a setting used for updating the vanadium universe.
type Manifest struct {
	Imports  []Import  `xml:"imports>import"`
	Projects []Project `xml:"projects>project"`
	Tools    []Tool    `xml:"tools>tool"`
	XMLName  struct{}  `xml:"manifest"`
}

// Imports maps manifest import names to their detailed description.
type Imports map[string]Import

// Import represnts a manifest import.
type Import struct {
	// Name is the name under which the manifest can be found the
	// manifest repository.
	Name string `xml:"name,attr"`
}

// Projects maps vanadium project names to their detailed description.
type Projects map[string]Project

// Project represents a vanadium project.
type Project struct {
	// Exclude is flag used to exclude previously included projects.
	Exclude bool `xml:"exclude,attr"`
	// Name is the project name.
	Name string `xml:"name,attr"`
	// Path is the path used to store the project locally. Project
	// manifest uses paths that are relative to the VANADIUM_ROOT
	// environment variable. When a manifest is parsed (e.g. in
	// RemoteProjects), the program logic converts the relative
	// paths to an absolute paths, using the current value of the
	// VANADIUM_ROOT environment variable as a prefix.
	Path string `xml:"path,attr"`
	// Protocol is the version control protocol used by the
	// project. If not set, "git" is used as the default.
	Protocol string `xml:"protocol,attr"`
	// Remote is the project remote.
	Remote string `xml:"remote,attr"`
	// Revision is the revision the project should be advanced to
	// during "v23 update". If not set, "HEAD" is used as the
	// default.
	Revision string `xml:"revision,attr"`
}

// Tools maps vanadium tool names, to their detailed description.
type Tools map[string]Tool

// Tool represents a vanadium tool.
type Tool struct {
	// Exclude is flag used to exclude previously included projects.
	Exclude bool `xml:"exclude,attr"`
	// Name is the name of the tool binary.
	Name string `xml:"name,attr"`
	// Package is the package path of the tool.
	Package string `xml:"package,attr"`
	// Project identifies the project that contains the tool. If not
	// set, "https://vanadium.googlesource.com/release.go.tools" is used
	// as the default.
	Project string `xml:"project,attr"`
}

type UnsupportedProtocolErr string

func (e UnsupportedProtocolErr) Error() string {
	return fmt.Sprintf("unsupported protocol %v", e)
}

// Update represents an update of vanadium projects as a map from
// project names to a collections of commits.
type Update map[string][]CL

// CreateSnapshot creates a manifest that encodes the current state of
// master branches of all projects and writes this snapshot out to the
// given file.
func CreateSnapshot(ctx *Context, path string) error {
	// Create an in-memory representation of the build manifest.
	manifest, err := snapshotLocalProjects(ctx)
	if err != nil {
		return err
	}
	perm := os.FileMode(0755)
	if err := ctx.Run().MkdirAll(filepath.Dir(path), perm); err != nil {
		return err
	}
	data, err := xml.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	perm = os.FileMode(0644)
	if err := ctx.Run().WriteFile(path, data, perm); err != nil {
		return fmt.Errorf("WriteFile(%v, %v) failed: %v", path, err, perm)
	}
	return nil
}

// LocalProjects scans the local filesystem to identify existing
// projects.
func LocalProjects(ctx *Context) (Projects, error) {
	root, err := VanadiumRoot()
	if err != nil {
		return nil, err
	}
	// TODO(jsimsa): Remove this function once all projects
	// created before go/vcl/1381 have been transitioned to the
	// new format of v23 projects.
	if err := createV23Dir(ctx, root); err != nil {
		return nil, err
	}
	projects := Projects{}
	if err := findLocalProjects(ctx, root, projects); err != nil {
		return nil, err
	}
	return projects, nil
}

// PollProjects returns the set of changelists that exist remotely but
// not locally. Changes are grouped by vanadium projects and contain
// author identification and a description of their content.
func PollProjects(ctx *Context, manifest string, projectSet map[string]struct{}) (_ Update, e error) {
	update := Update{}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	defer collect.Error(func() error { return ctx.Run().Chdir(cwd) }, &e)
	localProjects, err := LocalProjects(ctx)
	if err != nil {
		return nil, err
	}
	remoteProjects, _, err := ReadManifest(ctx, manifest)
	if err != nil {
		return nil, err
	}
	ops, err := computeOperations(localProjects, remoteProjects, manifest, false)
	if err != nil {
		return nil, err
	}
	for _, op := range ops {
		name := op.Project().Name
		if len(projectSet) > 0 {
			if _, ok := projectSet[name]; !ok {
				continue
			}
		}
		cls := []CL{}
		if updateOp, ok := op.(updateOperation); ok {
			switch updateOp.project.Protocol {
			case "git":
				if err := ctx.Run().Chdir(updateOp.destination); err != nil {
					return nil, err
				}
				if err := ctx.Git().Fetch("origin", "master"); err != nil {
					return nil, err
				}
				commitsText, err := ctx.Git().Log("FETCH_HEAD", "master", "%an%n%ae%n%B")
				if err != nil {
					return nil, err
				}
				for _, commitText := range commitsText {
					if got, want := len(commitText), 3; got < want {
						return nil, fmt.Errorf("Unexpected length of %v: got %v, want at least %v", commitText, got, want)
					}
					cls = append(cls, CL{
						Author:      commitText[0],
						Email:       commitText[1],
						Description: strings.Join(commitText[2:], "\n"),
					})
				}
			default:
				return nil, UnsupportedProtocolErr(updateOp.project.Protocol)
			}
		}
		update[name] = cls
	}
	return update, nil
}

// ReadManifest retrieves and parses the manifest(s) that determine
// what projects and tools are to be updated.
func ReadManifest(ctx *Context, manifest string) (Projects, Tools, error) {
	// Update the manifest repository.
	root, err := VanadiumRoot()
	if err != nil {
		return nil, nil, err
	}
	project := Project{
		Path:     filepath.Join(root, ".manifest"),
		Protocol: "git",
		Revision: "HEAD",
	}
	if err := pullProject(ctx, project); err != nil {
		return nil, nil, err
	}
	// Read either the local manifest, if it exists, or the remote
	// manifest specified by the given name.
	path, err := LocalManifestFile()
	if err != nil {
		return nil, nil, err
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			path, err = ResolveManifestPath(manifest)
		} else {
			return nil, nil, fmt.Errorf("Stat(%v) failed: %v", err)
		}
	}
	projects, tools, stack := Projects{}, Tools{}, map[string]struct{}{}
	if err := readManifest(path, projects, tools, stack); err != nil {
		return nil, nil, err
	}
	return projects, tools, nil
}

// UpdateUniverse updates all local projects and tools to match the
// remote counterparts identified by the given manifest. Optionally,
// the 'gc' flag can be used to indicate that local projects that no
// longer exist remotely should be removed.
func UpdateUniverse(ctx *Context, manifest string, gc bool) (e error) {
	remoteProjects, remoteTools, err := ReadManifest(ctx, manifest)
	if err != nil {
		return err
	}
	// 1. Update all local projects to match their remote counterparts.
	if err := updateProjects(ctx, remoteProjects, manifest, gc); err != nil {
		return err
	}
	// 2. Build all tools in a temporary directory.
	tmpDir, err := ctx.Run().TempDir("", "tmp-vanadium-tools-build")
	if err != nil {
		return fmt.Errorf("TempDir() failed: %v", err)
	}
	defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpDir) }, &e)
	if err := buildTools(ctx, remoteTools, tmpDir); err != nil {
		return err
	}
	// 3. Install the tools into $VANADIUM_ROOT/bin.
	return installTools(ctx, tmpDir)
}

// ApplyToLocalMaster applies an operation expressed as the given
// function to the local master branch of the given project.
func ApplyToLocalMaster(ctx *Context, project Project, fn func() error) (e error) {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return ctx.Run().Chdir(cwd) }, &e)
	if err := ctx.Run().Chdir(project.Path); err != nil {
		return err
	}
	switch project.Protocol {
	case "git":
		branch, err := ctx.Git().CurrentBranchName()
		if err != nil {
			return err
		}
		stashed, err := ctx.Git().Stash()
		if err != nil {
			return err
		}
		if stashed {
			defer collect.Error(func() error { return ctx.Git().StashPop() }, &e)
		}
		if err := ctx.Git().CheckoutBranch("master", !gitutil.Force); err != nil {
			return err
		}
		defer collect.Error(func() error { return ctx.Git().CheckoutBranch(branch, !gitutil.Force) }, &e)
	case "hg":
		branch, err := ctx.Hg().CurrentBranchName()
		if err != nil {
			return err
		}
		if err := ctx.Hg().CheckoutBranch("default"); err != nil {
			return err
		}
		defer collect.Error(func() error { return ctx.Hg().CheckoutBranch(branch) }, &e)
	default:
		return UnsupportedProtocolErr(project.Protocol)
	}
	return fn()
}

// BuildTool builds the given tool specified by its name and package, sets its
// version by counting the number of commits in current version-controlled
// directory, and places the resulting binary into the given directory.
func BuildTool(ctx *Context, outputDir, name, pkg string, toolsProject Project) error {
	// Change to tools project's local dir.
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer ctx.Run().Chdir(wd)
	if err := ctx.Run().Chdir(toolsProject.Path); err != nil {
		return fmt.Errorf("Chdir(%v) failed: %v", toolsProject.Path, err)
	}

	env, err := VanadiumEnvironment(HostPlatform())
	if err != nil {
		return err
	}
	output := filepath.Join(outputDir, name)
	count := 0
	switch toolsProject.Protocol {
	case "git":
		gitCount, err := ctx.Git().CountCommits("HEAD", "")
		if err != nil {
			return err
		}
		count = gitCount
	default:
		return UnsupportedProtocolErr(toolsProject.Protocol)
	}
	ldflags := fmt.Sprintf("-X v.io/tools/lib/version.Version %d", count)
	args := []string{"build", "-ldflags", ldflags, "-o", output, pkg}
	var stderr bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Env = env.Map()
	opts.Stdout = ioutil.Discard
	opts.Stderr = &stderr
	if err := ctx.Run().CommandWithOpts(opts, "go", args...); err != nil {
		return fmt.Errorf("%v tool build failed\n%v", name, stderr.String())
	}
	return nil
}

// buildTools builds and installs all vanadium tools using the version
// available in the local master branch of the tools
// repository. Notably, this function does not perform any version
// control operation on the master branch.
func buildTools(ctx *Context, remoteTools Tools, outputDir string) error {
	localProjects, err := LocalProjects(ctx)
	if err != nil {
		return err
	}
	failed := false
	names := []string{}
	for name, _ := range remoteTools {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		tool := remoteTools[name]
		updateFn := func() error {
			project, ok := localProjects[tool.Project]
			if !ok {
				return fmt.Errorf("unknown project %v", tool.Project)
			}
			return ApplyToLocalMaster(ctx, project, func() error {
				return BuildTool(ctx, outputDir, tool.Name, tool.Package, project)
			})
		}
		// Always log the output of updateFn, irrespective of
		// the value of the verbose flag.
		opts := runutil.Opts{Verbose: true}
		if err := ctx.Run().FunctionWithOpts(opts, updateFn, "build tool %q", tool.Name); err != nil {
			// TODO(jsimsa): Switch this to Run().Output()?
			fmt.Fprintf(ctx.Stderr(), "%v\n", err)
			failed = true
		}
	}
	if failed {
		return cmdline.ErrExitCode(2)
	}
	return nil
}

// createV23Dir makes sure that the VANADIUM_ROOT instance contains
// appropriate .v23 directories.
func createV23Dir(ctx *Context, path string) (e error) {
	// If <path> points a directory that already contains the .v23
	// subdirectory exists, do nothing.
	v23Dir := filepath.Join(path, ".v23")
	if _, err := os.Stat(v23Dir); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("Stat(%v) failed: %v", v23Dir, err)
	}

	// Otherwise, create it for any git or hg project using a fix
	// mapping from project remotes to project names.
	names := map[string]string{
		"git@git-mirror:/home/veyron/vanadium/deprecated":                         "deprecated",
		"git@git-mirror:/home/veyron/vanadium/environment":                        "environment",
		"git@git-mirror:/home/veyron/vanadium/experimental":                       "experimental",
		"git@git-mirror:/home/veyron/vanadium/infrastructure":                     "infrastructure",
		"git@git-mirror:/home/veyron/vanadium/release/go/src/v.io/apps":           "release.go.apps",
		"git@git-mirror:/home/veyron/vanadium/release/go/src/v.io/core":           "release.go.core",
		"git@git-mirror:/home/veyron/vanadium/release/go/src/v.io/jni":            "release.go.jni",
		"git@git-mirror:/home/veyron/vanadium/release/go/src/v.io/lib":            "release.go.lib",
		"git@git-mirror:/home/veyron/vanadium/release/go/src/v.io/playground":     "release.go.playground",
		"git@git-mirror:/home/veyron/vanadium/release/go/src/v.io/tools":          "release.go.tools",
		"git@git-mirror:/home/veyron/vanadium/release/go/src/v.io/wspr":           "release.go.wspr",
		"git@git-mirror:/home/veyron/vanadium/release/java":                       "release.java",
		"git@git-mirror:/home/veyron/vanadium/release/javascript/core":            "release.js.core",
		"git@git-mirror:/home/veyron/vanadium/release/javascript/pgbundle":        "release.js.pgbundle",
		"git@git-mirror:/home/veyron/vanadium/release/javascript/vom":             "release.js.vom",
		"git@git-mirror:/home/veyron/vanadium/release/projects/namespace_browser": "namespace_browser",
		"git@git-mirror:/home/veyron/vanadium/release/projects/chat":              "release.projects.chat",
		"git@git-mirror:/home/veyron/vanadium/roadmap/go/src/v.io/store":          "roadmap.go.store",
		"git@git-mirror:/home/veyron/vanadium/roadmap/javascript/store":           "roadmap.js.store",
		"git@git-mirror:/home/veyron/vanadium/scripts":                            "scripts",
		"git@git-mirror:/home/veyron/vanadium/third_party":                        "third_party",
		"https://github.com/veyron/veyron-www":                                    "veyron-www",
		"https://github.com/monopole/mdrip":                                       "mdrip",
		"https://vanadium.googlesource.com/deprecated":                            "deprecated",
		"https://vanadium.googlesource.com/environment":                           "environment",
		"https://vanadium.googlesource.com/experimental":                          "experimental",
		"https://vanadium.googlesource.com/infrastructure":                        "infrastructure",
		"https://vanadium.googlesource.com/namespace_browser":                     "namespace_browser",
		"https://vanadium.googlesource.com/release.go.apps":                       "release.go.apps",
		"https://vanadium.googlesource.com/release.go.core":                       "release.go.core",
		"https://vanadium.googlesource.com/release.go.jni":                        "release.go.jni",
		"https://vanadium.googlesource.com/release.go.lib":                        "release.go.lib",
		"https://vanadium.googlesource.com/release.go.playground":                 "release.go.playground",
		"https://vanadium.googlesource.com/release.go.tools":                      "release.go.tools",
		"https://vanadium.googlesource.com/release.go.wspr":                       "release.go.wspr",
		"https://vanadium.googlesource.com/release.java":                          "release.java",
		"https://vanadium.googlesource.com/release.js.core":                       "release.js.core",
		"https://vanadium.googlesource.com/release.js.pgbundle":                   "release.js.pgbundle",
		"https://vanadium.googlesource.com/release.js.vom":                        "release.js.vom",
		"https://vanadium.googlesource.com/release.projects.chat":                 "release.projects.chat",
		"https://vanadium.googlesource.com/roadmap.go.store":                      "roadmap.go.store",
		"https://vanadium.googlesource.com/roadmap.js.store":                      "roadmap.js.store",
		"https://vanadium.googlesource.com/scripts":                               "scripts",
		"https://vanadium.googlesource.com/third_party":                           "third_party",
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return ctx.Run().Chdir(cwd) }, &e)
	if err := ctx.Run().Chdir(path); err != nil {
		return err
	}
	var project Project
	gitDir := filepath.Join(path, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		remote, err := ctx.Git().RepoName()
		if err != nil {
			return err
		}
		name, ok := names[remote]
		if !ok {
			// Skip over repositories not specified in
			// <names>.
			return nil
		}
		project = Project{
			Name:     name,
			Path:     path,
			Protocol: "git",
			Remote:   remote,
			Revision: "HEAD",
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("Stat(%v) failed: %v", gitDir, err)
	}
	hgDir := filepath.Join(path, ".hg")
	if _, err := os.Stat(hgDir); err == nil {
		remote, err := ctx.Hg().RepoName()
		if err != nil {
			return err
		}
		name, ok := names[remote]
		if !ok {
			// Skip over repositories not specified in
			// <names>.
			return nil
		}
		project = Project{
			Name:     name,
			Path:     path,
			Protocol: "hg",
			Remote:   remote,
			Revision: "tip",
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("Stat(%v) failed: %v", hgDir, err)
	}
	if project.Path != "" {
		if err := writeMetadata(ctx, project); err != nil {
			return err
		}
	} else {
		fileInfos, err := ioutil.ReadDir(path)
		if err != nil {
			return fmt.Errorf("ReadDir(%v) failed: %v", path, err)
		}
		for _, fileInfo := range fileInfos {
			if fileInfo.IsDir() && !strings.HasPrefix(fileInfo.Name(), ".") {
				if err := createV23Dir(ctx, filepath.Join(path, fileInfo.Name())); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// findLocalProjects implements LocalProjects.
func findLocalProjects(ctx *Context, path string, projects Projects) (e error) {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return ctx.Run().Chdir(cwd) }, &e)
	if err := ctx.Run().Chdir(path); err != nil {
		return err
	}
	v23Dir := filepath.Join(path, ".v23")
	if _, err := os.Stat(v23Dir); err == nil {
		metadataFile := filepath.Join(v23Dir, "metadata.v2")
		bytes, err := ctx.Run().ReadFile(metadataFile)
		if err != nil {
			return err
		}
		var project Project
		if err := xml.Unmarshal(bytes, &project); err != nil {
			return fmt.Errorf("Unmarshal() failed: %v\n%s", err, string(bytes))
		}
		if p, ok := projects[project.Name]; ok {
			return fmt.Errorf("name conflict: both %v and %v contain the project %v", p.Path, project.Path, project.Name)
		}
		projects[project.Name] = project
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("Stat(%v) failed: %v", v23Dir, err)
	}
	fileInfos, err := ioutil.ReadDir(path)
	if err != nil {
		return fmt.Errorf("ReadDir(%v) failed: %v", path, err)
	}
	for _, fileInfo := range fileInfos {
		if fileInfo.IsDir() && !strings.HasPrefix(fileInfo.Name(), ".") {
			if err := findLocalProjects(ctx, filepath.Join(path, fileInfo.Name()), projects); err != nil {
				return err
			}
		}
	}
	return nil
}

// installTools installs the tools from the given directory into
// $VANADIUM_ROOT/bin.
func installTools(ctx *Context, dir string) error {
	if ctx.DryRun() {
		// In "dry run" mode, no binaries are built.
		return nil
	}
	root, err := VanadiumRoot()
	if err != nil {
		return err
	}
	fis, err := ioutil.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("ReadDir(%v) failed: %v", dir, err)
	}
	failed := false
	for _, fi := range fis {
		installFn := func() error {
			src := filepath.Join(dir, fi.Name())
			dst := filepath.Join(root, "bin", fi.Name())
			if err := ctx.Run().Rename(src, dst); err != nil {
				return err
			}
			return nil
		}
		opts := runutil.Opts{Verbose: true}
		if err := ctx.Run().FunctionWithOpts(opts, installFn, "install tool %q", fi.Name()); err != nil {
			fmt.Fprintf(ctx.Stderr(), "%v\n", err)
			failed = true
		}
	}
	if failed {
		return cmdline.ErrExitCode(2)
	}
	return nil
}

// pullProject advances the local master branch of the given
// project, which is expected to exist locally at project.Path.
func pullProject(ctx *Context, project Project) error {
	pullFn := func() error {
		switch project.Protocol {
		case "git":
			if err := ctx.Git().Pull("origin", "master"); err != nil {
				return err
			}
			return ctx.Git().Reset(project.Revision)
		case "hg":
			if err := ctx.Hg().Pull(); err != nil {
				return err
			}
			return ctx.Hg().CheckoutRevision(project.Revision)
		default:
			return UnsupportedProtocolErr(project.Protocol)
		}
	}
	return ApplyToLocalMaster(ctx, project, pullFn)
}

// readManifest reads the given manifest, processing all of its
// imports, projects and tools settings.
func readManifest(path string, projects Projects, tools Tools, stack map[string]struct{}) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("ReadFile(%v) failed: %v", path, err)
	}
	m := &Manifest{}
	if err := xml.Unmarshal(data, m); err != nil {
		return fmt.Errorf("Unmarshal(%v) failed: %v", string(data), err)
	}
	// Process all imports.
	for _, manifest := range m.Imports {
		if _, ok := stack[manifest.Name]; ok {
			return fmt.Errorf("import cycle encountered")
		}
		path, err := ResolveManifestPath(manifest.Name)
		if err != nil {
			return err
		}
		stack[manifest.Name] = struct{}{}
		if err := readManifest(path, projects, tools, stack); err != nil {
			return err
		}
		delete(stack, manifest.Name)
	}
	// Process all projects.
	root, err := VanadiumRoot()
	if err != nil {
		return err
	}
	for _, project := range m.Projects {
		if project.Exclude {
			// Exclude the project in case it was
			// previously included.
			delete(projects, project.Name)
			continue
		}
		// Replace the relative path with an absolute one.
		project.Path = filepath.Join(root, project.Path)
		// Use git as the default protocol.
		if project.Protocol == "" {
			project.Protocol = "git"
		}
		// Use HEAD and tip as the default revision for git
		// and mercurial respectively.
		if project.Revision == "" {
			switch project.Protocol {
			case "git":
				project.Revision = "HEAD"
			case "hg":
				project.Revision = "tip"
			default:
			}
		}
		projects[project.Name] = project
	}
	// Process all tools.
	for _, tool := range m.Tools {
		if tool.Exclude {
			// Exclude the tool in case it was previously
			// included.
			delete(tools, tool.Name)
			continue
		}
		// Use the "release.go.tools" project as the default project.
		if tool.Project == "" {
			tool.Project = "https://vanadium.googlesource.com/release.go.tools"
		}
		tools[tool.Name] = tool
	}
	return nil
}

// reportNonMaster checks if the given project is on master branch and
// if not, reports this fact along with information on how to update it.
func reportNonMaster(ctx *Context, project Project) (e error) {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return ctx.Run().Chdir(cwd) }, &e)
	if err := ctx.Run().Chdir(project.Path); err != nil {
		return err
	}
	switch project.Protocol {
	case "git":
		current, err := ctx.Git().CurrentBranchName()
		if err != nil {
			return err
		}
		if current != "master" {
			line1 := fmt.Sprintf(`NOTE: "v23 update" only updates the "master" branch and the current branch is %q`, current)
			line2 := fmt.Sprintf(`to update the %q branch once the master branch is updated, run "git merge master"`, current)
			opts := runutil.Opts{Verbose: true}
			ctx.Run().OutputWithOpts(opts, []string{line1, line2})
		}
		return nil
	case "hg":
		return nil
	default:
		return UnsupportedProtocolErr(project.Protocol)
	}
}

// snapshotLocalProjects returns an in-memory representation of the
// current state of all local projects
func snapshotLocalProjects(ctx *Context) (*Manifest, error) {
	localProjects, err := LocalProjects(ctx)
	if err != nil {
		return nil, err
	}
	root, err := VanadiumRoot()
	if err != nil {
		return nil, err
	}
	manifest := Manifest{}
	for _, project := range localProjects {
		revision := ""
		revisionFn := func() error {
			switch project.Protocol {
			case "git":
				gitRevision, err := ctx.Git().LatestCommitID()
				if err != nil {
					return err
				}
				revision = gitRevision
				return nil
			case "hg":
				return nil
			default:
				return UnsupportedProtocolErr(project.Protocol)
			}
		}
		if err := ApplyToLocalMaster(ctx, project, revisionFn); err != nil {
			return nil, err
		}
		project.Revision = revision
		project.Path = strings.TrimPrefix(project.Path, root)
		manifest.Projects = append(manifest.Projects, project)
	}
	return &manifest, nil
}

// updateProjects updates all vanadium projects.
func updateProjects(ctx *Context, remoteProjects Projects, manifest string, gc bool) error {
	localProjects, err := LocalProjects(ctx)
	if err != nil {
		return err
	}
	ops, err := computeOperations(localProjects, remoteProjects, manifest, gc)
	if err != nil {
		return err
	}
	for _, op := range ops {
		if err := op.Test(); err != nil {
			return err
		}
	}
	failed := false
	for _, op := range ops {
		updateFn := func() error { return op.Run(ctx) }
		// Always log the output of updateFn, irrespective of
		// the value of the verbose flag.
		opts := runutil.Opts{Verbose: true}
		if err := ctx.Run().FunctionWithOpts(opts, updateFn, "%v", op); err != nil {
			// TODO(jsimsa): Switch this to Run.Output()?
			fmt.Fprintf(ctx.Stderr(), "%v\n", err)
			failed = true
		}
	}
	if failed {
		return cmdline.ErrExitCode(2)
	}
	return nil
}

// writeMetadata writes the given project metadata to the disk.
func writeMetadata(ctx *Context, project Project) (e error) {
	metadataDir := filepath.Join(project.Path, ".v23")
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return ctx.Run().Chdir(cwd) }, &e)
	if err := ctx.Run().MkdirAll(metadataDir, os.FileMode(0755)); err != nil {
		return err
	}
	if err := ctx.Run().Chdir(metadataDir); err != nil {
		return err
	}
	bytes, err := xml.Marshal(project)
	if err != nil {
		return fmt.Errorf("Marhsal() failed: %v", err)
	}
	metadataFile := filepath.Join(metadataDir, "metadata.v2")
	tmpMetadataFile := metadataFile + ".tmp"
	if err := ctx.Run().WriteFile(tmpMetadataFile, bytes, os.FileMode(0644)); err != nil {
		return err
	}
	if err := ctx.Run().Rename(tmpMetadataFile, metadataFile); err != nil {
		return err
	}
	return nil
}

type operation interface {
	// Project identifies the project this operation pertains to.
	Project() Project
	// Run executes the operation.
	Run(ctx *Context) error
	// String returns a string representation of the operation.
	String() string
	// Test checks whether the operation would fail.
	Test() error
}

// commonOperation represents a project operation.
type commonOperation struct {
	// project holds information about the project such as its
	// name, local path, and the protocol it uses for version
	// control.
	project Project
	// destination is the new project path.
	destination string
	// source is the current project path.
	source string
}

func (commonOperation) Run(*Context) error {
	return nil
}

func (op commonOperation) Project() Project {
	return op.project
}

func (commonOperation) String() string {
	return ""
}

func (commonOperation) Test() error {
	return nil
}

// createOperation represents the creation of a project.
type createOperation struct {
	commonOperation
}

// preCommitHook is a git hook installed to all new projects. It
// prevents accidental commits to the local master branch.

const preCommitHook = `#!/bin/bash

# Get the current branch name.
readonly BRANCH=$(git rev-parse --abbrev-ref HEAD)

if [[ "${BRANCH}" == "master" ]]
then
  echo "========================================================================="
  echo "Vanadium code cannot be committed to master using the 'git commit' command."
  echo "Please make a feature branch and commit your code there."
  echo "========================================================================="
  exit 1
fi

exit 0
`

// prePushHook is a git hook installed to all new projects. It
// prevents accidental pushes to the remote master branch.
const prePushHook = `#!/bin/bash

readonly REMOTE="$1"

# Get the current branch name.
readonly BRANCH=$(git rev-parse --abbrev-ref HEAD)

if [[ "${REMOTE}" == "origin" && "${BRANCH}" == "master" ]]
then
  echo "======================================================================="
  echo "Vanadium code cannot be pushed to master using the 'git push' command."
  echo "Use the 'git v23 review' command to follow the code review workflow."
  echo "======================================================================="
  exit 1
fi

exit 0
`

func (op createOperation) Run(ctx *Context) (e error) {
	path, perm := filepath.Dir(op.destination), os.FileMode(0755)
	if err := ctx.Run().MkdirAll(path, perm); err != nil {
		return err
	}
	switch op.project.Protocol {
	case "git":
		if err := ctx.Git().Clone(op.project.Remote, op.destination); err != nil {
			return err
		}
		if strings.HasPrefix(op.project.Remote, VanadiumGitRepoHost()) {
			// Setup the repository for Gerrit code reviews.
			//
			// TODO(jsimsa): Decide what to do in case we would want to update the
			// commit hooks for existing repositories. Overwriting the existing
			// hooks is not a good idea as developers might have customized the
			// hooks.
			file := filepath.Join(op.destination, ".git", "hooks", "commit-msg")
			url := "https://gerrit-review.googlesource.com/tools/hooks/commit-msg"
			args := []string{"-Lo", file, url}
			var stderr bytes.Buffer
			opts := ctx.Run().Opts()
			opts.Stdout = ioutil.Discard
			opts.Stderr = &stderr
			if err := ctx.Run().CommandWithOpts(opts, "curl", args...); err != nil {
				return fmt.Errorf("failed to download commit message hook: %v\n%v", err, stderr.String())
			}
			if err := os.Chmod(file, perm); err != nil {
				return fmt.Errorf("Chmod(%v, %v) failed: %v", file, perm, err)
			}
			file = filepath.Join(op.destination, ".git", "hooks", "pre-commit")
			if err := ctx.Run().WriteFile(file, []byte(preCommitHook), perm); err != nil {
				return fmt.Errorf("WriteFile(%v, %v) failed: %v", file, perm, err)
			}
			file = filepath.Join(op.destination, ".git", "hooks", "pre-push")
			if err := ctx.Run().WriteFile(file, []byte(prePushHook), perm); err != nil {
				return fmt.Errorf("WriteFile(%v, %v) failed: %v", file, perm, err)
			}
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		defer collect.Error(func() error { return ctx.Run().Chdir(cwd) }, &e)
		if err := ctx.Run().Chdir(op.destination); err != nil {
			return err
		}
		if err := ctx.Git().Reset(op.project.Revision); err != nil {
			return err
		}
	case "hg":
		if err := ctx.Hg().Clone(op.project.Remote, op.destination); err != nil {
			return err
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		defer collect.Error(func() error { return ctx.Run().Chdir(cwd) }, &e)
		if err := ctx.Run().Chdir(op.destination); err != nil {
			return err
		}
		if err := ctx.Hg().CheckoutRevision(op.project.Revision); err != nil {
			return err
		}
	default:
		return UnsupportedProtocolErr(op.project.Protocol)
	}
	if err := writeMetadata(ctx, op.project); err != nil {
		return err
	}
	return nil
}

func (op createOperation) String() string {
	return fmt.Sprintf("create project %q in %q and advance it to %q", op.project.Name, op.destination, op.project.Revision)
}

func (op createOperation) Test() error {
	// Check the local file system.
	if _, err := os.Stat(op.destination); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("Stat(%v) failed: %v", op.destination, err)
		}
	} else {
		return fmt.Errorf("cannot create %q as it already exists", op.destination)
	}
	return nil
}

// deleteOperation represents the deletion of a project.
type deleteOperation struct {
	commonOperation
	// gc determines whether the operation should be executed or
	// whether it should only print a notification.
	gc bool
	// manifest records the name of the current project manifest.
	manifest string
}

func (op deleteOperation) Run(ctx *Context) error {
	if op.gc {
		return ctx.Run().RemoveAll(op.source)
	}
	lines := []string{
		fmt.Sprintf("NOTE: this project was not found in the %q manifest", op.manifest),
		"it was not automatically removed to avoid deleting uncommitted work",
		fmt.Sprintf(`if you no longer need it, invoke "rm -rf %v"`, op.source),
		`or invoke "v23 update -gc" to remove all such local projects`,
	}
	opts := runutil.Opts{Verbose: true}
	ctx.Run().OutputWithOpts(opts, lines)
	return nil
}

func (op deleteOperation) String() string {
	return fmt.Sprintf("delete project %q from %q", op.project.Name, op.source)
}

func (op deleteOperation) Test() error {
	if _, err := os.Stat(op.source); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cannot delete %q as it does not exist", op.source)
		}
		return fmt.Errorf("Stat(%v) failed: %v", op.source, err)
	}
	return nil
}

// moveOperation represents the relocation of a project.
type moveOperation struct {
	commonOperation
}

func (op moveOperation) Run(ctx *Context) error {
	path, perm := filepath.Dir(op.destination), os.FileMode(0755)
	if err := ctx.Run().MkdirAll(path, perm); err != nil {
		return err
	}
	if err := ctx.Run().Rename(op.source, op.destination); err != nil {
		return err
	}
	if err := reportNonMaster(ctx, op.project); err != nil {
		return err
	}
	if err := pullProject(ctx, op.project); err != nil {
		return err
	}
	if err := writeMetadata(ctx, op.project); err != nil {
		return err
	}
	return nil
}

func (op moveOperation) String() string {
	return fmt.Sprintf("move project %q located in %q to %q and advance it to %q", op.project.Name, op.source, op.destination, op.project.Revision)
}

func (op moveOperation) Test() error {
	if _, err := os.Stat(op.source); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cannot move %q to %q as the source does not exist", op.source, op.destination)
		}
		return fmt.Errorf("Stat(%v) failed: %v", op.source, err)
	}
	if _, err := os.Stat(op.destination); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("Stat(%v) failed: %v", op.destination, err)
		}
	} else {
		return fmt.Errorf("cannot move %q to %q as the destination already exists", op.source, op.destination)
	}
	return nil
}

// updateOperation represents the update of a project.
type updateOperation struct {
	commonOperation
}

func (op updateOperation) Run(ctx *Context) error {
	if err := reportNonMaster(ctx, op.project); err != nil {
		return err
	}
	if err := pullProject(ctx, op.project); err != nil {
		return err
	}
	if err := writeMetadata(ctx, op.project); err != nil {
		return err
	}
	return nil
}

func (op updateOperation) String() string {
	return fmt.Sprintf("advance project %q located in %q to %q", op.project.Name, op.source, op.project.Revision)
}

func (op updateOperation) Test() error {
	return nil
}

// operations is a sortable collection of operations
type operations []operation

// Len returns the length of the collection.
func (ops operations) Len() int {
	return len(ops)
}

// Less defines the order of operations. Operations are ordered first
// by their type and then by their project name.
//
// The order in which operation types are defined determines the order
// in which operations are performed. For correctness and also to
// minimize the chance of a conflict, the delete operations should
// happen before move operations, which should happen before create
// operations.
func (ops operations) Less(i, j int) bool {
	vals := make([]int, 2)
	for idx, op := range []operation{ops[i], ops[j]} {
		switch op.(type) {
		case deleteOperation:
			vals[idx] = 0
		case moveOperation:
			vals[idx] = 1
		case createOperation:
			vals[idx] = 2
		case updateOperation:
			vals[idx] = 3
		}
	}
	if vals[0] != vals[1] {
		return vals[0] < vals[1]
	}
	return ops[i].Project().Name < ops[j].Project().Name
}

// Swap swaps two elements of the collection.
func (ops operations) Swap(i, j int) {
	ops[i], ops[j] = ops[j], ops[i]
}

// computeOperations inputs a set of projects to update and the set of
// current and new projects (as defined by contents of the local file
// system and manifest file respectively) and outputs a collection of
// operations that describe the actions needed to update the target
// projects.
func computeOperations(localProjects, remoteProjects Projects, manifest string, gc bool) (operations, error) {
	result := operations{}
	allProjects := map[string]struct{}{}
	for name, _ := range localProjects {
		allProjects[name] = struct{}{}
	}
	for name, _ := range remoteProjects {
		allProjects[name] = struct{}{}
	}
	for name, _ := range allProjects {
		if localProject, ok := localProjects[name]; ok {
			if remoteProject, ok := remoteProjects[name]; ok {
				if localProject.Path == remoteProject.Path {
					result = append(result, updateOperation{commonOperation{
						destination: remoteProject.Path,
						project:     remoteProject,
						source:      localProject.Path,
					}})
				} else {
					result = append(result, moveOperation{commonOperation{
						destination: remoteProject.Path,
						project:     remoteProject,
						source:      localProject.Path,
					}})
				}
			} else {
				result = append(result, deleteOperation{commonOperation{
					destination: "",
					project:     localProject,
					source:      localProject.Path,
				}, gc, manifest})
			}
		} else if remoteProject, ok := remoteProjects[name]; ok {
			result = append(result, createOperation{commonOperation{
				destination: remoteProject.Path,
				project:     remoteProject,
				source:      "",
			}})
		} else {
			return nil, fmt.Errorf("project %v does not exist", name)
		}
	}
	sort.Sort(result)
	return result, nil
}
