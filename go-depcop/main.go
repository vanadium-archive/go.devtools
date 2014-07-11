package main

import (
	"fmt"
	"go/build"
	"os"
)

func usage() {
	fmt.Println("Usage: go-depcop [--list|-l|--recursive|-r] package1 [package2...]")
	os.Exit(9)
}

// listPackages handles `go-dep --list package` command
func listPackages(args []string) {
	if len(args) == 0 {
		usage()
	}

	for _, arg := range args {
		p, err := ImportPackage(arg)
		if err == nil {
			err = printDependencyHierarchy(p, map[*build.Package]bool{}, 0)
		}
		if err != nil {
			fmt.Println(err)
			os.Exit(2)
		}
	}
	os.Exit(0)
}

func main() {
	// TODO: Parse command line arguments using the cmdline library
	args := os.Args[1:]

	if len(args) == 0 {
		usage()
	}

	recurse := false
	if args[0] == "--list" || args[0] == "-l" {
		listPackages(args[1:])
	} else if args[0] == "--recursive" || args[0] == "-r" {
		recurse = true
		args = args[1:]
	} else if args[0] == "-" {
		args = args[1:]
	}

	if len(args) == 0 {
		usage()
	}

	violations := []DependencyRuleReference{}

	for _, arg := range args {
		p, err := ImportPackage(arg)
		if err == nil {
			var v []DependencyRuleReference
			v, err = verifyDependencyHierarchy(p, map[*build.Package]bool{}, nil, recurse)
			violations = append(violations, v...)
		}
		if err != nil {
			fmt.Println(err)
			os.Exit(2)
		}
	}

	for _, v := range violations {
		switch v.Direction {
		case OutgoingDependency:
			fmt.Printf("%q violates its outgoing rule by depending on %q:\n    {\"deny\": %q} (in %s)\n", v.Package.ImportPath, v.MatchingPackage.ImportPath, v.RuleSet[v.RuleIndex].PackageExpression, v.Path)
		case IncomingDependency:
			fmt.Printf("%q violates incoming rule of package %q:\n    {\"deny\": %q} (in %s)\n", v.MatchingPackage.ImportPath, v.Package.ImportPath, v.RuleSet[v.RuleIndex].PackageExpression, v.Path)
		}
	}

	if len(violations) > 0 {
		os.Exit(1)
	}

	os.Exit(0)
}
