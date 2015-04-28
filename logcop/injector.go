// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/gcimporter"
	"golang.org/x/tools/go/types"
	"golang.org/x/tools/go/types/typeutil"

	"v.io/x/devtools/internal/goutil"
	"v.io/x/devtools/internal/tool"
)

const (
	// logPackageIdentifier is the identifier through which the
	// log package is imported.
	logPackageIdentifier = "vlog"
	// logPackageImportPath is the import path for the log package.
	logPackageImportPath = "v.io/x/lib/vlog"
	// logCallFuncName is the name of the default logging function.
	logCallFuncName = "LogCall"
	// logCallfFuncName is the name of the formattable logging function.
	logCallfFuncName = "LogCallf"
	// nologComment is the magic comment text that disables log injection.
	nologComment = "nologcall"
)

// gcOrSourceImportert will use gcimporter to attempt to import from
// a .a file, but if one doesn't exist it will import from source code.
func gcOrSourceImporter(ctx *tool.Context, fset *token.FileSet, imports map[string]*types.Package, path string) (*types.Package, error) {
	if p, err := gcimporter.Import(imports, path); err == nil {
		return p, err
	}
	if progressFlag {
		fmt.Fprintf(ctx.Stdout(), "importing from source: %s\n", path)
	}
	bpkg, err := build.Default.Import(path, ".", build.ImportMode(build.ImportComment))
	_, pkg, err := parseAndTypeCheckPackage(ctx, fset, bpkg)
	if err != nil {
		return nil, err
	}
	return pkg, err
}

// importPkgs will expand the supplied list of interface and implementation
// packages using go list (so v.io/v23/... can be used as an interface package
// spec for example) and then import those packages.
func importPkgs(ctx *tool.Context, interfaces, implementations []string) (ifcs, impls []*build.Package, err error) {

	ifcPkgs, err := goutil.List(ctx, interfaces)
	if err != nil {
		return nil, nil, err
	}
	implPkgs, err := goutil.List(ctx, implementations)
	if err != nil {
		return nil, nil, err
	}

	importer := func(pkgs []string) ([]*build.Package, error) {
		pkgInfos := []*build.Package{}
		for _, pkg := range pkgs {
			pkgInfo, err := build.Default.Import(pkg, ".", build.ImportMode(build.ImportComment))
			if err != nil {
				return nil, err
			}
			pkgInfos = append(pkgInfos, pkgInfo)
		}
		return pkgInfos, nil
	}

	ifcs, err = importer(ifcPkgs)
	if err != nil {
		return nil, nil, fmt.Errorf("error importing interface packages: %v", err)
	}

	impls, err = importer(implPkgs)
	if err != nil {
		return nil, nil, fmt.Errorf("error importing implementation packages: %v", err)
	}
	return ifcs, impls, nil
}

// parseAndTypeCheckPackage will parse and type check a given package.
func parseAndTypeCheckPackage(ctx *tool.Context, fset *token.FileSet, bpkg *build.Package) ([]*ast.File, *types.Package, error) {

	config := &types.Config{}
	config.Import = func(imports map[string]*types.Package, path string) (*types.Package, error) {
		return gcOrSourceImporter(ctx, fset, imports, path)
	}
	config.IgnoreFuncBodies = true
	tpkg := types.NewPackage(bpkg.ImportPath, bpkg.Name)
	checker := types.NewChecker(config, fset, tpkg, nil)

	// Parse the files in this package.
	asts := []*ast.File{}
	dir := bpkg.Dir
	for _, fileInPkg := range bpkg.GoFiles {
		file := filepath.Join(dir, fileInPkg)
		a, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
		if err != nil {
			return nil, nil, err
		}
		asts = append(asts, a)
	}
	if err := checker.Files(asts); err != nil {
		return nil, nil, err
	}

	// make sure that type checking is complete at this stage. It should
	// always be so, so this is really an 'assertion' that it is.
	if !tpkg.Complete() {
		return nil, nil, fmt.Errorf("checked %q is not completely parsed+checked", bpkg.Name)
	}
	return asts, tpkg, nil
}

// exists is used as the value to indicate existence for maps that
// function as sets.
var exists = struct{}{}

// newStringSet creates a new set out of a slice of strings.
func newStringSet(values []string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, s := range values {
		set[s] = exists
	}
	return set
}

// run runs the log injector.
func run(ctx *tool.Context, interfaceList, implementationList []string, checkOnly bool) error {
	// use 'go list' and the builder to import all of the packages
	// specified as interfaces and implementations.
	ifcs, impls, err := importPkgs(ctx, interfaceList, implementationList)
	if err != nil {
		return err
	}

	if progressFlag {
		printHeader(ctx.Stdout(), "Package Summary")
		fmt.Fprintf(ctx.Stdout(), "%v expands to %d interface packages\n", interfaceList, len(ifcs))
		fmt.Fprintf(ctx.Stdout(), "%v expands to %d implementation packages\n", implementationList, len(impls))
	}

	// positions are relative to fset
	fset := token.NewFileSet()
	checkFailed := []string{}
	for _, impl := range impls {
		asts, tpkg, err := parseAndTypeCheckPackage(ctx, fset, impl)
		if err != nil {
			return fmt.Errorf("failed to parse+type check: %s: %s", impl.ImportPath, err)
		}
		// We've parsed and type checked this implementation package.
		// The next step is to find the public interfaces imported by it
		// that occur in the interface packages above.
		publicInterfaces := findPublicInterfaces(ctx, ifcs, tpkg)

		// Now find the methods that implement those public interfaces.
		methods := findMethodsImplementing(ctx, fset, tpkg, publicInterfaces)
		// and their positions in the files.
		methodPositions := functionDeclarationsAtPositions(asts, methods)
		// then check to see if those methods already have logging statements.
		needsInjection := checkMethods(methodPositions)

		if checkOnly {
			if len(needsInjection) > 0 {
				printHeader(ctx.Stdout(), "Check Results")
				reportResults(ctx, fset, needsInjection)
				checkFailed = append(checkFailed, impl.ImportPath)
			}
		} else {
			if err := inject(ctx, fset, needsInjection); err != nil {
				return fmt.Errorf("injection failed for: %s: %s", impl.ImportPath, err)
			}
		}
	}

	if checkOnly && len(checkFailed) > 0 {
		for _, p := range checkFailed {
			fmt.Fprintf(ctx.Stdout(), "check failed for: %s\n", p)
		}
		os.Exit(1)
	}

	return nil
}

// funcDeclRef stores a reference to a function declaration, paired
// with the file containing it.
type funcDeclRef struct {
	Decl *ast.FuncDecl
	File *ast.File
}

// methodSetVisibleThroughInterfaces returns intersection of all
// exported method names implemented by t and the union of all method
// names declared by interfaces.
func methodSetVisibleThroughInterfaces(t types.Type, interfaces []*types.Interface) map[string]struct{} {
	set := map[string]struct{}{}
	for _, ifc := range interfaces {
		if types.Implements(t, ifc) || types.Implements(types.NewPointer(t), ifc) {
			// t implements ifc, so add all the public
			// method names of ifc to set.
			for i := 0; i < ifc.NumMethods(); i++ {
				if name := ifc.Method(i).Name(); ast.IsExported(name) {
					set[name] = exists
				}
			}
		}
	}
	return set
}

// functionDeclarationsAtPositions returns references to function
// declarations in packages where the position of the identifier token
// representing the name of the function is in positions.
func functionDeclarationsAtPositions(files []*ast.File, positions map[token.Pos]struct{}) []funcDeclRef {
	result := []funcDeclRef{}
	for _, file := range files {
		for _, decl := range file.Decls {
			if decl, ok := decl.(*ast.FuncDecl); ok {
				// for each function declaration in packages:
				//
				// it's important not to use decl.Pos() here
				// as it gives us the position of the "func"
				// token, whereas positions has collected
				// the locations of method name tokens:
				if _, ok := positions[decl.Name.Pos()]; ok {
					result = append(result, funcDeclRef{decl, file})
				}
			}
		}
	}
	return result
}

// findMethodsImplementing searches the specified packages and returns
// a list of function declarations that are implementations for
// the specified interfaces.
func findMethodsImplementing(ctx *tool.Context, fset *token.FileSet, tpkg *types.Package, interfaces []*types.Interface) map[token.Pos]struct{} {
	// positions will hold the set of Pos values of methods
	// that should be logged.  Each element will be the position of
	// the identifier token representing the method name of such
	// methods.  The reason we collect the positions first is that
	// our static analysis library has no easy way to map types.Func
	// objects to ast.FuncDecl objects, so we then look into AST
	// declarations and find everything that has a matching position.
	positions := map[token.Pos]struct{}{}

	printHeader(ctx.Stdout(), "Methods Implementing Public Interfaces in %s", tpkg.Path())

	// msetCache caches information for typeutil.IntuitiveMethodSet()
	msetCache := types.MethodSetCache{}
	scope := tpkg.Scope()
	for _, child := range scope.Names() {
		object := scope.Lookup(child)
		typ := object.Type()
		// ignore interfaces as they have no method implementations
		if types.IsInterface(typ) {
			continue
		}

		// for each non-interface type t declared in packages:
		apiMethodSet := methodSetVisibleThroughInterfaces(typ, interfaces)

		// optimization: if t implements no non-empty interfaces that
		// we care about, we can just ignore it.
		if len(apiMethodSet) > 0 {
			// find all the methods explicitly declared or implicitly
			// inherited through embedding on type t or *t.
			for _, method := range typeutil.IntuitiveMethodSet(typ, &msetCache) {
				fn := method.Obj().(*types.Func)
				// t may have a method that is not declared in any of
				// the interfaces we care about. No need to log that.
				if _, ok := apiMethodSet[fn.Name()]; ok {
					if fn.Pos() == 0 {
						// TODO(cnicolaou): figure out where these functions
						// with no pos information come from.
						continue
					}
					if progressFlag {
						fmt.Printf("%s.%s: %s\n", tpkg.Path(), fn.Name(), fset.Position(fn.Pos()))
					}
					positions[fn.Pos()] = exists
				}
			}
		}
	}
	return positions
}

type patch struct {
	Offset int
	Text   string
}

type patchSorter []patch

func (p patchSorter) Len() int {
	return len(p)
}

func (p patchSorter) Less(i, j int) bool {
	return p[i].Offset < p[j].Offset
}

func (p patchSorter) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

// logPackageQuotedImportPath is the quoted identifier for the import
// path of the logging library. It is used to check for existence of
// an import statement for the vlog runtime library or to inject a new
// import statement.
const logPackageQuotedImportPath = `"` + logPackageImportPath + `"`

// countOverlap counts the length of the common prefix between two strings.
func countOverlap(a, b string) (i int) {
	for ; i < len(a) && i < len(b) && a[i] == b[i]; i++ {
	}
	return
}

// ensureImportLogPackage will make sure that the file includes an
// import declaration to the log package, and adds one if it does not
// already.
func ensureImportLogPackage(fset *token.FileSet, file *ast.File) (patch, bool) {
	maxOverlap := 0
	var candidate token.Pos

	for _, d := range file.Decls {
		d, ok := d.(*ast.GenDecl)
		if !ok || d.Tok != token.IMPORT {
			// We encountered a non-import declaration. As
			// imports always precede other declarations,
			// we are done with our search.
			break
		}

		for _, s := range d.Specs {
			s := s.(*ast.ImportSpec)
			overlap := countOverlap(s.Path.Value, logPackageQuotedImportPath)
			if overlap == len(logPackageQuotedImportPath) && (s.Name == nil || s.Name.Name == logPackageIdentifier) {
				// We found a valid import for the
				// logging package. No need to inject
				// a duplicate one.
				return patch{}, false
			}
			if d.Lparen.IsValid() && overlap > maxOverlap {
				maxOverlap = overlap
				candidate = s.Pos()
			}
		}
	}

	if maxOverlap > 0 {
		return patch{Offset: fset.Position(candidate).Offset, Text: logPackageQuotedImportPath + "\n"}, true
	}

	// No import declaration found with parenthesis; create a new
	// one and add it to the beginning of the file.
	return patch{Offset: fset.Position(file.Decls[0].Pos()).Offset, Text: "import " + logPackageQuotedImportPath + "\n"}, true
}

// methodBeginsWithNoLogComment returns true if method has a
// "nologcall" comment before any non-whitespace or non-comment token.
func methodBeginsWithNoLogComment(m funcDeclRef) bool {
	method := m.Decl
	lbound := method.Body.Lbrace
	ubound := method.Body.Rbrace
	stmts := method.Body.List
	if len(stmts) > 0 {
		ubound = stmts[0].Pos()
	}

	for _, cmt := range m.File.Comments {
		if lbound <= cmt.Pos() && cmt.End() <= ubound {
			for _, line := range strings.Split(cmt.Text(), "\n") {
				line := strings.TrimSpace(line)
				if line == nologComment {
					return true
				}
			}
		}
	}

	return false
}

// checkMethods checks all items in methods and returns the subset
// of them that do not have valid log statements.
func checkMethods(methods []funcDeclRef) map[funcDeclRef]error {
	result := map[funcDeclRef]error{}
	for _, m := range methods {
		if err := checkMethod(m); err != nil {
			result[m] = err
		}
	}
	return result
}

// checkMethod checks that method includes an acceptable logging
// construct before any other non-whitespace or non-comment token.
func checkMethod(method funcDeclRef) error {
	if err := validateLogStatement(method.Decl); err != nil && !methodBeginsWithNoLogComment(method) {
		return err
	}
	return nil
}

// gofmt runs "gofmt -w files...".
func gofmt(ctx *tool.Context, files []string) error {
	if len(files) == 0 || !gofmtFlag {
		return nil
	}
	return ctx.Run().Command("gofmt", append([]string{"-w"}, files...)...)
}

// inject injects a log call at the beginning of each method in methods.
func inject(ctx *tool.Context, fset *token.FileSet, methods map[funcDeclRef]error) error {
	// Warn the user for methods that already have something at
	// their beginning that looks like a logging construct, but it
	// is invalid for some reason.
	for m, err := range methods {
		if _, ok := err.(*errInvalid); ok {
			method := m.Decl
			position := fset.Position(method.Pos())
			methodName := method.Name.Name
			fmt.Fprintf(ctx.Stdout(), "Warning: %v: %s: %v\n", position, methodName, err)
		}
	}

	files := map[*ast.File][]patch{}
	for m, _ := range methods {
		delta := patch{Offset: fset.Position(m.Decl.Body.Lbrace).Offset + 1, Text: "\ndefer vlog.LogCall()(); "}
		file := m.File
		files[file] = append(files[file], delta)
	}

	for file, deltas := range files {
		if delta, hasChanges := ensureImportLogPackage(fset, file); hasChanges {
			files[file] = append(deltas, delta)
		}
	}

	filesToFormat := []string{}
	for file, patches := range files {
		filename := fset.Position(file.Pos()).Filename
		filesToFormat = append(filesToFormat, filename)
		sort.Sort(patchSorter(patches))
		src, err := ioutil.ReadFile(filename)
		if err != nil {
			return err
		}
		beginOffset := 0
		patchedSrc := []byte{}
		for _, patch := range patches {
			patchedSrc = append(patchedSrc, src[beginOffset:patch.Offset]...)
			patchedSrc = append(patchedSrc, patch.Text...)
			beginOffset = patch.Offset
		}
		patchedSrc = append(patchedSrc, src[beginOffset:]...)
		ctx.Run().WriteFile(filename, patchedSrc, 644)
	}

	return gofmt(ctx, filesToFormat)
}

// reportResults prints out the validation results from checkMethods
// in a human-readable form.
func reportResults(ctx *tool.Context, fset *token.FileSet, methods map[funcDeclRef]error) {
	for m, err := range methods {
		fmt.Fprintf(ctx.Stdout(), "%v: %s: %v\n", fset.Position(m.Decl.Pos()), m.Decl.Name.Name, err)
	}
}

// ensureExprsArePointers returns an error if at least one of the
// expressions in exprs is not in the form of &x.
func ensureExprsArePointers(exprs []ast.Expr) error {
	for _, expr := range exprs {
		if !isAddressOfExpression(expr) {
			return &errInvalid{"output arguments should be passed to the log function via their addresses"}
		}
	}
	return nil
}

// validateLogStatement returns an error if method does not begin
// with a valid defer vlog.LogCall or defer vlog.LogCallf call.
func validateLogStatement(method *ast.FuncDecl) error {
	stmtList := method.Body.List

	if len(stmtList) == 0 {
		return &errNotExists{}
	}

	deferStmt, ok := stmtList[0].(*ast.DeferStmt)
	if !ok {
		return &errNotExists{}
	}

	logCall, ok := deferStmt.Call.Fun.(*ast.CallExpr)
	if !ok {
		return &errNotExists{}
	}

	selector, ok := logCall.Fun.(*ast.SelectorExpr)
	if !ok {
		return &errNotExists{}
	}

	packageIdent, ok := selector.X.(*ast.Ident)
	if !ok {
		return &errNotExists{}
	}

	if packageIdent.Name != logPackageIdentifier {
		return &errNotExists{}
	}

	switch selector.Sel.Name {
	case logCallFuncName:
		return ensureExprsArePointers(deferStmt.Call.Args)
	case logCallfFuncName:
		if len(deferStmt.Call.Args) < 1 {
			return &errInvalid{"no format specifier specified for " + logCallFuncName}
		}
		return ensureExprsArePointers(deferStmt.Call.Args[1:])
	}

	return &errNotExists{}
}

// isAddressOfExpression checks if expr is an expression in the form
// of `&expression`
func isAddressOfExpression(expr ast.Expr) (isAddrExpr bool) {
	// TODO: support (&x) as well as &x
	unaryExpr, ok := expr.(*ast.UnaryExpr)
	return ok && unaryExpr.Op == token.AND
}

func printHeader(out io.Writer, format string, args ...interface{}) {
	if progressFlag {
		s := fmt.Sprintf(format, args...)
		fmt.Fprintln(out)
		fmt.Fprintln(out, s)
		fmt.Fprintln(out, strings.Repeat("=", len(s)))
	}
}

// findPublicInterfaces returns all the public interfaces defined in this
// packages imports.
func findPublicInterfaces(ctx *tool.Context, ifcs []*build.Package, tpkg *types.Package) (interfaces []*types.Interface) {
	isInterfacePackage := func(t *types.Package) bool {
		for _, b := range ifcs {
			if t.Name() == b.Name && t.Path() == b.ImportPath {
				return true
			}
		}
		return false
	}
	printHeader(ctx.Stdout(), "Public Interfaces for %s", tpkg.Path())
	tpkgs := append(tpkg.Imports(), tpkg)
	for _, imported := range tpkgs {
		if !isInterfacePackage(imported) {
			continue
		}
		scope := imported.Scope()
		for _, child := range scope.Names() {
			object := scope.Lookup(child)
			typ := object.Type()
			if object.Exported() && types.IsInterface(typ) {
				ifcType := typ.Underlying().(*types.Interface)
				if !ifcType.Empty() {
					if progressFlag {
						fmt.Printf("%s.%s\n", imported.Path(), object.Name())
					}
					interfaces = append(interfaces, ifcType)
				}
			}
		}
	}
	return interfaces
}

type errInvalid struct {
	message string
}

func (l errInvalid) Error() string {
	if len(l.message) > 0 {
		return l.message
	}
	return "invalid log statement"
}

type errNotExists struct{}

func (errNotExists) Error() string {
	return "log statement does not exist"
}
