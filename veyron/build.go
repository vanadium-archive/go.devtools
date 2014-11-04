package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"tools/lib/cmdline"
	"tools/lib/util"
)

var cmdBuild = &cmdline.Command{
	Name:  "build",
	Short: "Tool for managing veyron builds",
	Long: `
The build command can be used to manage veyron builds. In particular,
it can be used to list known builds and generate new builds.

The builds are represented as manifests and are revisioned using the
manifest repository located in $VEYRON_ROOT/.manifest. Each build is
identified with a tag, which the $VEYRON_ROOT/tools/conf/veyron
configuration file associates with a set of jenkins projects that
determine the stability of the build.

Internally, build manifests are currently organized as follows:

 <manifest-dir>/
   builds/
     <tag1>/
       <tag1-build1>
       <tag1-build2>
       ...
     <tag2>/
       <tag2-build1>
       <tag2-build2>
       ...
     <tag3>/
     ...
   <tag1> # a symlink to a one of <tag1-build*>
   <tag2> # a symlink to a one of <tag2-build*>
   ...

NOTE: Unlike the veyron tool commands, the above internal organization
is not an API. It is an implementation and can change without notice.
`,
	Children: []*cmdline.Command{cmdBuildGenerate, cmdBuildList},
}

// cmdBuildGenerate represents the 'generate' sub-command of the
// 'veyron' command of the veyron tool.
var cmdBuildGenerate = &cmdline.Command{
	Run:   runBuildGenerate,
	Name:  "generate",
	Short: "Generate a new veyron build",
	Long: `
Given a build tag, the "buildbot generate" command checks whether all
tests associated with the tag in the $VEYRON_ROOT/tools/conf/buildbot
config file pass. If so, the tool creates a new manifest that captures
the current state of the veyron universe repositories, commits this
manifest to the manifest repository, and updates the build "symlink"
to point to the latest build.
`,
	ArgsName: "<tag>",
	ArgsLong: "<tag> is a build tag.",
}

func runBuildGenerate(command *cmdline.Command, args []string) error {
	if got, want := len(args), 1; got != want {
		return command.UsageErrorf("unexpected number of arguments: got %v, want %v", got, want)
	}

	// Load the configuration file and run the tests that should
	// be run for the given tag.
	if err := runTests(command, args[0]); err != nil {
		return err
	}

	// Create a manifest that encodes the current state of all of
	// the projects.
	ctx := util.NewContextFromCommand(command, verboseFlag)
	if err := util.CreateBuildManifest(ctx, args[0]); err != nil {
		return err
	}
	return nil
}

// runTests runs the tests associated with the given build tag.
func runTests(command *cmdline.Command, tag string) error {
	root, err := util.VeyronRoot()
	if err != nil {
		return err
	}
	config, err := util.VeyronConfig()
	if err != nil {
		return err
	}
	tests, ok := config.TestMap[tag]
	if !ok {
		return fmt.Errorf("tag %v not found in config file %v", tag, filepath.Join(root, "tools", "conf", "veyron"))
	}
	for _, test := range tests {
		testPath := filepath.Join(root, "scripts", "jenkins", test)
		testCmd := exec.Command(testPath)
		testCmd.Stdout = command.Stdout()
		testCmd.Stdout = command.Stderr()
		if err := testCmd.Run(); err != nil {
			return fmt.Errorf("%v failed: %v", strings.Join(testCmd.Args, " "), err)
		}
	}
	return nil
}

// cmdBuildList represents the 'list' sub-command of 'build' command
// of the veyron tool.
var cmdBuildList = &cmdline.Command{
	Run:   runBuildList,
	Name:  "list",
	Short: "List existing veyron builds",
	Long: `
The "buildbot list" command lists existing veyron builds for the tags
specified as command-line arguments. If no arguments are provided, the
command lists builds for all known tags.
`,
	ArgsName: "<tag ...>",
	ArgsLong: "<tag ...> is a list of build tags.",
}

func runBuildList(command *cmdline.Command, args []string) error {
	manifestDir, err := util.RemoteManifestDir()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		// Identify all known builds tags, using a heuristic
		// that looks for all symbolic links <foo> in the
		// manifest directory that point to a file in the
		// "builds/<foo>" subdirectory of the manifest
		// directory.
		fileInfoList, err := ioutil.ReadDir(manifestDir)
		if err != nil {
			return fmt.Errorf("ReadDir(%v) failed: %v", manifestDir, err)
		}
		for _, fileInfo := range fileInfoList {
			if fileInfo.Mode()&os.ModeSymlink != 0 {
				path := filepath.Join(manifestDir, fileInfo.Name())
				dst, err := filepath.EvalSymlinks(path)
				if err != nil {
					return fmt.Errorf("EvalSymlinks(%v) failed: %v", path, err)
				}
				if strings.HasSuffix(filepath.Dir(dst), filepath.Join("builds", fileInfo.Name())) {
					args = append(args, fileInfo.Name())
				}
			}
		}
	}
	// Check that all tags exist.
	failed := false
	for _, tag := range args {
		buildDir := filepath.Join(manifestDir, "builds", tag)
		if _, err := os.Stat(buildDir); err != nil {
			if os.IsNotExist(err) {
				failed = true
				fmt.Fprintf(command.Stderr(), "build tag %v not found", tag)
			} else {
				return fmt.Errorf("Stat(%v) failed: %v", buildDir, tag)
			}
		}
	}
	if failed {
		return cmdline.ErrExitCode(2)
	}
	// Print builds for all tags.
	sort.Strings(args)
	for _, tag := range args {
		// Scan the manifest directory "builds/<tag>" printing
		// all builds.
		buildDir := filepath.Join(manifestDir, "builds", tag)
		fileInfoList, err := ioutil.ReadDir(buildDir)
		if err != nil {
			return fmt.Errorf("ReadDir(%v) failed: %v", buildDir, err)
		}
		fmt.Fprintf(command.Stdout(), "%q builds:\n", tag)
		for _, fileInfo := range fileInfoList {
			fmt.Fprintf(command.Stdout(), "  %v\n", fileInfo.Name())
		}
	}
	return nil
}
