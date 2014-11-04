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
	ctx := util.NewContextFromCommand(command, verboseFlag)
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
	projectSet := map[string]struct{}{}
	if len(args) > 0 {
		config, err := util.VeyronConfig()
		if err != nil {
			return err
		}
		for _, arg := range args {
			curProjects, ok := config.PollMap[arg]
			if !ok {
				return fmt.Errorf("failed to find the key %q in %#v", arg, config.PollMap)
			}
			for _, project := range curProjects {
				projectSet[project] = struct{}{}
			}
		}
	}
	ctx := util.NewContextFromCommand(command, verboseFlag)
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

// cmdUpdate represents the 'update' command of the veyron tool.
var cmdUpdate = &cmdline.Command{
	Run:   runUpdate,
	Name:  "update",
	Short: "Update all veyron tools and projects",
	Long: `
Updates all veyron projects, builds the latest version of veyron
tools, and installs the resulting binaries into $VEYRON_ROOT/bin. The
sequence in which the individual updates happen guarantees that we end
up with a consistent set of tools and source code.

The set of project and tools to update is describe by a
manifest. Veyron manifests are revisioned and stored in a "manifest"
repository, that is available locally in $VEYRON_ROOT/.manifest. The
manifest uses the following XML schema:

 <manifest>
   <imports>
     <import name="default"/>
     ...
   </imports>
   <projects>
     <project name="https://veyron.googlesource.com/veyrong.go"
              path="veyron/go/src/veyron.io/veyron"
              protocol="git"
              revision="HEAD"/>
     ...
   </projects>
   <tools>
     <tool name="veyron" package="tools/veyron"/>
     ...
   </tools>
 </manifest>

The <import> element can be used to share settings across multiple
manifests. Import names are interpreted relative to the
$VEYRON_ROOT/.manifest/v1 directory. Import cycles are not allowed and
if a project or a tool is specified multiple times, the last
specification takes effect. In particular, the elements <project
name="foo" exclude="true"/> and <tool name="bar" exclude="true"/> can
be used to exclude previously included projects and tools.

The tool identifies which manifest to use using the following
algorithm. If the $VEYRON_ROOT/.local_manifest file exists, then it is
used. Otherwise, the $VEYRON_ROOT/.manifest/v1/<manifest>.xml file is
used, which <manifest> is the value of the -manifest command-line
flag, which defaults to "default".

NOTE: Unlike the veyron tool commands, the above manifest file format
is not an API. It is an implementation and can change without notice.
`,
}

func runUpdate(command *cmdline.Command, _ []string) error {
	ctx := util.NewContextFromCommand(command, verboseFlag)
	return util.UpdateUniverse(ctx, manifestFlag, gcFlag)
}
