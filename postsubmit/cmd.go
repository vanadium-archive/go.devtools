package main

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"veyron.io/lib/cmdline"
	"veyron.io/tools/lib/testutil"
	"veyron.io/tools/lib/util"
	"veyron.io/tools/lib/version"
)

var (
	// flags
	dryRunFlag       bool
	jenkinsHostFlag  string
	jenkinsTokenFlag string
	manifestFlag     string
	noColorFlag      bool
	verboseFlag      bool

	// A root project watches changes and triggers other projects.
	defaultRootProjects = map[string]struct{}{
		"veyron-go-build":                       struct{}{},
		"third_party-go-build":                  struct{}{},
		"veyron-javascript-browser-integration": struct{}{},
		"veyron-javascript-build-extension":     struct{}{},
		"veyron-javascript-node-integration":    struct{}{},
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
	root, err := util.VeyronRoot()
	if err != nil {
		return err
	}

	// Get the latest snapshot file from $VEYRON_ROOT/.update_history directory.
	historyDir := filepath.Join(root, ".update_history")
	var maxTime int64
	latestSnapshotFile := ""
	filepath.Walk(historyDir, func(path string, info os.FileInfo, err error) error {
		if info.ModTime().Unix() > maxTime {
			latestSnapshotFile = path
		}
		return nil
	})

	// Get repos with new changes from the latest snapshots.
	snapshotFileBytes, err := ioutil.ReadFile(latestSnapshotFile)
	if err != nil {
		return fmt.Errorf("ReadAll() failed: %v", err)
	}
	repos, err := getChangedReposFromSnapshot(ctx, root, snapshotFileBytes)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		fmt.Fprintf(ctx.Stdout(), "No changes.\n")
		return nil
	}
	fmt.Fprintf(ctx.Stdout(), "Repos with new changes:\n%s\n", strings.Join(repos, "\n"))

	// Identify the Jenkins projects that should be started.
	jenkinsProjects, err := jenkinsProjectsToStart(repos)
	if err != nil {
		return err
	}
	fmt.Fprintf(ctx.Stdout(), "\nJenkins projects to start:\n%s\n", strings.Join(jenkinsProjects, "\n"))

	// Start Jenkins projects.
	fmt.Fprintf(ctx.Stdout(), "\nStarting new builds:\n")
	if err := startJenkinsProjects(ctx, jenkinsProjects); err != nil {
		return err
	}

	return nil
}

// getChangedReposFromSnapshot returns a slice of repos that have changes by
// comparing the revisions in the given snapshot with master branches.
func getChangedReposFromSnapshot(ctx *util.Context, veyronRoot string, snapshotContent []byte) ([]string, error) {
	// Parse snapshot.
	snapshot := util.Manifest{}
	if err := xml.Unmarshal(snapshotContent, &snapshot); err != nil {
		return nil, fmt.Errorf("Unmarshal() failed: %v\n%v", err, string(snapshotContent))
	}

	// Use "git log" to detect changes for each repo.
	repos := []string{}
	for _, project := range snapshot.Projects {
		git := ctx.Git(util.RootDirOpt(filepath.Join(veyronRoot, project.Path)))
		commits, err := git.Log("master", project.Revision, "")
		if err != nil {
			return nil, err
		}
		if len(commits) != 0 {
			repos = append(repos, project.Name)
		}
	}
	return repos, nil
}

// jenkinsProjectsToStart get a list of jenkins projects that need to be started
// based on the given repos.
func jenkinsProjectsToStart(repos []string) ([]string, error) {
	// Parse tools config to get project-tests map.
	var config util.CommonConfig
	if err := util.LoadConfig("common", &config); err != nil {
		return nil, err
	}
	projectTestsMap := config.ProjectTests

	// Get all Jenkins projects for the given repos.
	jenkinsProjectsMap := map[string]struct{}{}
	for _, repo := range repos {
		if projects, ok := projectTestsMap[repo]; ok {
			for _, project := range projects {
				jenkinsProjectsMap[project] = struct{}{}
			}
		}
	}
	jenkinsProjects := []string{}
	for p := range jenkinsProjectsMap {
		jenkinsProjects = append(jenkinsProjects, p)
	}
	sort.Strings(jenkinsProjects)

	// Only return the "root" projects.
	rootProjects := []string{}
	for _, p := range jenkinsProjects {
		if _, ok := defaultRootProjects[p]; ok {
			rootProjects = append(rootProjects, p)
		}
	}
	return rootProjects, nil
}

// startJenkinsProjects uses Jenkins API to start a build to each of the given
// Jenkins projects.
func startJenkinsProjects(ctx *util.Context, projects []string) error {
	urlParam := url.Values{
		"token": {jenkinsTokenFlag},
	}.Encode()
	jenkinsUrl, err := url.Parse(jenkinsHostFlag)
	if err != nil {
		return fmt.Errorf("Parse(%q) failed: %v", jenkinsHostFlag, err)
	}
	for _, p := range projects {
		addBuildUrl := jenkinsUrl
		addBuildUrl.Path = fmt.Sprintf("%s/job/%s/build", jenkinsUrl.Path, p)
		addBuildUrl.RawQuery = urlParam
		resp, err := http.Get(addBuildUrl.String())
		msg := fmt.Sprintf("add build to %q\n", p)
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
