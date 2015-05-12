// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $V23_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go .

package main

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/lib/cmdline"
)

var (
	// flags
	dryRunFlag      bool
	jenkinsHostFlag string
	manifestFlag    string
	colorFlag       bool
	verboseFlag     bool

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
		"vanadium-playground-test":        struct{}{},
		"vanadium-www-site":               struct{}{},
		"vanadium-www-tutorials":          struct{}{},
	}
)

func init() {
	cmdRoot.Flags.BoolVar(&dryRunFlag, "n", false, "Show what commands will run but do not execute them.")
	cmdRoot.Flags.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
	cmdRoot.Flags.StringVar(&jenkinsHostFlag, "host", "", "The Jenkins host. Presubmit will not send any CLs to an empty host.")
	cmdRoot.Flags.BoolVar(&colorFlag, "color", true, "Use color to format output.")
	cmdPoll.Flags.StringVar(&manifestFlag, "manifest", "", "Name of the project manifest.")
}

func main() {
	cmdline.Main(cmdRoot)
}

// cmdRoot represents the root of the postsubmit tool.
var cmdRoot = &cmdline.Command{
	Name:  "postsubmit",
	Short: "Perform Vanadium postsubmit related functions",
	Long: `
Command postsubmit performs Vanadium postsubmit related functions.
`,
	Children: []*cmdline.Command{cmdPoll, cmdVersion},
}

// cmdPoll represents the "poll" command of the postsubmit tool.
var cmdPoll = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runPoll),
	Name:   "poll",
	Short:  "Poll changes and start corresponding builds on Jenkins",
	Long:   "Poll changes and start corresponding builds on Jenkins.",
}

func runPoll(env *cmdline.Env, _ []string) error {
	ctx := tool.NewContextFromEnv(env, tool.ContextOpts{
		Color:    &colorFlag,
		DryRun:   &dryRunFlag,
		Manifest: &manifestFlag,
		Verbose:  &verboseFlag,
	})
	root, err := util.V23Root()
	if err != nil {
		return err
	}

	// Get the latest snapshot file from $V23_ROOT/.update_history directory.
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
	jenkinsTests, err := jenkinsTestsToStart(ctx, projects)
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
func getChangedProjectsFromSnapshot(ctx *tool.Context, vroot string, snapshotContent []byte) ([]string, error) {
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
			git := ctx.Git(tool.RootDirOpt(filepath.Join(vroot, project.Path)))
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
func jenkinsTestsToStart(ctx *tool.Context, projects []string) ([]string, error) {
	// Parse tools config to get project-tests map.
	config, err := util.LoadConfig(ctx)
	if err != nil {
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
func startJenkinsTests(ctx *tool.Context, tests []string) error {
	jenkins, err := ctx.Jenkins(jenkinsHostFlag)
	if err != nil {
		return err
	}

	for _, t := range tests {
		msg := fmt.Sprintf("add build to %q\n", t)
		if err := jenkins.AddBuild(t); err == nil {
			test.Pass(ctx, "%s", msg)
		} else {
			test.Fail(ctx, "%s", msg)
			fmt.Fprintf(ctx.Stderr(), "%v\n", err)
		}
	}
	return nil
}

// cmdVersion represent the "version" command of the postsubmit tool.
var cmdVersion = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runVersion),
	Name:   "version",
	Short:  "Print version",
	Long:   "Print version of the postsubmit tool.",
}

func runVersion(env *cmdline.Env, _ []string) error {
	fmt.Fprintf(env.Stdout, "postsubmit tool version %v\n", tool.Version)
	return nil
}
