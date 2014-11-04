package main

import (
	"flag"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"tools/lib/cmdline"
	"tools/lib/gitutil"
	"tools/lib/runutil"
	"tools/lib/util"
	"tools/lib/version"
)

var (
	hostGo             string
	targetGo           string
	branchesFlag       bool
	gcFlag             bool
	manifestFlag       string
	novdlFlag          bool
	platformFlag       string
	numTestWorkersFlag int
	verboseFlag        bool
)

func init() {
	flag.StringVar(&hostGo, "host-go", "go", "Go command for the host platform.")
	flag.StringVar(&targetGo, "target-go", "go", "Go command for the target platform.")
	cmdProjectList.Flags.BoolVar(&branchesFlag, "branches", false, "Show project branches.")
	cmdUpdate.Flags.BoolVar(&gcFlag, "gc", false, "Garbage collect obsolete repositories.")
	cmdUpdate.Flags.StringVar(&manifestFlag, "manifest", "default", "Name of the project manifest.")
	cmdGo.Flags.BoolVar(&novdlFlag, "novdl", false, "Disable automatic generation of vdl files.")
	cmdXGo.Flags.BoolVar(&novdlFlag, "novdl", false, "Disable automatic generation of vdl files.")
	cmdEnv.Flags.StringVar(&platformFlag, "platform", "", "Target platform.")
	cmdIntegrationTestRun.Flags.IntVar(&numTestWorkersFlag, "workers", 0, "Number of test workers. The default 0 matches the number of CPUs.")
	cmdRoot.Flags.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
}

// root returns a command that represents the root of the veyron tool.
func root() *cmdline.Command {
	return cmdRoot
}

// cmdRoot represents the root of the veyron tool.
var cmdRoot = &cmdline.Command{
	Name:  "veyron",
	Short: "Tool for managing veyron development",
	Long:  "The veyron tool helps manage veyron development.",
	Children: []*cmdline.Command{
		cmdBuild,
		cmdContributors,
		cmdProfile,
		cmdProject,
		cmdUpdate,
		cmdEnv,
		cmdRun,
		cmdGo,
		cmdGoExt,
		cmdXGo,
		cmdIntegrationTest,
		cmdVersion,
	},
}

// cmdContributors represents the 'contributors' command of the veyron tool.
var cmdContributors = &cmdline.Command{
	Run:   runContributors,
	Name:  "contributors",
	Short: "List veyron project contributors",
	Long: `
Lists veyron project contributors and the number of their
commits. Veyron projects to consider can be specified as an
argument. If no projects are specified, all veyron projects are
considered by default.
`,
	ArgsName: "<projects>",
	ArgsLong: "<projects> is a list of projects to consider.",
}

func runContributors(command *cmdline.Command, args []string) error {
	ctx := util.NewContextFromCommand(command, verboseFlag)
	projects, err := util.LocalProjects(ctx)
	if err != nil {
		return err
	}
	repos := map[string]struct{}{}
	if len(args) != 0 {
		for _, arg := range args {
			repos[arg] = struct{}{}
		}
	} else {
		for name, _ := range projects {
			repos[name] = struct{}{}
		}
	}
	contributors := map[string]int{}
	for repo, _ := range repos {
		project, ok := projects[repo]
		if !ok {
			continue
		}
		if err := ctx.Run().Function(runutil.Chdir(project.Path)); err != nil {
			return err
		}
		switch project.Protocol {
		case "git":
			lines, err := listCommitters(ctx.Git())
			if err != nil {
				return err
			}
			for _, line := range lines {
				tokens := strings.SplitN(line, "\t", 2)
				n, err := strconv.Atoi(strings.TrimSpace(tokens[0]))
				if err != nil {
					return fmt.Errorf("Atoi(%v) failed: %v", tokens[0], err)
				}
				contributors[strings.TrimSpace(tokens[1])] += n
			}
		default:
		}
	}
	names := []string{}
	for name, _ := range contributors {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(command.Stdout(), "%4d %v\n", contributors[name], name)
	}
	return nil
}

func listCommitters(git *gitutil.Git) ([]string, error) {
	branch, err := git.CurrentBranchName()
	if err != nil {
		return nil, err
	}
	stashed, err := git.Stash()
	if err != nil {
		return nil, err
	}
	if stashed {
		defer git.StashPop()
	}
	if err := git.CheckoutBranch("master", !gitutil.Force); err != nil {
		return nil, err
	}
	defer git.CheckoutBranch(branch, !gitutil.Force)
	return git.Committers()
}

// cmdVersion represents the 'version' command of the veyron tool.
var cmdVersion = &cmdline.Command{
	Run:   runVersion,
	Name:  "version",
	Short: "Print version",
	Long:  "Print version of the veyron tool.",
}

func runVersion(command *cmdline.Command, _ []string) error {
	fmt.Fprintf(command.Stdout(), "veyron tool version %v\n", version.Version)
	return nil
}
