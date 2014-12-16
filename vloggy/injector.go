package main

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"sort"
	"strings"

	"veyron.io/tools/lib/util"

	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/go/types"
	"golang.org/x/tools/go/types/typeutil"
)

const (
	// logPackageIdentifier is the identifier through which the
	// log package is imported.
	logPackageIdentifier = "vlog"
	// logPackageImportPath is the import path for the log package.
	logPackageImportPath = "veyron.io/veyron/veyron2/vlog"
	// logCallFuncName is the name of the default logging function.
	logCallFuncName = "LogCall"
	// logCallfFuncName is the name of the formattable logging function.
	logCallfFuncName = "LogCallf"
	// nologComment is the magic comment text that disables log injection.
	nologComment = "nologcall"
)

// TODO(jsimsa): expand "..." in package names in command line
func load(interfaces, implementations, tags []string) (prog *loader.Program, err error) {
	buildContext := build.Default
	buildContext.BuildTags = tags
	conf := loader.Config{SourceImports: true, Build: &buildContext}
	allPackages := append(append([]string{}, interfaces...), implementations...)
	conf.FromArgs(allPackages, false)
	conf.ParserMode |= parser.ParseComments
	return conf.Load()
}

func findPackages(prog *loader.Program, interfaces, implementations []string) (interfacePackages, implementationPackages []*loader.PackageInfo) {
	iSet := newStringSet(interfaces)
	mSet := newStringSet(implementations)

	iPackages := []*loader.PackageInfo{}
	mPackages := []*loader.PackageInfo{}

	for _, pkg := range prog.InitialPackages() {
		path := pkg.Pkg.Path()
		if _, ok := iSet[path]; ok {
			iPackages = append(iPackages, pkg)
		}
		if _, ok := mSet[path]; ok {
			mPackages = append(mPackages, pkg)
		}
	}

	return iPackages, mPackages
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
func run(ctx *util.Context, interfaceList, implementationList []string, checkOnly bool) error {
	prog, err := load(interfaceList, implementationList, nil)
	if err != nil {
		return err
	}

	interfacePackages, implementationPackages := findPackages(prog, interfaceList, implementationList)

	interfaces := findPublicInterfaces(interfacePackages)
	methods := findMethodsImplementing(implementationPackages, interfaces)
	needsInjection := checkMethods(methods)

	if checkOnly {
		reportResults(ctx, prog.Fset, needsInjection)
		if len(needsInjection) > 0 {
			os.Exit(1)
		}
		return nil
	}

	return inject(ctx, prog.Fset, needsInjection)
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
	for _, iface := range interfaces {
		if types.Implements(t, iface) || types.Implements(types.NewPointer(t), iface) {
			// t implements iface, so add all the public
			// method names of iface to set.
			for i := 0; i < iface.NumMethods(); i++ {
				if name := iface.Method(i).Name(); ast.IsExported(name) {
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
func functionDeclarationsAtPositions(packages []*loader.PackageInfo, positions map[token.Pos]struct{}) []funcDeclRef {
	result := []funcDeclRef{}
	for _, pkg := range packages {
		for _, file := range pkg.Files {
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
	}
	return result
}

// isInterface returns true if t is an interface declaration.
func isInterface(t types.Type) bool {
	_, ok := t.Underlying().(*types.Interface)
	return ok
}

// findMethodsImplementing searches the specified packages and returns
// a list of function declarations that are implementations for
// the specified interfaces.
func findMethodsImplementing(packages []*loader.PackageInfo, interfaces []*types.Interface) []funcDeclRef {
	// positions will hold the set of Pos values of methods
	// that should be logged.  Each element will be the position of
	// the identifier token representing the method name of such
	// methods.  The reason we collect the positions first is that
	// our static analysis library has no easy way to map types.Func
	// objects to ast.FuncDecl objects, so we then look into AST
	// declarations and find everything that has a matching position.
	positions := map[token.Pos]struct{}{}

	// msetCache caches information for typeutil.IntuitiveMethodSet()
	msetCache := types.MethodSetCache{}
	for _, pkg := range packages {
		for _, def := range pkg.Defs {
			if def, ok := def.(*types.TypeName); ok {
				t := def.Type()
				// ignore interfaces as they have no method implementations
				if isInterface(t) {
					continue
				}

				// for each non-interface type t declared in packages:
				apiMethodSet := methodSetVisibleThroughInterfaces(t, interfaces)

				// optimization: if t implements no non-empty interfaces that
				// we care about, we can just ignore it.
				if len(apiMethodSet) > 0 {
					// find all the methods explicitly declared or implicitly
					// inherited through embedding on type t or *t.
					for _, method := range typeutil.IntuitiveMethodSet(t, &msetCache) {
						fn := method.Obj().(*types.Func)
						// t may have a method that is not declared in any of
						// the interfaces we care about. No need to log that.
						if _, ok := apiMethodSet[fn.Name()]; ok {
							positions[fn.Pos()] = exists
						}
					}
				}
			}
		}
	}

	return functionDeclarationsAtPositions(packages, positions)
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
func gofmt(ctx *util.Context, files []string) error {
	if !gofmtFlag {
		return nil
	}
	return ctx.Run().Command("gofmt", append([]string{"-w"}, files...)...)
}

// inject injects a log call at the beginning of each method in methods.
func inject(ctx *util.Context, fset *token.FileSet, methods map[funcDeclRef]error) error {
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
func reportResults(ctx *util.Context, fset *token.FileSet, methods map[funcDeclRef]error) {
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

// findPublicInterfaces returns all the public interfaces defined in
// packages
func findPublicInterfaces(packages []*loader.PackageInfo) (interfaces []*types.Interface) {
	for _, pkg := range packages {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				if decl, ok := decl.(*ast.GenDecl); ok && decl.Tok == token.TYPE && len(decl.Specs) > 0 {
					if typeSpec, ok := decl.Specs[0].(*ast.TypeSpec); ok && ast.IsExported(typeSpec.Name.Name) {
						if ifaceType, ok := typeSpec.Type.(*ast.InterfaceType); ok {
							iface := pkg.TypeOf(ifaceType).(*types.Interface)
							if !iface.Empty() {
								interfaces = append(interfaces, pkg.TypeOf(ifaceType).(*types.Interface))
							}
						}
					}
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
