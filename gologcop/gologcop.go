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
	"go/types"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"v.io/jiri/collect"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/goutil"
)

const (
	// nologComment is the magic comment text that disables log injection.
	nologComment = "nologcall"
	// logCallComment is the comment to be appended to all injected calls.
	logCallComment = "// gologcop: DO NOT EDIT, MUST BE FIRST STATEMENT"

	v23ContextPackage  = "v.io/v23/context"
	v23ContextTypeName = "T"
)

var (
	// the import tag for inject, if any, as in import tag "path"
	injectImportTag string
	// the import path for inject.
	injectImportPath string
	// the package name to use at call sites, it's either the
	// injectImportTag if one is specified or the base name of injectImportPath
	injectPackage string
	// the call to be injected, without the package name.
	injectCall string

	// the package and call to be removed
	removePackage, removeCall string
)

// parseState encapsulates all of the state acquired during parsing and
// type checking. It makes sure that any give file or package
// is parsed or type checked once and only once.
type parseState struct {
	ctx      *tool.Context
	config   *types.Config
	fset     *token.FileSet
	info     *types.Info
	packages map[string]*types.Package // keyed by the package path name.
	asts     map[string][]*ast.File    // keyed by the package path name
}

func newState(ctx *tool.Context) *parseState {
	ps := &parseState{
		ctx:      ctx,
		fset:     token.NewFileSet(),
		packages: make(map[string]*types.Package),
		asts:     make(map[string][]*ast.File),
		config: &types.Config{
			IgnoreFuncBodies: true,
		},
		info: &types.Info{
			Types: make(map[ast.Expr]types.TypeAndValue),
			Defs:  make(map[*ast.Ident]types.Object),
			Uses:  make(map[*ast.Ident]types.Object),
		},
	}
	ps.config.Importer = ps
	return ps
}

func (ps *parseState) Import(path string) (*types.Package, error) {
	return ps.sourceImporter(path)
}

func (ps *parseState) parsedPackage(path string) (*types.Package, []*ast.File) {
	return ps.packages[path], ps.asts[path]
}

func (ps *parseState) addParsedPackage(path string, pkg *types.Package, asts []*ast.File) {
	if p, _ := ps.parsedPackage(path); p != nil {
		fmt.Fprintf(ps.ctx.Stdout(), "Warning: %s is already cached\n", path)
		return
	}
	ps.packages[path] = pkg
	ps.asts[path] = asts
}

// sourceImporter will always import from source code.
func (ps *parseState) sourceImporter(path string) (*types.Package, error) {
	// It seems that we need to special case the unsafe package.
	if path == "unsafe" {
		return types.Unsafe, nil
	}
	if pkg, _ := ps.parsedPackage(path); pkg != nil {
		return pkg, nil
	}
	progressMsg(ps.ctx.Stdout(), "importing from source: %s\n", path)
	bpkg, err := build.Default.Import(path, ".", build.ImportMode(build.ImportComment))
	_, pkg, err := ps.parseAndTypeCheckPackage(bpkg)
	if err != nil {
		return nil, err
	}
	return pkg, err
}

// parseAndTypeCheckPackage will parse and type check a given package.
func (ps *parseState) parseAndTypeCheckPackage(bpkg *build.Package) ([]*ast.File, *types.Package, error) {
	if tpkg, asts := ps.parsedPackage(bpkg.ImportPath); tpkg != nil {
		return asts, tpkg, nil
	}

	tpkg := types.NewPackage(bpkg.ImportPath, bpkg.Name)
	checker := types.NewChecker(ps.config, ps.fset, tpkg, ps.info)

	// Parse the files in this package.
	asts := []*ast.File{}
	dir := bpkg.Dir
	for _, fileInPkg := range bpkg.GoFiles {
		file := filepath.Join(dir, fileInPkg)
		a, err := parser.ParseFile(ps.fset, file, nil, parser.ParseComments)
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
	progressMsg(ps.ctx.Stdout(), "parsed from source: %s\n", bpkg.ImportPath)
	ps.addParsedPackage(bpkg.ImportPath, tpkg, asts)
	return asts, tpkg, nil
}

// importPkgs will expand the supplied list of  packages using go list
// (so v.io/v23/... can be used as an interface package spec for example) and
// then import those packages.
func importPkgs(ctx *tool.Context, packageSpec []string) (ifcs []*build.Package, err error) {
	pkgs, err := goutil.List(ctx, packageSpec...)
	if err != nil {
		return nil, err
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
	bpkgs, err := importer(pkgs)
	if err != nil {
		return nil, fmt.Errorf("error importing packages: %v", err)
	}
	return bpkgs, nil
}

// exists is used as the value to indicate existence for maps that
// function as sets.
var exists = struct{}{}

func initInjectorFlags() error {
	parts := strings.FieldsFunc(injectCallImportFlag, unicode.IsSpace)
	var err error
	switch len(parts) {
	case 1:
		injectImportTag = ""
		injectImportPath, err = strconv.Unquote(injectCallImportFlag)
		if err != nil {
			injectImportPath = injectCallImportFlag
		}
		injectPackage = path.Base(injectImportPath)
	case 2:
		injectImportTag = parts[0]
		injectImportPath, err = strconv.Unquote(parts[1])
		if err != nil {
			injectImportPath = parts[1]
		}
		injectPackage = injectImportTag
	default:
		return fmt.Errorf("%q doesn't look like an import declaration", injectCallImportFlag)
	}
	injectCall = injectCallFlag
	return nil
}

// run runs the log injector.
func runInjector(ctx *tool.Context, interfaceList, implementationList []string, checkOnly bool) error {
	if err := initInjectorFlags(); err != nil {
		return err
	}
	// use 'go list' and the builder to import all of the packages
	// specified as interfaces and implementations.
	ifcs, err := importPkgs(ctx, interfaceList)
	if err != nil {
		return err
	}

	impls, err := importPkgs(ctx, implementationList)
	if err != nil {
		return err
	}

	printHeader(ctx.Stdout(), "Package Summary")
	progressMsg(ctx.Stdout(), "%v expands to %d interface packages\n", interfaceList, len(ifcs))
	progressMsg(ctx.Stdout(), "%v expands to %d implementation packages\n", implementationList, len(impls))

	ps := newState(ctx)
	checkFailed := []string{}

	printHeader(ctx.Stdout(), "Parsing and Type Checking Interface Packages")

	ifcPkgs := []*types.Package{}
	for _, ifc := range ifcs {
		_, tpkg, err := ps.parseAndTypeCheckPackage(ifc)
		if err != nil {
			return fmt.Errorf("failed to parse+type check: %s: %s", ifc.ImportPath, err)
		}
		ifcPkgs = append(ifcPkgs, tpkg)
	}
	publicInterfaces := findPublicInterfaces(ctx, ifcPkgs)

	for _, impl := range impls {
		printHeader(ctx.Stdout(), "Parsing and Type Checking Implementation Packages")
		asts, tpkg, err := ps.parseAndTypeCheckPackage(impl)
		if err != nil {
			return fmt.Errorf("failed to parse+type check: %s: %s", impl.ImportPath, err)
		}

		// Now find the methods that implement those public interfaces.
		methods := findMethodsImplementing(ctx, ps.fset, tpkg, publicInterfaces)

		// and their positions in the files.
		methodPositions, err := functionDeclarationsAtPositions(ps.fset, asts, ps.info, methods)
		if err != nil {
			return err
		}
		// then check to see if those methods already have logging statements.
		needsInjection := checkMethods(methodPositions)

		if checkOnly {
			if len(needsInjection) > 0 {
				printHeader(ctx.Stdout(), "Check Results")
				reportResults(ctx, ps.fset, needsInjection)
				checkFailed = append(checkFailed, impl.ImportPath)
			}
		} else {
			if err := inject(ctx, ps.fset, needsInjection); err != nil {
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

func initRemoverFlags() error {
	parts := strings.Split(removeCallFlag, ".")
	switch len(parts) {
	case 2:
		removePackage = parts[0]
		removeCall = parts[1]
	default:
		return fmt.Errorf("%q doesn't look like a function call on an imported package", removeCallFlag)
	}
	return nil
}

func runRemover(ctx *tool.Context, implementationList []string) error {
	if err := initRemoverFlags(); err != nil {
		return err
	}

	// use 'go list' and the builder to import all of the packages
	// specified as implementations.
	impls, err := importPkgs(ctx, implementationList)
	if err != nil {
		return err
	}

	ps := newState(ctx)

	printHeader(ctx.Stdout(), "Package Summary")
	progressMsg(ctx.Stdout(), "%v expands to %d implementation packages\n", implementationList, len(impls))

	for _, impl := range impls {
		asts, tpkg, err := ps.parseAndTypeCheckPackage(impl)
		if err != nil {
			return fmt.Errorf("failed to parse+type check: %s: %s", impl.ImportPath, err)
		}
		methods := findMethods(ctx, ps.fset, tpkg)
		methodPositions, err := functionDeclarationsAtPositions(ps.fset, asts, ps.info, methods)
		if err != nil {
			return err
		}
		needsRemoval := findRemovals(methodPositions)
		if err := remove(ctx, ps.fset, needsRemoval); err != nil {
			return fmt.Errorf("removal failed for: %s: %s", impl.ImportPath, err)
		}
	}
	return nil
}

// funcDeclRef stores a reference to a function declaration, paired
// with the file containing it.
type funcDeclRef struct {
	Decl    *ast.FuncDecl
	File    *ast.File
	LogCall string
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

func hasV23Context(info *types.Info, parameters *ast.FieldList) (*ast.FieldList, string) {
	if !useContextFlag {
		return parameters, ""
	}
	if parameters == nil {
		return nil, "nil"
	}
	filtered := *parameters
	for i, field := range filtered.List {
		typ := info.TypeOf(field.Type)
		ptr, ok := typ.(*types.Pointer)
		if !ok {
			continue
		}
		named, ok := ptr.Elem().(*types.Named)
		if !ok {
			continue
		}
		name := named.Obj()
		if name.Pkg().Path() == v23ContextPackage && name.Name() == v23ContextTypeName {
			filtered.List = append(filtered.List[:i], filtered.List[i+1:]...)
			ctxname := "nil"
			if len(field.Names) > 0 && field.Names[0].Name != "_" {
				ctxname = field.Names[0].Name
			}
			return &filtered, ctxname
		}
	}
	return &filtered, "nil"
}

func genFmt(info *types.Info, fields *ast.FieldList, indirect bool) ([]string, []string, error) {

	fmtForBasicType := func(typ *types.Basic) string {
		if typ.Kind() == types.String {
			return "%.10s..."
		} else {
			return "%v"
		}
	}

	if fields == nil {
		return nil, nil, nil
	}
	format := []string{}
	args := []string{}
	for _, param := range fields.List {
		typ := info.TypeOf(param.Type)
		var f string
		printable := false
		ellipsis := false
		switch v := typ.(type) {
		case *types.Basic:
			f = fmtForBasicType(v)
			printable = true
		case *types.Named:
			switch u := typ.Underlying().(type) {
			case *types.Basic:
				f = fmtForBasicType(u)
				printable = true
			case *types.Interface:
				if v.Obj().Name() == "error" {
					f = "%v"
					printable = true
				}
			}
		case nil:
			if _, ok := param.Type.(*ast.Ellipsis); !ok {
				return nil, nil, fmt.Errorf("failed to locate type for %v", param.Names)
			}
			// We'll print out the ellipsis args as a slice of whatever type it is.
			f = "%v"
			printable = true
			ellipsis = true
		}
		for _, n := range param.Names {
			if n.Name != "_" && len(n.Name) > 0 {
				if printable {
					if ellipsis {
						format = append(format, n.Name+"...="+f)
					} else {
						format = append(format, n.Name+"="+f)
					}
					name := n.Name
					if indirect {
						name = "&" + name
					}
					args = append(args, name)
				} else {
					format = append(format, n.Name+"=")
				}
			}
		}
	}
	return format, args, nil
}

func genCall(info *types.Info, params, results *ast.FieldList) (string, error) {
	params, contextPar := hasV23Context(info, params)
	noargs := fmt.Sprintf("\n\tdefer %s.%s(%s)(%s) %s", injectPackage, injectCall, contextPar, contextPar, logCallComment)
	if info == nil {
		return noargs, nil
	}

	argFormat, printableArgs, err := genFmt(info, params, false)
	if err != nil {
		return "", err
	}

	resFormat, printableResults, err := genFmt(info, results, true)
	if err != nil {
		return "", err
	}

	if len(argFormat) == 0 && len(resFormat) == 0 {
		return noargs, nil
	}

	formatArgs := func(format, parameters []string) string {
		if len(format) > 0 {
			formatStr := strings.TrimSpace(strings.Join(format, ","))
			parametersStr := strings.Join(parameters, ",")
			return fmt.Sprintf("\"%s\", %s", formatStr, parametersStr)
		}
		return "\"\""
	}

	pars := formatArgs(argFormat, printableArgs)
	res := formatArgs(resFormat, printableResults)

	contextParArg, contextParRes := contextPar, contextPar
	if len(contextPar) > 0 {
		if len(pars) > 0 {
			contextParArg += ", "
		}
		if len(res) > 0 {
			contextParRes += ", "
		}
	}

	return fmt.Sprintf("\n\tdefer %s.%sf(%s%s)(%s%s) %s", injectPackage, injectCall, contextParArg, pars, contextParRes, res, logCallComment), nil
}

// functionDeclarationsAtPositions returns references to function
// declarations in packages where the position of the identifier token
// representing the name of the function is in positions.
func functionDeclarationsAtPositions(fset *token.FileSet, files []*ast.File, info *types.Info, positions map[token.Pos]struct{}) ([]funcDeclRef, error) {
	result := []funcDeclRef{}
	for _, file := range files {
		for _, decl := range file.Decls {
			if decl, ok := decl.(*ast.FuncDecl); ok {
				call, err := genCall(info, decl.Type.Params, decl.Type.Results)
				if err != nil {
					pos := fset.Position(decl.Pos())
					return nil, fmt.Errorf("%s:%d: %v", pos.Filename, pos.Line, err)
				}
				// for each function declaration in packages:
				//
				// it's important not to use decl.Pos() here
				// as it gives us the position of the "func"
				// token, whereas positions has collected
				// the locations of method name tokens:
				if _, ok := positions[decl.Name.Pos()]; ok {
					result = append(result, funcDeclRef{decl, file, call})
				}
			}
		}
	}
	return result, nil
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
			methodSet := types.NewMethodSet(typ)
			if methodSet.Len() == 0 {
				methodSet = types.NewMethodSet(types.NewPointer(typ))
			}
			for i := 0; i < methodSet.Len(); i++ {
				method := methodSet.At(i)
				fn := method.Obj().(*types.Func)
				// t may have a method that is not declared in any of
				// the interfaces we care about. No need to log that.
				if _, ok := apiMethodSet[fn.Name()]; ok {
					if fn.Pos() == 0 {
						// Embedded functions show up with a zero pos.
						continue
					}
					progressMsg(ctx.Stdout(), "%s.%s: %s\n", tpkg.Path(), fn.Name(), fset.Position(fn.Pos()))
					positions[fn.Pos()] = exists
				}
			}
		}
	}
	return positions
}

func findMethodsInScope(ctx *tool.Context, fset *token.FileSet, positions map[token.Pos]struct{}, scope *types.Scope) {
	for _, child := range scope.Names() {
		object := scope.Lookup(child)
		typ := object.Type()
		switch v := typ.(type) {
		case *types.Named:
			for i := 0; i < v.NumMethods(); i++ {
				m := v.Method(i)
				positions[m.Pos()] = exists
			}
		case *types.Signature:
			positions[object.Pos()] = exists
		}
	}
}

func findMethods(ctx *tool.Context, fset *token.FileSet, tpkg *types.Package) map[token.Pos]struct{} {
	positions := map[token.Pos]struct{}{}
	printHeader(ctx.Stdout(), "Methods in %s", tpkg.Path())
	scope := tpkg.Scope()
	findMethodsInScope(ctx, fset, positions, scope)
	return positions
}

type patch struct {
	Offset     int
	Text       string
	NextOffset int
}

type patchSorter []patch

func insertAt(offset int, text string) patch {
	return patch{
		Offset:     offset,
		Text:       text,
		NextOffset: offset,
	}
}

func removeRange(from, to int) patch {
	return patch{
		Offset:     from,
		NextOffset: to,
	}
}

func (p patchSorter) Len() int {
	return len(p)
}

func (p patchSorter) Less(i, j int) bool {
	return p[i].Offset < p[j].Offset
}

func (p patchSorter) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

// countOverlap counts the length of the common prefix between two strings.
func countOverlap(a, b string) (i int) {
	for ; i < len(a) && i < len(b) && a[i] == b[i]; i++ {
	}
	return
}

// ©3ImportLogPackage will make sure that the file includes an
// import declaration to the package to be injected, and adds one if it does not
// already.
func ensureImportLogPackage(fset *token.FileSet, file *ast.File) (patch, bool) {
	maxOverlap := 0
	var candidate token.Pos

	quotedImportPath := strconv.Quote(injectImportPath)

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
			tag := ""
			if s.Name != nil {
				tag = s.Name.Name
			}
			path := s.Path.Value

			// Match import tag.
			if len(injectImportTag) > 0 && injectImportTag == tag {
				return patch{}, false
			}

			// Match path.
			if quotedImportPath == path {
				return patch{}, false
			}

			// Keep track of which import in a parenthesised list of imports
			// has the greatest overlap with the one we're going to add - i.e.
			// make sure we insert the new import in the lexicographically ordered
			// location.
			overlap := countOverlap(s.Path.Value, quotedImportPath)
			if d.Lparen.IsValid() && overlap > maxOverlap {
				maxOverlap = overlap
				candidate = s.Pos()
			}
		}
	}

	impStmt := func() string {
		if len(injectImportTag) > 0 {
			return injectImportTag + " " + quotedImportPath + "\n"
		}
		return quotedImportPath + "\n"
	}

	if maxOverlap > 0 {
		return insertAt(fset.Position(candidate).Offset, impStmt()), true
	}

	// No import declaration found with parenthesis; create a new
	// one and add it to the beginning of the file.
	return insertAt(fset.Position(file.Decls[0].Pos()).Offset, "import "+impStmt()), true
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

func findRemovals(methods []funcDeclRef) map[funcDeclRef]error {
	result := map[funcDeclRef]error{}
	for _, m := range methods {
		if err := validateLogStatement(m.Decl, removePackage, removeCall); err == nil {
			result[m] = nil
		}
	}
	return result
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
	if err := validateLogStatement(method.Decl, injectPackage, injectCall); err != nil && !methodBeginsWithNoLogComment(method) {
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

// writeFiles writes out files modified by the patch sets supplied to it.
func writeFiles(ctx *tool.Context, fset *token.FileSet, files map[*ast.File][]patch) (e error) {
	filesToFormat := []string{}

	// Write out files in a fixed order so that other tools/tests can count on the
	// diff output.
	filenames := []string{}
	asts := map[string]*ast.File{}
	for file, _ := range files {
		filename := fset.Position(file.Pos()).Filename
		filenames = append(filenames, filename)
		asts[filename] = file
	}
	sort.Strings(filenames)

	for _, filename := range filenames {
		file := asts[filename]
		patches := files[file]
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
			beginOffset = patch.NextOffset
		}
		patchedSrc = append(patchedSrc, src[beginOffset:]...)
		if diffOnlyFlag {
			tmpDir, err := ctx.Run().TempDir("", "")
			if err != nil {
				return err
			}
			tmpFilename := filepath.Join(tmpDir, "gologcop-"+filepath.Base(filename))
			defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpDir) }, &e)
			if err := ctx.Run().WriteFile(tmpFilename, patchedSrc, os.FileMode(0644)); err != nil {
				return err
			}
			progressMsg(ctx.Stdout(), "Diffing %s with %s\n", filename, tmpFilename)
			verbose := false
			nctx := ctx.Clone(tool.ContextOpts{Verbose: &verbose})
			gofmt(nctx, []string{tmpFilename})
			nctx.Run().Command("diff", filename, tmpFilename)
		} else {
			ctx.Run().WriteFile(filename, patchedSrc, 644)
		}
	}
	if diffOnlyFlag {
		return nil
	}
	return gofmt(ctx, filesToFormat)
}

// remove removes a log call at the beginning of each method in methods.
func remove(ctx *tool.Context, fset *token.FileSet, methods map[funcDeclRef]error) error {
	files := map[*ast.File][]patch{}
	comments := map[*ast.File]ast.CommentMap{}
	for fdRef, _ := range methods {
		file := fdRef.File
		if _, present := comments[file]; !present {
			comments[fdRef.File] = ast.NewCommentMap(fset, file, file.Comments)
		}
	}

	// endAt returns the position of the next statement, comment or function,
	// i.e. the end of the block of code to be removed.
	endAt := func(fn *ast.FuncDecl, cm ast.CommentMap) int {
		endpos := fn.Body.Rbrace
		stmt := fn.Body.List[0]
		if len(fn.Body.List) > 1 {
			nextStmt := fn.Body.List[1]
			endpos = nextStmt.Pos()
			if cg := cm.Filter(nextStmt).Comments(); len(cg) > 0 {
				if len(cg[0].List) > 0 {
					if cg[0].List[0].Pos() < endpos {
						// Only use a comment if it comes before the next statemnt.
						endpos = cg[0].List[0].Pos()
					}
				}
			}
		}
		stmtLine := fset.Position(stmt.Pos()).Line
		// Delete any comment on the same line as the logcall.
		for _, cg := range cm.Filter(stmt).Comments() {
			for _, c := range cg.List {
				if fset.Position(c.Pos()).Line > stmtLine {
					endpos = c.Pos()
					break
				}
			}
		}
		return fset.Position(endpos).Offset
	}

	for m, _ := range methods {
		file := m.File
		stmts := m.Decl.Body.List
		if len(stmts) == 0 {
			return fmt.Errorf("no statements found for %s", m.Decl.Name)
		}
		// The first statement should be the call we want to remove.
		start := fset.Position(stmts[0].Pos()).Offset
		end := endAt(m.Decl, comments[m.File])
		files[file] = append(files[file], removeRange(start, end))
	}
	return writeFiles(ctx, fset, files)
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
		text := m.LogCall
		// Catch the case where the function body is on the same line - e.g. func() {}
		// so that we make sure we add a newline to the comment to push the right brace
		// onto the next line.
		if fset.Position(m.Decl.Body.Lbrace).Line == fset.Position(m.Decl.Body.Rbrace).Line {
			text += "\n"
		}
		delta := insertAt(fset.Position(m.Decl.Body.Lbrace).Offset+1, text)
		file := m.File
		files[file] = append(files[file], delta)
	}

	for file, deltas := range files {
		if delta, hasChanges := ensureImportLogPackage(fset, file); hasChanges {
			files[file] = append(deltas, delta)
		}
	}
	return writeFiles(ctx, fset, files)
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
// with a valid defer call.
func validateLogStatement(method *ast.FuncDecl, pkg, name string) error {
	stmtList := method.Body.List

	if len(stmtList) == 0 {
		return &errNotExists{"empty method"}
	}

	deferStmt, ok := stmtList[0].(*ast.DeferStmt)
	if !ok {
		return &errNotExists{"no defer statement"}
	}

	logCall, ok := deferStmt.Call.Fun.(*ast.CallExpr)
	if !ok {
		return &errNotExists{"defer is a not a function call"}
	}

	selector, ok := logCall.Fun.(*ast.SelectorExpr)
	if !ok {
		return &errNotExists{"not a <pkg>.<method> call"}
	}

	packageIdent, ok := selector.X.(*ast.Ident)
	if !ok {
		return &errNotExists{"not a valid package selector"}
	}

	if packageIdent.Name != pkg {
		return &errNotExists{fmt.Sprintf("wrong package: got %q, want %q", packageIdent.Name, pkg)}
	}

	deferArgs := deferStmt.Call.Args
	if useContextFlag && len(deferArgs) > 0 {
		deferArgs = deferArgs[1:]
	}

	switch selector.Sel.Name {
	case name:
		return ensureExprsArePointers(deferArgs)
	case name + "f":
		nFnArgs := 0
		if fnCall, ok := deferStmt.Call.Fun.(*ast.CallExpr); ok {
			nFnArgs = len(fnCall.Args)
		}
		if nFnArgs < 1 {
			return &errInvalid{"no format specifier specified for called defer func: " + name}
		}
		nCallArgs := len(deferStmt.Call.Args)
		if nCallArgs < 1 {
			return &errInvalid{"no format specifier specified for returned defer func: " + name}
		}
		if len(deferArgs) > 0 {
			// Skip past format flag, but if we're called for a Remove
			// then we can't be sure there is a format.
			deferArgs = deferArgs[1:]
		}
		return ensureExprsArePointers(deferArgs)
	}

	return &errNotExists{fmt.Sprintf("got \"%s.%s\", want \"%s.%s\"", packageIdent.Name, selector.Sel.Name, pkg, name)}
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

func progressMsg(out io.Writer, format string, args ...interface{}) {
	if progressFlag {
		fmt.Fprintf(out, format, args...)
	}
}

// findPublicInterfaces returns all the public interfaces defined in the
// supplied packages.
func findPublicInterfaces(ctx *tool.Context, ifcs []*types.Package) (interfaces []*types.Interface) {
	for _, ifc := range ifcs {
		printHeader(ctx.Stdout(), "Public Interfaces for %s", ifc.Path())
		scope := ifc.Scope()
		for _, child := range scope.Names() {
			object := scope.Lookup(child)
			typ := object.Type()

			if object.Exported() && types.IsInterface(typ) {
				ifcType := typ.Underlying().(*types.Interface)

				if !ifcType.Empty() {
					progressMsg(ctx.Stdout(), "%s.%s\n", ifc.Path(), object.Name())
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

type errNotExists struct {
	message string
}

func (e errNotExists) Error() string {
	return fmt.Sprintf("injected statement does not exist: %s", e.message)
}
