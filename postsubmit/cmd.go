package main

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"v.io/lib/cmdline"
	"v.io/tools/lib/testutil"
	"v.io/tools/lib/util"
	"v.io/tools/lib/version"
)

var (
	// flags
	dryRunFlag       bool
	jenkinsHostFlag  string
	jenkinsTokenFlag string
	manifestFlag     string
	noColorFlag      bool
	verboseFlag      bool

	// A root test watches changes and triggers other Jenkins targets.
	defaultRootTests = map[string]struct{}{
		"third_party-go-build":            struct{}{},
		"vanadium-go-build":               struct{}{},
		"vanadium-js-browser-integration": struct{}{},
		"vanadium-js-build-extension":     struct{}{},
		"vanadium-js-node-integration":    struct{}{},
		"vanadium-js-unit":                struct{}{},
		"vanadium-js-vdl":                 struct{}{},
		"vanadium-js-vom":                 struct{}{},
		"vanadium-namespace-browser-test": struct{}{},
		"vanadium-www-playground":         struct{}{},
		"vanadium-www-site":               struct{}{},
		"vanadium-www-tutorials":          struct{}{},
	}
)

func init() {
	cmdRoot.Flags.BoolVar(&dryRunFlag, "n", false, "Show what commands will run but do not execute them.")
	cmdRoot.Flags.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
	cmdRoot.Flags.StringVar(&jenkinsHostFlag, "host", "", "The Jenkins host. Presubmit will not send any CLs to an empty host.")
	cmdRoot.Flags.StringVar(&jenkinsTokenFlag, "token", "", "The Jenkins API token.")
	cmdRoot.Flags.BoolVar(&noColorFlag, "nocolor", false, "Do not use color to format output.")
	cmdPoll.Flags.StringVar(&manifestFlag, "manifest", "default", "Name of the project manifest.")
}

// root returns a command that represents the root of the postsubmit tool.
func root() *cmdline.Command {
	return cmdRoot
}

// cmdRoot represents the root of the postsubmit tool.
var cmdRoot = &cmdline.Command{
	Name:     "postsubmit",
	Short:    "Tool for performing various postsubmit related functions",
	Long:     "The postsubmit tool performs various postsubmit related functions.",
	Children: []*cmdline.Command{cmdPoll, cmdVersion},
}

// cmdPoll represents the "poll" command of the postsubmit tool.
var cmdPoll = &cmdline.Command{
	Run:   runPoll,
	Name:  "poll",
	Short: "Poll changes and start corresponding builds on Jenkins",
	Long:  "Poll changes and start corresponding builds on Jenkins.",
}

func runPoll(command *cmdline.Command, _ []string) error {
	ctx := util.NewContextFromCommand(command, !noColorFlag, dryRunFlag, verboseFlag)
	root, err := util.VanadiumRoot()
	if err != nil {
		return err
	}

	// Get the latest snapshot file from $VANADIUM_ROOT/.update_history directory.
	historyDir := filepath.Join(root, ".update_history")
	var maxTime int64
	latestSnapshotFile := ""
	filepath.Walk(historyDir, func(path string, info os.FileInfo, err error) error {
		if info.ModTime().Unix() > maxTime {
			latestSnapshotFile = path
		}
		return nil
	})

	// Get projects with new changes from the latest snapshots.
	snapshotFileBytes, err := ioutil.ReadFile(latestSnapshotFile)
	if err != nil {
		return fmt.Errorf("ReadAll() failed: %v", err)
	}
	projects, err := getChangedProjectsFromSnapshot(ctx, root, snapshotFileBytes)
	if err != nil {
		return err
	}
	if len(projects) == 0 {
		fmt.Fprintf(ctx.Stdout(), "No changes.\n")
		return nil
	}
	fmt.Fprintf(ctx.Stdout(), "Projects with new changes:\n%s\n", strings.Join(projects, "\n"))

	// Identify the Jenkins tests that should be started.
	jenkinsTests, err := jenkinsTestsToStart(projects)
	if err != nil {
		return err
	}
	fmt.Fprintf(ctx.Stdout(), "\nJenkins tests to start:\n%s\n", strings.Join(jenkinsTests, "\n"))

	// Start Jenkins tests.
	fmt.Fprintf(ctx.Stdout(), "\nStarting new builds:\n")
	if err := startJenkinsTests(ctx, jenkinsTests); err != nil {
		return err
	}

	return nil
}

// getChangedProjectsFromSnapshot returns a slice of projects that
// have changes by comparing the revisions in the given snapshot with
// master branches.
func getChangedProjectsFromSnapshot(ctx *util.Context, vroot string, snapshotContent []byte) ([]string, error) {
	// Parse snapshot.
	snapshot := util.Manifest{}
	if err := xml.Unmarshal(snapshotContent, &snapshot); err != nil {
		return nil, fmt.Errorf("Unmarshal() failed: %v\n%v", err, string(snapshotContent))
	}

	// Use "git log" to detect changes for each project.
	//
	// TODO(jingjin, jsimsa): Add support for non-git projects.
	changedProjects := []string{}
	for _, project := range snapshot.Projects {
		switch project.Protocol {
		case "git":
			git := ctx.Git(util.RootDirOpt(filepath.Join(vroot, project.Path)))
			commits, err := git.Log("master", project.Revision, "")
			if err != nil {
				return nil, err
			}
			if len(commits) != 0 {
				changedProjects = append(changedProjects, project.Name)
			}
		}
	}
	return changedProjects, nil
}

// jenkinsTestsToStart returns a list of jenkins tests that need to be
// started based on the given projects.
func jenkinsTestsToStart(projects []string) ([]string, error) {
	// Parse tools config to get project-tests map.
	var config util.Config
	if err := util.LoadConfig("common", &config); err != nil {
		return nil, err
	}

	// Get all Jenkins tests for the given projects.
	jenkinsTests := config.ProjectTests(projects)

	// Only return the "root" tests.
	rootTests := []string{}
	for _, test := range jenkinsTests {
		if _, ok := defaultRootTests[test]; ok {
			rootTests = append(rootTests, test)
		}
	}

	// If vanadium-go-build is in the root tests, remove all
	// js tests because they will be triggered by vanadium-go-build
	// in Jenkins.
	filteredRootTests := []string{}
	hasGoBuild := false
	for _, test := range rootTests {
		if test == "vanadium-go-build" {
			hasGoBuild = true
			break
		}
	}
	for _, test := range rootTests {
		if !hasGoBuild || (hasGoBuild && !strings.HasPrefix(test, "vanadium-js")) {
			filteredRootTests = append(filteredRootTests, test)
		}
	}
	return filteredRootTests, nil
}

// startJenkinsTests uses Jenkins API to start a build to each of the
// given Jenkins tests.
func startJenkinsTests(ctx *util.Context, tests []string) error {
	urlParam := url.Values{
		"token": {jenkinsTokenFlag},
	}.Encode()
	jenkinsUrl, err := url.Parse(jenkinsHostFlag)
	if err != nil {
		return fmt.Errorf("Parse(%q) failed: %v", jenkinsHostFlag, err)
	}
	basePath := jenkinsUrl.Path
	for _, test := range tests {
		addBuildUrl := jenkinsUrl
		addBuildUrl.Path = fmt.Sprintf("%s/job/%s/build", basePath, test)
		addBuildUrl.RawQuery = urlParam
		resp, err := http.Get(addBuildUrl.String())
		msg := fmt.Sprintf("add build to %q\n", test)
		if err == nil {
			resp.Body.Close()
			testutil.Pass(ctx, "%s", msg)
		} else {
			testutil.Fail(ctx, "%s", msg)
			fmt.Fprintf(ctx.Stderr(), "%v\n", err)
		}
	}
	return nil
}

// cmdVersion represent the "version" command of the postsubmit tool.
var cmdVersion = &cmdline.Command{
	Run:   runVersion,
	Name:  "version",
	Short: "Print version",
	Long:  "Print version of the postsubmit tool.",
}

func runVersion(command *cmdline.Command, _ []string) error {
	fmt.Fprintf(command.Stdout(), "postsubmit tool version %v\n", version.Version)
	return nil
}
