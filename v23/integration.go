package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"strings"

	"v.io/lib/cmdline"
	"v.io/tools/lib/collect"
)

// cmdIntegration represents the "v23 integration" command
var cmdIntegration = &cmdline.Command{
	Name:     "integration",
	Short:    "Manage vanadium integration test support",
	Long:     "Manage vanadium integration test support",
	Children: []*cmdline.Command{cmdIntegrationGenerate},
}

var cmdIntegrationGenerate = &cmdline.Command{
	Run:   runIntegrationGenerate,
	Name:  "generate",
	Short: "Generates supporting code for vanadium tests.",
	Long: `
The v23 integration subcommand supports the vanadium integration test
framework and unit tests by generating go files that contain supporting
code. v23 integration generate is intended to be invoked via the
'go generate' mechanism and the resulting files are to be checked in.

Integration tests are functions of the form shown below that are defined
in 'external' tests (i.e. those occurring in _test packages, rather than
being part of the package being tested). This ensures that integration
tests are isolated from the packages being tested and can be moved to their
own package if need be. Integration tests have the following form:

    func V23Test<x> (i integration.T)

    'v23 integration generate' operates as follows:

In addition, some commonly used functionality in vanadium unit tests
is streamlined. Arguably this should be in a separate command/file but
for now they are lumped together. The additional functionality is as
follows:

1. v.io/veyron/lib/modules requires the use of an explicit
   registration mechanism. 'v23 integration generate' automatically
   generates these registration functions for any test function matches
   the modules.Main signature.

   For:
   // SubProc does the following...
   // Usage: <a> <b>...
   func SubProc(stdin io.Reader, stdout, stderr io.Writer, env map[string]string, args ...string) error

   It will generate:

   modules.RegisterChild("SubProc",` + "`" + `SubProc does the following...
Usage: <a> <b>...` + "`" + `, SubProc)

2. 'TestMain' is used as the entry point for all vanadium tests, integration
   and otherwise. v23 will generate an appropriate version of this if one is
   not already defined. TestMain is 'special' in that only one definiton can
   occur across both the internal and external test packages. This is a
   consequence of how the go testing system is implemented.
`,

	// TODO(cnicolaou): once the initial deployment is done, revisit the
	// this functionality and possibly dissallow the 'if this doesn't exist
	// generate it' behaviour and instead always generate the required helper
	// functions.

	ArgsName: "[packages]",
	ArgsLong: "list of go packages"}

var (
	outputFileName string
)

func init() {
	cmdIntegrationGenerate.Flags.StringVar(&outputFileName, "output", "v23_test.go", "name of output files; two files are generated, <file_name> and internal_<file_name>.")
}

func runIntegrationGenerate(command *cmdline.Command, args []string) error {
	// TODO(cnicolaou): use http://godoc.org/golang.org/x/tools/go/loader
	// to replace accessing the AST directly. In the meantime make sure
	// the command line API is consistent with that change.

	if len(args) > 1 || (len(args) == 1 && args[0] != ".") {
		return command.UsageErrorf("unexpected or wrong arguments, currently only . is supported as a package name.")
	}
	fi, err := ioutil.ReadDir(".")
	if err != nil {
		return err
	}
	candidates := []string{}
	re := regexp.MustCompile(".*_test.go")
	for _, f := range fi {
		if f.IsDir() {
			continue
		}
		if re.MatchString(f.Name()) {
			candidates = append(candidates, f.Name())
		}
	}

	integrationTests := []string{}

	internalModules := []moduleCommand{}
	externalModules := []moduleCommand{}

	hasTestMain := false
	packageName := ""

	re = regexp.MustCompile(`V23Test(.*)`)
	fset := token.NewFileSet() // positions are relative to fset
	for _, file := range candidates {
		// Ignore the files we are generating.
		if file == outputFileName || file == "internal_"+outputFileName {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
		if err != nil {
			return err
		}

		// An external test package is one named <pkg>_test.
		isExternal := strings.HasSuffix(f.Name.Name, "_test")
		if !isExternal && len(packageName) == 0 {
			packageName = f.Name.Name
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}

			// If this function matches the declaration for modules.Main,
			// keep track of the names and comments associated with
			// such functions so that we can generate calls to
			// modules.RegisterChild for them.
			if n, c := isModulesMain(fn); len(n) > 0 {

				if isExternal {
					externalModules = append(externalModules, moduleCommand{n, c})
				} else {
					internalModules = append(internalModules, moduleCommand{n, c})

				}
			}

			// If this function is the testing TestMain then
			// keep track of the fact that we've seen it.
			if isTestMain(fn) {
				hasTestMain = true
			}
			name := fn.Name.String()
			if parts := re.FindStringSubmatch(name); isExternal && len(parts) == 2 {
				integrationTests = append(integrationTests, parts[1])
			}
		}
	}

	needInternalFile := len(internalModules) > 0
	needExternalFile := len(externalModules) > 0 || len(integrationTests) > 0

	// TestMain is special in that it can only occur once even across
	// internal and external test packages. If if it doesn't occur
	// in either, we want to make sure we write it out in the internal
	// package.
	if !hasTestMain && !needInternalFile && !needExternalFile {
		needInternalFile = true
	}

	if needInternalFile {
		if err := writeInternalFile("internal_"+outputFileName, packageName, !hasTestMain, internalModules); err != nil {
			return err
		}
		hasTestMain = true
	}

	if needExternalFile {
		return writeExternalFile(outputFileName, packageName, !hasTestMain, externalModules, integrationTests)
	}
	return nil
}

func isModulesMain(d ast.Decl) (string, string) {
	fn, ok := d.(*ast.FuncDecl)
	if !ok {
		return "", ""
	}

	if fn.Type == nil || fn.Type.Params == nil || fn.Type.Results == nil {
		return "", ""
	}
	name := fn.Name.Name

	typeNames := func(fl *ast.FieldList) []string {
		names := []string{}
		for _, f := range fl.List {
			switch v := f.Type.(type) {
			case *ast.Ident:
				names = append(names, v.Name)
			case *ast.SelectorExpr:
				// Deal with 'a, b type' parameters.
				for _, _ = range f.Names {
					if pkg, ok := v.X.(*ast.Ident); ok {
						names = append(names, pkg.Name+"."+v.Sel.Name)
					}
				}
			case *ast.MapType:
				if t, ok := v.Key.(*ast.Ident); !ok || t.Name != "string" {
					break
				}
				if t, ok := v.Value.(*ast.Ident); !ok || t.Name != "string" {
					break
				}
				names = append(names, "map[string]string")
			case *ast.Ellipsis:
				if t, ok := v.Elt.(*ast.Ident); !ok || t.Name != "string" {
					break
				}
				names = append(names, "...string")
			}
		}
		return names
	}

	cmp := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i, av := range a {
			if av != b[i] {
				return false
			}
		}
		return true
	}

	comments := func(cg *ast.CommentGroup) string {
		if cg == nil {
			return ""
		}
		c := ""
		for _, l := range cg.List {
			t := strings.TrimPrefix(l.Text, "//")
			c += strings.TrimSpace(t) + "\n"
		}
		return strings.TrimSuffix(c, "\n")
	}

	// the Modules.Main signature is as follows:
	// type Main func(stdin io.Reader, stdout, stderr io.Writer, env map[string]string, args ...string) error
	results := []string{"error"}
	parameters := []string{"io.Reader", "io.Writer", "io.Writer", "map[string]string", "...string"}
	_, _ = results, parameters

	p := typeNames(fn.Type.Params)
	r := typeNames(fn.Type.Results)

	if !cmp(results, r) || !cmp(parameters, p) {
		return "", ""
	}
	return name, comments(fn.Doc)
}

func isTestMain(fn *ast.FuncDecl) bool {
	// TODO(cnicolaou): check the signature as well as the name
	if fn.Name.Name != "TestMain" {
		return false
	}
	return true
}

type moduleCommand struct {
	name, comment string
}

// writeInternalFile writes a generated test file that is inside the package.
// It cannot contain integration tests.
func writeInternalFile(fileName string, packageName string, needsTestMain bool, modules []moduleCommand) (e error) {

	hasModules := len(modules) > 0

	if !needsTestMain && !hasModules {
		return nil
	}

	out, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return out.Close() }, &e)

	fmt.Fprintln(out, "// This file was auto-generated via go generate.")
	fmt.Fprintln(out, "// DO NOT UPDATE MANUALLY")
	fmt.Fprintf(out, "package %s\n\n", packageName)

	if needsTestMain {
		fmt.Fprintln(out, `import "testing"`)
		if needsTestMain {
			fmt.Fprintln(out, `import "os"`)
		}
		fmt.Fprintln(out, "")
	}

	if hasModules {
		fmt.Fprintln(out, `import "v.io/core/veyron/lib/modules"`)
	}

	if needsTestMain {
		fmt.Fprintln(out, `import "v.io/core/veyron/lib/testutil"`)
	}

	if hasModules {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "func init() {")
		writeModuleRegistration(out, modules)
		fmt.Fprintln(out, "}")
	}

	if needsTestMain {
		writeTestMain(out)
	}
	return nil
}

// writeExternalFile write a generated test file that is outside the package.
// It can contain intgreation tests.
func writeExternalFile(fileName string, packageName string, needsTestMain bool, modules []moduleCommand, tests []string) (e error) {

	hasTests := len(tests) > 0
	hasModules := len(modules) > 0
	if !needsTestMain && !hasModules && !hasTests {
		return nil
	}

	out, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return out.Close() }, &e)

	fmt.Fprintln(out, "// This file was auto-generated via go generate.")
	fmt.Fprintln(out, "// DO NOT UPDATE MANUALLY")
	fmt.Fprintf(out, "package %s_test\n\n", packageName)

	if needsTestMain {
		fmt.Fprintln(out, `import "testing"`)
		if needsTestMain {
			fmt.Fprintln(out, `import "os"`)
		}
		fmt.Fprintln(out, "")
	}

	if hasModules {
		fmt.Fprintln(out, `import "v.io/core/veyron/lib/modules"`)
	}

	if needsTestMain {
		fmt.Fprintln(out, `import "v.io/core/veyron/lib/testutil"`)
	}

	if hasTests {
		fmt.Fprintln(out, `import "v.io/core/veyron/lib/testutil/integration"`)
	}

	if hasModules {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "func init() {")
		writeModuleRegistration(out, modules)
		fmt.Fprintln(out, "}")
	}

	if needsTestMain {
		writeTestMain(out)
	}

	// integration test wrappers.
	for _, t := range tests {
		fmt.Fprintf(out, "\nfunc TestV23%s(t *testing.T) {\n", t)
		fmt.Fprintf(out, "\tintegration.RunTest(t, V23Test%s)\n}\n", t)
	}
	return nil
}

func writeTestMain(out io.Writer) {
	fmt.Fprintf(out, `
func TestMain(m *testing.M) {
	testutil.Init()
	// TODO(cnicolaou): call modules.Dispatch and remove the need for TestHelperProcess
	os.Exit(m.Run())
}
`)
}

func writeModuleRegistration(out io.Writer, modules []moduleCommand) {
	for _, m := range modules {
		fmt.Fprintf(out, "\tmodules.RegisterChild(%q, `%s`, %s)\n", m.name, m.comment, m.name)
	}
}