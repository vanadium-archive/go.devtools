package main

import (
	"fmt"
	"io"
	"regexp"

	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/go/types"

	"veyron.io/lib/cmdline"
	"veyron.io/tools/lib/version"
)

var (
	interfaceName string
	regexpArg     string
	conf          loader.Config
	debug         bool
	onlyFiles     bool
	onlyTypes     bool
	verboseFlag   bool
)

func init() {
	cmdRoot.Flags.BoolVar(&debug, "debug", false, "Toggle debugging.")
	cmdRoot.Flags.BoolVar(&onlyFiles, "only_files", false, "Show only files.")
	cmdRoot.Flags.BoolVar(&onlyTypes, "only_types", false, "Show only types, takes precedence over only_files.")
	cmdRoot.Flags.StringVar(&interfaceName, "interface", "", "Name of the interface.")
	cmdRoot.Flags.StringVar(&regexpArg, "regexp", "", "Look for implementations in packages matching this filter.")
	cmdRoot.Flags.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
}

// root returns a command that represents the root of the veyron tool.
func root() *cmdline.Command {
	return cmdRoot
}

// cmdRoot represents the root of the find-impl tool.
var cmdRoot = &cmdline.Command{
	Run:   runRoot,
	Name:  "find-impl",
	Short: "Tool for finding interface implementations",
	Long: `
A simple tool to find implementations of a specified interface.

It uses golang.org/x/tools/{loader,types} to load and examine
the types of a collection of go files. The input must be valid go
packages.

find-impl --interface=veyron2/security.Context <packages> will find
all implementations of veyron2/security.Context in the specified
packages.

A common use case will be:
cd <repo>/go/src
find-impl --interface=<package>.<interface> $(find * -type d)

The output is of the form:
<type> in <file>
no attempt is made to sort or dedup this output, rather existing tools
such as awk, sort, uniq should be used.

spaces in package names will confound simple scripts and the flags
--only_files, --only_types should be used in such cases to produce
output with one type or one filename per line.

The implementation is a brute force approach, taking two passes the
first over the entire space of types to find the named interface and
then a second over the set of implicit conversions to see which ones
can be implemented using the type found in the first pass. This
appears to be fast enough for our immediate needs.
`,
	ArgsName: "<pkg ...>",
	ArgsLong: "<pkg ...> a list of packages to search",
	Children: []*cmdline.Command{cmdVersion},
}

// TODO(cnicolaou): add some shortcuts for using directory names,
// ./... etc as per the 'go build' command.
func runRoot(command *cmdline.Command, args []string) error {
	re, err := regexp.Compile(regexpArg)
	if err != nil {
		return fmt.Errorf("failed to compile re: %q: %v", regexpArg, err)
	}
	if _, err := conf.FromArgs(args, false); err != nil {
		return err
	}
	prog, err := conf.Load()
	if err != nil {
		return fmt.Errorf("error loading packages: %v", err)
	}
	interfaceObject := findInterface(command.Stdout(), re, prog.AllPackages, interfaceName)
	if interfaceObject == nil {
		return fmt.Errorf("failed to find interface %q", interfaceName)
	}
	findImplementations(command.Stdout(), re, interfaceObject, prog)
	return nil
}

func findInterface(stdout io.Writer, re *regexp.Regexp, imports map[*types.Package]*loader.PackageInfo, interfaceName string) types.Object {
	for k, v := range imports {
		if !re.MatchString(k.Path()) {
			if debug {
				fmt.Fprintf(stdout, "Filtered %s\n", k.Path())
			}
			continue
		}
		if debug {
			fmt.Fprintf(stdout, "Import %s\n", k.Path())
		}
		for _, object := range v.Defs {
			if object == nil {
				continue
			}
			name := k.Path() + "." + object.Name()
			typ := object.Type().Underlying()
			if typ == nil {
				continue
			}
			if _, ok := typ.(*types.Interface); ok && name == interfaceName {
				return object
			}
		}
	}
	return nil
}

func findImplementations(stdout io.Writer, re *regexp.Regexp, ifObject types.Object, prog *loader.Program) {
	ifType := ifObject.Type().Underlying().(*types.Interface)
	for k, v := range prog.AllPackages {
		if !re.MatchString(k.Path()) {
			continue
		}
		for _, object := range v.Implicits {
			if types.Implements(object.Type(), ifType) {
				position := prog.Fset.Position(object.Pos())
				if debug {
					fmt.Fprintf(stdout, "Path: %s\n", k.Path())
					fmt.Fprintf(stdout, "Type: %s\n", object.Type())
					fmt.Fprintf(stdout, "Underlying type: %s\n", object.Type().Underlying())
					fmt.Fprintf(stdout, "Position: %s\n", position)
				}
				switch {
				case onlyTypes:
					fmt.Fprintln(stdout, object.Type())
				case onlyFiles:
					fmt.Fprintln(stdout, position.Filename)
				default:
					fmt.Fprintf(stdout, "%s in %s\n", object.Type(), position.Filename)
				}
			}
		}
	}
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
