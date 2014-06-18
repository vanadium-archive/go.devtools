// A simple tool to find implementations of a specified interface.
//
// It uses code.google.com/p/go.tools/{loader,types} to load and examine
// the types of a collection of go files. The input must be valid go packages.
//
// find_impl --interface=veyron2/security.Context <packages>
// will find all implementations of veyron2/security.Context in the
// specified packages.
//
// A common use case will be:
// cd <repo>/go/src
// find_impl --interface=<package>.<interface> $(find * -type d)
//
// The output is of the form:
// <type> in <file>
// no attempt is made to sort or dedup this output, rather existing tools
// such as awk, sort, uniq should be used.
//
// spaces in package names will confound simple scripts and the flags
// --only_files, --only_types should be used in such cases to produce
// output with one type or one filename per line.
//
// TODO(cnicolaou): add some shortcuts for using directory names, ./... etc
// as per the 'go build' command.
//
// The implementation is a brute force approach, taking two passes
// the first over the entire space of types to find the named interface and
// then a second over the set of implicit conversions to see which ones
// can be implemented using the type found in the first pass. This appears
// to be fast enough for our immediate needs.
package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"

	"code.google.com/p/go.tools/go/loader"
	"code.google.com/p/go.tools/go/types"
)

var (
	interfaceName string
	regexpArg     string
	conf          loader.Config
	debug         bool
	onlyFiles     bool
	onlyTypes     bool
)

func init() {
	flag.BoolVar(&debug, "debug", false, "toggle debugging")
	flag.BoolVar(&onlyFiles, "only_files", false, "show only files")
	flag.BoolVar(&onlyTypes, "only_types", false, "show only types, takes precedence over only_files")
	flag.StringVar(&interfaceName, "interface", "", "name of the interface")
	flag.StringVar(&regexpArg, "regexp", "", "look for implementations in packages matching this filter")

}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s: <flags> <args>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "<flags> ")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, loader.FromArgsUsage)
	}
	flag.Parse()
	re, err := regexp.Compile(regexpArg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to compile re: %q: %v\n", regexpArg, err)
		os.Exit(1)
	}
	if _, err := conf.FromArgs(flag.Args(), true); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	prog, err := conf.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading packages: %v\n", err)
		os.Exit(1)
	}
	interfaceObject := findInterface(re, prog.AllPackages, interfaceName)
	if interfaceObject == nil {
		fmt.Fprintf(os.Stderr, "failed to find interface %q\n", interfaceName)
		os.Exit(1)
	}
	findImplementations(re, interfaceObject, prog)
}

func findInterface(re *regexp.Regexp, imports map[*types.Package]*loader.PackageInfo, interfaceName string) types.Object {
	for k, v := range imports {
		if !re.MatchString(k.Path()) {
			if debug {
				fmt.Printf("Filtered %s\n", k.Path())
			}
			continue
		}
		if debug {
			fmt.Printf("Import %s\n", k.Path())
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

func findImplementations(re *regexp.Regexp, ifObject types.Object, prog *loader.Program) {
	ifType := ifObject.Type().Underlying().(*types.Interface)
	for k, v := range prog.AllPackages {
		if !re.MatchString(k.Path()) {
			continue
		}
		for _, object := range v.Implicits {
			if types.Implements(object.Type(), ifType) {
				position := prog.Fset.Position(object.Pos())
				if debug {
					fmt.Printf("Path: %s\n", k.Path())
					fmt.Printf("Type: %s\n", object.Type())
					fmt.Printf("Underlying type: %s\n", object.Type().Underlying())
					fmt.Printf("Position: %s\n", position)
				}
				switch {
				case onlyTypes:
					fmt.Println(object.Type())
				case onlyFiles:
					fmt.Println(position.Filename)
				default:
					fmt.Printf("%s in %s\n", object.Type(), position.Filename)
				}
			}
		}
	}
}
