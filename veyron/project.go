package main

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"

	"veyron.io/lib/cmdline"
	"veyron.io/tools/lib/util"
)

// cmdProject represents the "veyron project" command.
var cmdProject = &cmdline.Command{
	Name:     "project",
	Short:    "Manage veyron projects",
	Long:     "Manage veyron projects.",
	Children: []*cmdline.Command{cmdProjectList, cmdProjectPoll},
}

// cmdProjectList represents the "veyron project list" command.
var cmdProjectList = &cmdline.Command{
	Run:   runProjectList,
	Name:  "list",
	Short: "List existing veyron projects",
	Long:  "Inspect the local filesystem and list the existing projects.",
}

// runProjectList generates a listing of local projects.
func runProjectList(command *cmdline.Command, _ []string) error {
	ctx := util.NewContextFromCommand(command, !noColorFlag, dryRunFlag, verboseFlag)
	projects, err := util.LocalProjects(ctx)
	if err != nil {
		return err
	}
	names := []string{}
	for name := range projects {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		project := projects[name]
		fmt.Fprintf(ctx.Stdout(), "project=%q path=%q\n", path.Base(name), project.Path)
		if branchesFlag {
			if err := ctx.Run().Chdir(project.Path); err != nil {
				return err
			}
			branches, current := []string{}, ""
			switch project.Protocol {
			case "git":
				branches, current, err = ctx.Git().GetBranches()
				if err != nil {
					return err
				}
			case "hg":
				branches, current, err = ctx.Hg().GetBranches()
				if err != nil {
					return err
				}
			default:
				return util.UnsupportedProtocolErr(project.Protocol)
			}
			for _, branch := range branches {
				if branch == current {
					fmt.Fprintf(ctx.Stdout(), "  * %v\n", branch)
				} else {
					fmt.Fprintf(ctx.Stdout(), "  %v\n", branch)
				}
			}
		}
	}
	return nil
}

// cmdProjectPoll represents the "veyron project poll" command.
var cmdProjectPoll = &cmdline.Command{
	Run:   runProjectPoll,
	Name:  "poll",
	Short: "Poll existing veyron projects",
	Long: `
Poll veyron projects that can affect the outcome of the given tests
and report whether any new changes in these projects exist. If no
tests are specified, all projects are polled by default.
`,
	ArgsName: "<test ...>",
	ArgsLong: "<test ...> is a list of tests that determine what projects to poll.",
}

// runProjectPoll generates a description of changes that exist
// remotely but do not exist locally.
func runProjectPoll(command *cmdline.Command, args []string) error {
	projectSet := map[string]struct{}{}
	if len(args) > 0 {
		var config util.CommonConfig
		if err := util.LoadConfig("common", &config); err != nil {
			return err
		}
		// Invert the config.ProjectTests map that maps
		// projects to tests to run.
		testProjects := map[string][]string{}
		for project, tests := range config.ProjectTests {
			for _, test := range tests {
				testProjects[test] = append(testProjects[test], project)
			}
		}
		for _, arg := range args {
			projects, ok := testProjects[arg]
			if !ok {
				return fmt.Errorf("failed to find any projects for test %q", arg)
			}
			for _, project := range projects {
				projectSet[project] = struct{}{}
			}
		}
	}
	ctx := util.NewContextFromCommand(command, !noColorFlag, dryRunFlag, verboseFlag)
	update, err := util.PollProjects(ctx, manifestFlag, projectSet)
	if err != nil {
		return err
	}

	// Remove projects with empty changes.
	for project := range update {
		if changes := update[project]; len(changes) == 0 {
			delete(update, project)
		}
	}

	// Print update if it is not empty.
	if len(update) > 0 {
		bytes, err := json.MarshalIndent(update, "", "  ")
		if err != nil {
			return fmt.Errorf("MarshalIndent() failed: %v", err)
		}
		fmt.Fprintf(command.Stdout(), "%s\n", bytes)
	}
	return nil
}
