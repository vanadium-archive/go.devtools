// The following enables go generate to generate the doc.go file.
//go:generate go run $VANADIUM_ROOT/veyron/go/src/v.io/lib/cmdline/testdata/gendoc.go . -help
package main

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/go/types"
	"v.io/lib/cmdline"
)

func main() {
	cmdGoPkg.Main()
}

var cmdGoPkg = &cmdline.Command{
	Run:   runGoPkg,
	Name:  "gopkg",
	Short: "Print information about go package(s)",
	Long: `
Print information about go package(s).

Example of printing all top-level information about the vdl package:
  veyron run gopkg v.io/veyron/veyron2/vdl

Example of printing the names of all Test* funcs from the vdl package:
  veyron run gopkg -test -kind=func -name_re 'Test.*' -type_re 'func\(.*testing\.T\)' -noheader -notype v.io/veyron/veyron2/vdl
`,
	ArgsName: "<args>",
	ArgsLong: loader.FromArgsUsage,
}

var (
	flagTest     bool
	flagNoHeader bool
	flagNoName   bool
	flagNoType   bool
	flagKind     Kinds = KindAll
	flagNameRE   string
	flagTypeRE   string
)

func init() {
	cmdGoPkg.Flags.BoolVar(&flagTest, "test", false, "Load test code (*_test.go) for packages.")
	cmdGoPkg.Flags.BoolVar(&flagNoHeader, "noheader", false, "Don't print headers.")
	cmdGoPkg.Flags.BoolVar(&flagNoName, "noname", false, "Don't print identifier names.")
	cmdGoPkg.Flags.BoolVar(&flagNoType, "notype", false, "Don't print type descriptions.")
	cmdGoPkg.Flags.Var(&flagKind, "kind", "Print information for the specified kinds, in the order listed.")
	cmdGoPkg.Flags.StringVar(&flagNameRE, "name_re", ".*", "Filter out identifier names that don't match this regexp.")
	cmdGoPkg.Flags.StringVar(&flagTypeRE, "type_re", ".*", "Filter out type descriptions that don't match this regexp.")
}

func parseRegexp(expr string) (*regexp.Regexp, error) {
	// Make sure the regexp performs a full match against the target string.
	expr = strings.TrimSpace(expr)
	if !strings.HasPrefix(expr, "^") {
		expr = "^" + expr
	}
	if !strings.HasSuffix(expr, "$") {
		expr = expr + "$"
	}
	return regexp.Compile(expr)
}

func runGoPkg(cmd *cmdline.Command, args []string) error {
	// Parse flags.
	nameRE, err := parseRegexp(flagNameRE)
	if err != nil {
		return err
	}
	typeRE, err := parseRegexp(flagTypeRE)
	if err != nil {
		return err
	}
	// Load packages specified in args.
	config := loader.Config{
		SourceImports:       false,
		TypeCheckFuncBodies: func(string) bool { return false },
	}
	args, err = config.FromArgs(args, flagTest)
	if err != nil {
		return cmd.UsageErrorf("failed to parse args: %v", err)
	}
	if len(args) != 0 {
		return cmd.UsageErrorf("unrecognized args %q", args)
	}
	prog, err := config.Load()
	if err != nil {
		return err
	}
	// Print information for each loaded package.
	for _, pkginfo := range prog.InitialPackages() {
		pkg := pkginfo.Pkg
		if !flagNoHeader {
			fmt.Fprintf(cmd.Stdout(), "%s (%s)\n", pkg.Path(), pkg.Name())
		}
		scope := pkg.Scope()
		data := make(map[Kind][]NameType)
		for _, name := range scope.Names() {
			if !nameRE.MatchString(name) {
				continue
			}
			kind, nt := NameTypeFromObject(scope.Lookup(name))
			if !typeRE.MatchString(nt.Type) {
				continue
			}
			data[kind] = append(data[kind], nt)
		}
		for _, kind := range flagKind {
			if !flagNoHeader {
				fmt.Fprintf(cmd.Stdout(), "%ss\n", strings.Title(kind.String()))
			}
			for _, nt := range data[kind] {
				var line string
				if !flagNoName {
					line += " " + nt.Name
				}
				if !flagNoType {
					line += " " + nt.Type
				}
				line = strings.TrimSpace(line)
				if line != "" {
					fmt.Fprintf(cmd.Stdout(), "  %s\n", line)
				}
			}
		}
	}
	return nil
}

// NameType holds the name and type of a top-level declaration.
type NameType struct {
	Name string
	Type string
}

func NameTypeFromObject(obj types.Object) (Kind, NameType) {
	var kind Kind
	switch obj.(type) {
	case *types.Const:
		kind = Const
	case *types.Var:
		kind = Var
	case *types.Func:
		kind = Func
	case *types.TypeName:
		kind = Type
	default:
		panic(fmt.Errorf("unhandled types.Object %#v", obj))
	}
	return kind, NameType{obj.Name(), obj.Type().String()}
}

// Kind describes the kind of a top-level declaration, usable as a flag.
type Kind int

// Kinds holds a slice of Kind, usable as a flag.
type Kinds []Kind

const (
	Const Kind = iota // Top-level const declaration.
	Var               // Top-level var declaration.
	Func              // Top-level func declaration.
	Type              // Top-level type declaration.
)

var KindAll = Kinds{Const, Var, Func, Type}

func KindFromString(s string) (k Kind, err error) {
	err = k.Set(s)
	return
}

func (k *Kind) Set(s string) error {
	switch s {
	case "const":
		*k = Const
		return nil
	case "var":
		*k = Var
		return nil
	case "func":
		*k = Func
		return nil
	case "type":
		*k = Type
		return nil
	default:
		*k = -1
		return fmt.Errorf("unknown Kind %q", s)
	}
}

func (k Kind) String() string {
	switch k {
	case Const:
		return "const"
	case Var:
		return "var"
	case Func:
		return "func"
	case Type:
		return "type"
	default:
		return fmt.Sprintf("Kind(%d)", k)
	}
}

func (kinds *Kinds) Set(s string) error {
	*kinds = nil
	seen := make(map[Kind]bool)
	for _, kindstr := range strings.Split(s, ",") {
		if kindstr == "" {
			continue
		}
		k, err := KindFromString(kindstr)
		if err != nil {
			return err
		}
		if !seen[k] {
			seen[k] = true
			*kinds = append(*kinds, k)
		}
	}
	return nil
}

func (kinds Kinds) String() string {
	var strs []string
	for _, k := range kinds {
		strs = append(strs, k.String())
	}
	return strings.Join(strs, ",")
}
