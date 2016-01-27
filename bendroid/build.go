// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"v.io/x/lib/envvar"
)

var errNoTests = fmt.Errorf("There were no tests.")

func (t *testrun) build() error {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, t.BuildPkg.Dir, nil, parser.ParseComments)
	if err != nil {
		return err
	}
	base, ext, err := getBaseExtPackages(pkgs)
	if err != nil {
		return err
	}
	var foundfunc bool
	if foundfunc, err = t.rewritePackage(fset, base, t.BaseDir); err != nil {
		return err
	}
	if foundfunc {
		t.FuncImports = append(t.FuncImports, path.Join(t.BuildPkg.ImportPath, t.BasePkg))
	}
	if ext != nil {
		if foundfunc, err = t.rewritePackage(fset, ext, t.ExtDir); err != nil {
			return err
		}
		if foundfunc {
			t.FuncImports = append(t.FuncImports, path.Join(t.BuildPkg.ImportPath, t.BasePkg, t.ExtPkg))
		}
	}
	if len(t.FuncImports) == 0 {
		return errNoTests
	}
	if err := t.writeMainPackage(); err != nil {
		return err
	}
	// TODO(mattr): Vendor gomobile and then use the vendored version.
	args := []string{"build", "-o", t.apk}
	if *work {
		args = append(args, "-work")
	}
	if len(*tags) > 0 {
		args = append(args, "-tags", *tags)
	}
	args = append(args, path.Join(t.BuildPkg.ImportPath, t.BasePkg, t.MainPkg))
	cmd := exec.Command("gomobile", args...)
	cmd.Env = envvar.MapToSlice(t.Env.Vars)
	cmd.Stdout, cmd.Stderr = t.Env.Stdout, t.Env.Stderr
	return cmd.Run()
}

func getBaseExtPackages(pkgs map[string]*ast.Package) (base, ext *ast.Package, err error) {
	var basename string
	for pname := range pkgs {
		basename = strings.TrimSuffix(pname, "_test")
		break
	}
	base, ext = pkgs[basename], pkgs[basename+"_test"]
	if base == nil {
		return nil, nil, fmt.Errorf("Expected one base package and maybe an external test package in each directory, got %v", pkgs)
	}
	return
}

func (t *testrun) rewritePackage(fset *token.FileSet, pkg *ast.Package, outdir string) (bool, error) {
	var mains []funcref
	foundfunc := false
	for fname, f := range pkg.Files {
		f.Name.Name = filepath.Base(outdir)
		for _, imprt := range f.Imports {
			if strings.Trim(imprt.Path.Value, "\"") == t.BuildPkg.ImportPath {
				imprt.Path.Value = fmt.Sprintf(`"%s/%s"`, t.BuildPkg.ImportPath, t.BasePkg)
				if imprt.Name == nil {
					imprt.Name = &ast.Ident{Name: t.BuildPkg.Name}
				}
			}
		}
		ffs := []*funcfinder{
			newfuncfinder(&t.Tests, fset, f, "Test", "*testing.T"),
			newfuncfinder(&t.Benchmarks, fset, f, "Benchmark", "*testing.B"),
			newfuncfinder(&mains, fset, f, "TestMain", "*testing.M"),
		}
		for _, decl := range f.Decls {
			if fd, ok := decl.(*ast.FuncDecl); ok {
				for _, ff := range ffs {
					if ff.ProcessDecl(fd) {
						foundfunc = true
					}
				}
			}
		}
		if strings.HasSuffix(fname, "_test.go") {
			fname = fname[:len(fname)-len(".go")] + "_bendroid.go"
		}
		w, err := os.Create(filepath.Join(outdir, filepath.Base(fname)))
		if err != nil {
			return false, err
		}
		if err := format.Node(w, fset, f); err != nil {
			return false, err
		}
		if err := w.Close(); err != nil {
			return false, err
		}
	}
	for _, ref := range mains {
		if ref.Name == "TestMain" {
			t.TestMainPackage = ref.Package
			foundfunc = true
			break
		}
	}
	return foundfunc, nil
}

func (t *testrun) writeMainPackage() error {
	w, err := os.Create(filepath.Join(t.MainDir, "main.go"))
	if err != nil {
		return err
	}
	if err := mainTempl.Execute(w, t); err != nil {
		return err
	}
	w.Close()
	w, err = os.Create(filepath.Join(t.MainDir, "AndroidManifest.xml"))
	if err != nil {
		return err
	}
	if err := manifestTempl.Execute(w, t); err != nil {
		return err
	}
	w.Close()
	return nil
}

type funcfinder struct {
	fset   *token.FileSet
	out    *[]funcref
	prefix string
	args   []string
	pkg    string
}

func newfuncfinder(out *[]funcref, fset *token.FileSet, f *ast.File, prefix string, args ...string) *funcfinder {
	args = translateTypes(fset, f.Imports, args)
	return &funcfinder{fset: fset, prefix: prefix, out: out, args: args, pkg: f.Name.Name}
}

func (f *funcfinder) ProcessDecl(decl *ast.FuncDecl) bool {
	if strings.HasPrefix(decl.Name.Name, f.prefix) && checkParams(f.fset, decl.Type, f.args) {
		*f.out = append(*f.out, funcref{Name: decl.Name.Name, Package: f.pkg})
		return true
	}
	return false
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

// checkParams returns true if the given function type has argument types matching
// the strings in args.
func checkParams(fset *token.FileSet, ftype *ast.FuncType, args []string) bool {
	i := 0
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
				return false
			}
		}
	}
	return true
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
