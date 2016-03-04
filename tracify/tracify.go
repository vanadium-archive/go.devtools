// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go . -help

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"os/exec"
	"strings"

	"text/template"
	"v.io/x/lib/cmdline"
	"v.io/x/lib/envvar"
)

var skipPackages = map[string]bool{
	"v.io/v23/vtrace":                    true,
	"v.io/v23/verror":                    true,
	"v.io/x/ref/runtime/internal/vtrace": true,
}

func main() {
	cmdline.Main(cmdTracify)
}

var (
	transitive = flag.Bool("t", false, "include transitive dependencies of named packages.")
)

var cmdTracify = &cmdline.Command{
	Name:  "tracify",
	Short: "Add vtrace annotations to functions in the specified packages.",
	Long: `
tracify adds vtrace annotations to all functions in the given packages that
have a context as the first argument.

TODO(mattr): We will eventually support various options like excluding certain functions
or including specific information in the span name.
`,
	ArgsName: "[-t] [packages]",
	Runner:   cmdline.RunnerFunc(tracify),
}

// tracify adds vtrace spans to functions in the packages defined by args.
func tracify(env *cmdline.Env, args []string) error {
	pkgs, err := readPackages(env, args)
	if err != nil {
		return err
	}
	if *transitive {
		tPkgs := map[string]*build.Package{}
		for _, pkg := range pkgs {
			if err := addTransitive(tPkgs, pkg, true); err != nil {
				return err
			}
		}
		pkgs = tPkgs
	}
	for _, pkg := range pkgs {
		if pkg != nil {
			if err := processPackage(pkg); err != nil {
				return err
			}
		}
	}
	return nil
}

// processPackage processes a build package, rewriting any file in the package
// to include vtrace annotations.
func processPackage(pkg *build.Package) error {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, pkg.Dir, nil, parser.ParseComments)
	if err != nil {
		return err
	}
	for _, p := range pkgs {
		for fname, f := range p.Files {
			processFile(fset, fname, f)
		}
	}
	return nil
}

var vtraceTpl = template.Must(template.New("vtrace").Parse(`
	{{.CtxName}}, vspan := {{.VtraceName}}.WithNewSpan({{.CtxName}}, "{{.FuncName}}")
	defer vspan.Finish()
`))

type decl struct {
	pos        token.Position
	CtxName    string
	FuncName   string
	VtraceName string
}

// processFile Processes a single source file, rewriting it to include vtrace
// spans where necessary.
func processFile(fset *token.FileSet, fname string, f *ast.File) error {
	vtraceName := ""
	for _, i := range f.Imports {
		if i.Path.Value == "\"v.io/v23/vtrace\"" {
			if i.Name == nil {
				vtraceName = "vtrace"
			} else {
				vtraceName = i.Name.Name
			}
		}
	}

	decls := []decl{}
	args := translateTypes(fset, f.Imports, []string{"*context.T"})
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok {
			matches, names := checkParams(fset, fd.Type, args)
			if !matches || len(names) == 0 || names[0] == "_" {
				continue
			}
			decls = append(decls, decl{
				pos:      fset.Position(fd.Body.Lbrace),
				CtxName:  names[0],
				FuncName: fd.Name.Name,
			})
		}
	}

	if len(decls) > 0 {
		inj, err := newInjector(fname)
		if err != nil {
			return err
		}
		if vtraceName == "" {
			if err := inj.inject(fset.Position(f.Name.End()), "\nimport \"v.io/v23/vtrace\"\n"); err != nil {
				return err
			}
			vtraceName = "vtrace"
		}
		for _, d := range decls {
			d.VtraceName = vtraceName
			if err := inj.execute(d.pos, vtraceTpl, d); err != nil {
				return err
			}
		}
		if err := inj.format(); err != nil {
			return err
		}
	}
	return nil
}

// readPackages resolves the user-supplied package patterns to a list of actual packages.
// We just call out to 'go list' for this since there is actually a lot of subtlety
// in resolving the patterns.
func readPackages(env *cmdline.Env, args []string) (map[string]*build.Package, error) {
	buf := &bytes.Buffer{}
	opts := []string{"list", "-json"}
	cmd := exec.Command("go", append(opts, args...)...)
	cmd.Env = envvar.MapToSlice(env.Vars)
	cmd.Stderr = env.Stderr
	cmd.Stdout = buf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("Could not list packages: %v", err)
	}
	dec := json.NewDecoder(buf)
	packages := map[string]*build.Package{}
	for {
		var pkg build.Package
		if err := dec.Decode(&pkg); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		packages[pkg.ImportPath] = &pkg
	}
	return packages, nil
}

// addTransitive adds the transitive dependencies of pkg to packages.
func addTransitive(packages map[string]*build.Package, pkg *build.Package, alsoTest bool) error {
	if skipPackages[pkg.ImportPath] {
		return nil
	}
	if _, ok := packages[pkg.ImportPath]; ok {
		return nil
	}
	packages[pkg.ImportPath] = nil
	foundCtx := false
	for _, dep := range pkg.Imports {
		if dep == "v.io/v23/context" {
			foundCtx = true
			break
		}
	}

	allImports := [][]string{pkg.Imports}
	if alsoTest {
		allImports = append(allImports, pkg.TestImports, pkg.XTestImports)
	}

	for _, imports := range allImports {
		for _, dep := range imports {
			if dep == "C" {
				continue
			}
			depPkg, err := build.Import(dep, "", 0)
			if err != nil {
				return err
			}
			if err := addTransitive(packages, depPkg, false); err != nil {
				return err
			}
		}
	}
	// Skip if we don't depend on context.
	if foundCtx {
		packages[pkg.ImportPath] = pkg
	}
	return nil
}

// checkParams returns true if the given function type has argument types matching
// the strings in args.
func checkParams(fset *token.FileSet, ftype *ast.FuncType, args []string) (bool, []string) {
	i := 0
	names := []string{}
	buf := &bytes.Buffer{}
	for _, param := range ftype.Params.List {
		buf.Reset()
		format.Node(buf, fset, param.Type)
		typeStr := buf.String()
		nnames := len(param.Names)
		if nnames == 0 {
			nnames = 1 //Anonymous field.
		}
		for n := 0; n < nnames; n++ {
			if args[i] != typeStr {
				return false, nil
			}
			if n < len(param.Names) {
				names = append(names, param.Names[n].Name)
			}
			if i++; i >= len(args) {
				return true, names
			}
		}
	}
	return false, nil
}

// translateTypes uses the declared imports to change a list of queried types to their mapped versions.
// For example if you call:
//    translateTypes(fset, imports, "*testing.T")
// and imports contains the import:
//    import t "testing"
// Then we will return:
//    []string{"*t.T"}
func translateTypes(fset *token.FileSet, imports []*ast.ImportSpec, types []string) []string {
	namedImports := map[string]string{}
	for _, i := range imports {
		if i.Name != nil {
			path := strings.Trim(i.Path.Value, "\"")
			namedImports[path] = i.Name.Name
		}
	}
	out := make([]string, len(types))
	buf := &bytes.Buffer{}
	for i, typ := range types {
		out[i] = typ
		if expr, err := parser.ParseExpr(typ); err == nil && changePackage(expr, namedImports) {
			buf.Reset()
			if err := format.Node(buf, fset, expr); err == nil {
				out[i] = buf.String()
			}
		}
	}
	return out
}

// This changes the package name of a type to the mapped name.
// For example if you pass in the expr corresponding to "*testing.T" but
// the file has declared:
//   import t "testing"
// then it will return the expr for "*t.T".
// TODO(mattr): I'm not sure this catches all the cases.  If we find a breakage
// we can fix it then.
func changePackage(expr ast.Expr, namedImports map[string]string) bool {
	switch e := expr.(type) {
	case *ast.StarExpr:
		return changePackage(e.X, namedImports)
	case *ast.SelectorExpr:
		if id, ok := e.X.(*ast.Ident); ok {
			if name, has := namedImports[id.Name]; has {
				id.Name = name
			}
			return true
		}
		return false
	default:
		return false
	}
}
