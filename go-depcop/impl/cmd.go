package impl

import (
	"fmt"
	"go/build"

	"tools/lib/cmdline"
	"tools/lib/util"
)

var (
	gorootFlag     bool
	manifestFlag   string
	prettyFlag     bool
	recursiveFlag  bool
	transitiveFlag bool
	verboseFlag    bool
)

func init() {
	cmdCheck.Flags.BoolVar(&recursiveFlag, "r", false, "Check dependencies recursively.")
	cmdList.Flags.BoolVar(&prettyFlag, "pretty-print", false, "Make output easy to read, indenting nested dependencies.")
	cmdList.Flags.BoolVar(&gorootFlag, "show-goroot", false, "Show packages in goroot.")
	cmdList.Flags.BoolVar(&transitiveFlag, "transitive", false, "List transitive dependencies.")
	cmdRoot.Flags.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
	cmdSelfUpdate.Flags.StringVar(&manifestFlag, "manifest", "absolute", "Name of the project manifest.")
}

// Root returns a command that represents the root of the go-depcop tool.
func Root() *cmdline.Command {
	return cmdRoot
}

var cmdRoot = &cmdline.Command{
	Name:  "go-depcop",
	Short: "Command-line tool for checking Go dependencies",
	Long: `
The go-depcop tool checks if a package imports respects outgoing and
incoming dependency constraints described in the GO.PACKAGE files.

go-depcop also enforces "internal" package rules.

GO.PACKAGE files are traversed hierarchically, from the deepmost
package to GOROOT, until a matching rule is found.  If no matching
rule is found, the default behavior is to allow the dependency,
to stay compatible with existing packages that do not include
dependency rules.

GO.PACKAGE is a JSON file with a structure along the lines of:

{
	"dependencies": {
		"outgoing": {
			{"allow": "allowpattern1/..."},
			{"deny": "denypattern"},
			{"allow": "pattern2"}
		},
		"incoming": {
			{"allow": "pattern3"},
			{"deny": "pattern4"}
		}
	}
}
`,
	Children: []*cmdline.Command{cmdCheck, cmdList, cmdRevList, cmdSelfUpdate, cmdVersion},
}

// cmdCheck represents the 'check' command of the go-depcop tool.
var cmdCheck = &cmdline.Command{
	Run:      runCheck,
	Name:     "check",
	Short:    "Check package dependency constraints",
	Long:     "Check package dependency constraints.",
	ArgsName: "<packages>",
	ArgsLong: "<packages> is a list of packages",
}

func runCheck(command *cmdline.Command, args []string) error {
	violations := []DependencyRuleReference{}

	for _, arg := range args {
		p, err := ImportPackage(arg)
		if err != nil {
			return err
		}
		var v []DependencyRuleReference
		v, err = verifyDependencyHierarchy(p, map[*build.Package]bool{}, nil, recursiveFlag)
		if err != nil {
			return err
		}
		violations = append(violations, v...)
	}

	for _, v := range violations {
		switch v.Direction {
		case OutgoingDependency:
			fmt.Printf("%q violates its outgoing rule by depending on %q:\n    {\"deny\": %q} (in %s)\n",
				v.Package.ImportPath, v.MatchingPackage.ImportPath, v.RuleSet[v.RuleIndex].PackageExpression, v.Path)
		case IncomingDependency:
			if v.InternalPackage {
				fmt.Printf("%q is inaccessible by package %q because it is internal\n", v.Package.ImportPath, v.MatchingPackage.ImportPath)
			} else {
				fmt.Printf("%q violates incoming rule of package %q:\n    {\"deny\": %q} (in %s)\n",
					v.MatchingPackage.ImportPath, v.Package.ImportPath, v.RuleSet[v.RuleIndex].PackageExpression, v.Path)
			}
		}
	}

	if len(violations) > 0 {
		return fmt.Errorf("dependency violation")
	}

	return nil
}

// cmdList represents the 'list' command of the go-depcop tool.
var cmdList = &cmdline.Command{
	Run:      runList,
	Name:     "list",
	Short:    "List outgoing package dependencies",
	Long:     "List outgoing package dependencies.",
	ArgsName: "<packages>",
	ArgsLong: "<packages> is a list of packages",
}

func runList(command *cmdline.Command, args []string) error {
	if len(args) == 0 {
		command.Errorf("not enough arguments")
	}

	for _, arg := range args {
		p, err := ImportPackage(arg)
		if err != nil {
			return err
		}
		if err := printDependencyHierarchy(p, map[*build.Package]bool{}, 0); err != nil {
			return err
		}
	}
	return nil
}

// cmdRevList represents the 'rlist' command of the go-depcop tool.
var cmdRevList = &cmdline.Command{
	Run:      runRevList,
	Name:     "rlist",
	Short:    "List incoming package dependencies",
	Long:     "List incoming package dependencies.",
	ArgsName: "<packages>",
	ArgsLong: "<packages> is a list of packages",
}

// TODO(jsimsa): Implement transitive incoming dependencies as a
// fix-point.
func runRevList(command *cmdline.Command, args []string) error {
	if len(args) == 0 {
		command.Errorf("not enough arguments")
	}
	revDeps, err := computeIncomingDependencies()
	if err != nil {
		return err
	}
	for _, arg := range args {
		if deps, ok := revDeps[arg]; !ok {
			fmt.Fprintf(command.Stderr(), "package %v not found\n", arg)
		} else {
			for dep, _ := range deps {
				fmt.Fprintf(command.Stdout(), "%v\n", dep)
			}
		}
	}
	return nil
}

// cmdSelfUpdate represents the 'selfupdate' command of the go-depcop
// tool.
var cmdSelfUpdate = &cmdline.Command{
	Run:   runSelfUpdate,
	Name:  "selfupdate",
	Short: "Update the go-depcop tool",
	Long:  "Download and install the latest version of the go-depcop tool.",
}

func runSelfUpdate(command *cmdline.Command, args []string) error {
	return util.SelfUpdate(verboseFlag, manifestFlag, "go-depcop")
}

// cmdVersion represent the 'version' command of the go-depcop tool.
var cmdVersion = &cmdline.Command{
	Run:   runVersion,
	Name:  "version",
	Short: "Print version",
	Long:  "Print version of the go-depcop tool.",
}

const version string = "0.1.0"

// commitId should be over-written during build:
// go build -ldflags "-X tools/go-depcop/impl.commitId <commitId>" tools/go-depcop
var commitId string = "test-build"

func runVersion(cmd *cmdline.Command, args []string) error {
	fmt.Printf("go-depcop tool version %v (build %v)\n", version, commitId)
	return nil
}
