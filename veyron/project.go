package main

import (
	"encoding/json"
	"fmt"

	"tools/lib/cmdline"
	"tools/lib/util"
)

// cmdProject represents the 'project' command of the veyron tool.
var cmdProject = &cmdline.Command{
	Name:     "project",
	Short:    "Manage veyron projects",
	Long:     "Manage veyron projects.",
	Children: []*cmdline.Command{cmdProjectList, cmdProjectPoll},
}

// cmdProjectList represents the 'list' sub-command of the
// 'project' command of the veyron tool.
var cmdProjectList = &cmdline.Command{
	Run:   runProjectList,
	Name:  "list",
	Short: "List existing veyron projects",
	Long:  "Inspect the local filesystem and list the existing projects.",
}

// runProjectList generates a human-readable description of
// existing projects.
func runProjectList(command *cmdline.Command, _ []string) error {
	ctx := util.NewContext(verboseFlag, command.Stdout(), command.Stderr())
	return util.ListProjects(ctx, branchesFlag)
}

// cmdProjectPoll represents the 'poll' sub-command of the 'project'
// command of the veyron tool.
var cmdProjectPoll = &cmdline.Command{
	Run:   runProjectPoll,
	Name:  "poll",
	Short: "Poll existing veyron projects",
	Long: `
Poll existing veyron projects and report whether any new changes exist.
Projects to poll can be specified as command line arguments.
If no projects are specified, all projects are polled by default.
`,
	ArgsName: "<project ...>",
	ArgsLong: "<project ...> is a list of projects to poll.",
}

// runProjectPoll generates a description of changes that exist
// remotely but do not exist locally.
func runProjectPoll(command *cmdline.Command, args []string) error {
	// Get all the changes.
	ctx := util.NewContext(verboseFlag, command.Stdout(), command.Stderr())
	update, err := util.PollProjects(ctx, manifestFlag)
	if err != nil {
		return err
	}

	// Remove repos with empty changes.
	for repo := range update {
		if changes := update[repo]; len(changes) == 0 {
			delete(update, repo)
		}
	}

	// Get poll configs from veyron config file and get the set of repos to report new changes for.
	if len(args) > 0 {
		config, err := util.VeyronConfig()
		if err != nil {
			return err
		}
		repoSet := map[string]struct{}{}
		for _, arg := range args {
			curRepos, ok := config.PollConfig[arg]
			if !ok {
				return fmt.Errorf("failed to find the key %q in %#v", arg, config.PollConfig)
			}
			for _, repo := range curRepos {
				repoSet[repo] = struct{}{}
			}
		}
		repos := []string{}
		for repo := range repoSet {
			repos = append(repos, repo)
		}

		filteredUpdate := util.Update{}
		for _, repo := range repos {
			if changes, ok := update[repo]; ok {
				filteredUpdate[repo] = changes
			}
		}
		update = filteredUpdate
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

// cmdUpdate represents the 'update' command of the veyron tool.
var cmdUpdate = &cmdline.Command{
	Run:   runUpdate,
	Name:  "update",
	Short: "Update all veyron tools and projects",
	Long: `
Updates all veyron tools to their latest version, installing them
into $VEYRON_ROOT/bin, and then updates all veyron projects. The
sequence in which the individual updates happen guarantees that we
end up with a consistent set of tools and source code.
`,
}

func runUpdate(command *cmdline.Command, _ []string) error {
	ctx := util.NewContext(verboseFlag, command.Stdout(), command.Stderr())
	return util.UpdateUniverse(ctx, manifestFlag, gcFlag)
}
