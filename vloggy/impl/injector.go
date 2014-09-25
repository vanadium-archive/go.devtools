package impl

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"os/exec"
	"strings"

	"code.google.com/p/go.tools/go/loader"
	"code.google.com/p/go.tools/go/types"
	"code.google.com/p/go.tools/go/types/typeutil"
)

const (
	// logPackageIdentifier is the identifier through which the log package
	// is imported.
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

func load(interfaces, implementations, tags []string) (prog *loader.Program, err error) {
	// TODO: expand "..." in package names in command line
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

// exists is used as the value to indicate existence for maps
// that function as sets.
var exists = struct{}{}

// newStringSet creates a new set out of a slice of strings.
func newStringSet(values []string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, s := range values {
		set[s] = exists
	}
	return set
}

// Run runs the log injector.
func Run(interfaceList, implementationList []string, checkOnly bool) error {
	prog, err := load(interfaceList, implementationList, nil)
	if err != nil {
		return err
	}

	interfacePackages, implementationPackages := findPackages(prog, interfaceList, implementationList)

	interfaces := findPublicInterfaces(interfacePackages)
	methods := findMethodsImplementing(implementationPackages, interfaces)
	needsInjection := checkMethods(methods)

	if checkOnly {
		reportResults(prog.Fset, needsInjection)
		if len(needsInjection) > 0 {
			os.Exit(1)
		}
		return nil
	}

	return injectInSource(prog.Fset, needsInjection)
}

// funcDeclRef stores a reference to a function declaration, paired
// with the file containing it.
type funcDeclRef struct {
	Decl *ast.FuncDecl
	File *ast.File
}

// methodSetVisibleThroughInterfaces returns intersection of all
// exported method names implemented by t and the union of all
// method names declared by interfaces.
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
// declarations in packages where the position of the identifier
// token representing the name of the function is in positions.
func functionDeclarationsAtPositions(packages []*loader.PackageInfo, positions map[token.Pos]struct{}) []funcDeclRef {
	result := []funcDeclRef{}
	for _, pkg := range packages {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				if decl, ok := decl.(*ast.FuncDecl); ok {
					// for each function declaration in packages:

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

// logPackageQuotedImportPath is the quoted identifier for the import path
// of the logging library.  It is used to check for existence of an
// import statement for the vlog runtime library or to inject a new
// import statement.
const logPackageQuotedImportPath = `"` + logPackageImportPath + `"`

// injectImportLogPackage adds a new import for the logging package to file.
func injectImportLogPackage(file *ast.File) {
	newImportSpec := []ast.Spec{&ast.ImportSpec{Path: &ast.BasicLit{Value: logPackageQuotedImportPath}}}

	// Try appending the new import spec to the first import declaration
	// if one exists and contains a block
	if len(file.Decls) > 0 {
		if importDecl, ok := file.Decls[0].(*ast.GenDecl); ok && importDecl.Tok == token.IMPORT && importDecl.Lparen.IsValid() {
			importDecl.Specs = append(newImportSpec, importDecl.Specs...)
			return
		}
	}

	// No import declaration found; create a new one
	// and add it to the beginning of the file
	file.Decls = append([]ast.Decl{&ast.GenDecl{Tok: token.IMPORT, Specs: newImportSpec}}, file.Decls...)
}

// ensureImportLogPackage will make sure that the file includes an import declaration
// to the vlog package, and adds one if it does not already.
func ensureImportLogPackage(file *ast.File) {
	for _, d := range file.Decls {
		d, ok := d.(*ast.GenDecl)
		if !ok || d.Tok != token.IMPORT {
			// We encountered a non-import declaration. As imports always
			// precede other declarations, we are done with our search.
			break
		}

		for _, s := range d.Specs {
			s := s.(*ast.ImportSpec)
			if s.Path.Value == logPackageQuotedImportPath && (s.Name == nil || s.Name.Name == logPackageIdentifier) {
				// We found a valid import for the logging package.
				// No need to inject a duplicate one.
				return
			}
		}
	}

	injectImportLogPackage(file)
}

// injectLogStatement adds a "defer vlog.LogCall()()" statement at the
// beginning of the specified method.
func injectLogStatement(method *ast.FuncDecl) {
	method.Body.List = append([]ast.Stmt{newDeferStmtWithSelector(ast.NewIdent(logPackageIdentifier), ast.NewIdent(logCallFuncName))}, method.Body.List...)
}

// methodBeginsWithNoLogComment returns true if method has a "nologcall"
// comment before any non-whitespace or non-comment token.
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

// doInjection injects log statements in methods, returning the set
// of modified files.
func doInjection(fset *token.FileSet, methods map[funcDeclRef]error) (modified map[*ast.File]struct{}) {
	modified = map[*ast.File]struct{}{}
	for m, _ := range methods {
		injectLogStatement(m.Decl)
		file := m.File
		if _, ok := modified[file]; !ok {
			modified[file] = exists
			// We should make sure the log package is imported if we are the
			// first one adding a method call that depends on that import.
			ensureImportLogPackage(file)
		}
	}
	return modified
}

// gofmt runs gofmt -w on files.
func gofmt(files []string) error {
	if !gofmtFlag {
		return nil
	}
	args := []string{"-w"}
	args = append(args, files...)
	cmd := exec.Command("gofmt", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// injectInSource modifies methods and saves them in the source tree.
func injectInSource(fset *token.FileSet, methods map[funcDeclRef]error) error {
	modified := doInjection(fset, methods)

	// Warn the user for methods that already have something at their beginning
	// that looks like a logging construct, but it is invalid for some reason.
	for m, err := range methods {
		if _, ok := err.(*errInvalid); ok {
			method := m.Decl
			position := fset.Position(method.Pos())
			methodName := method.Name.Name
			fmt.Printf("Warning: %v: %s: %v\n", position, methodName, err)
		}
	}

	files := []string{}
	for file, _ := range modified {
		filename := fset.Position(file.Pos()).Filename
		files = append(files, filename)

		if err := func() error {
			fileHandle, err := os.OpenFile(filename, os.O_WRONLY, 0644)
			if err != nil {
				return err
			}
			defer fileHandle.Close()

			prn := &printer.Config{
				Mode:     printer.UseSpaces | printer.TabIndent,
				Tabwidth: 8,
			}

			return prn.Fprint(fileHandle, fset, file)
		}(); err != nil {
			return err
		}
	}

	return gofmt(files)
}

// reportResults prints out the validation results from checkMethods
// in a human-readable form.
func reportResults(fset *token.FileSet, methods map[funcDeclRef]error) {
	for m, err := range methods {
		fmt.Printf("%v: %s: %v\n", fset.Position(m.Decl.Pos()), m.Decl.Name.Name, err)
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

// isAddressOfExpression checks if expr is an expression in the form of
// `&expression`
func isAddressOfExpression(expr ast.Expr) (isAddrExpr bool) {
	// TODO: support (&x) as well as &x
	unaryExpr, ok := expr.(*ast.UnaryExpr)
	return ok && unaryExpr.Op == token.AND
}

// newDeferStmtWithSelector returns an abstract syntax node representing
// the statement
//
//     defer x.sel()()
//
func newDeferStmtWithSelector(x ast.Expr, sel *ast.Ident) (deferStatement ast.Stmt) {
	return &ast.DeferStmt{
		Call: &ast.CallExpr{Fun: &ast.CallExpr{Fun: &ast.SelectorExpr{Sel: sel, X: x}}},
	}
}

// findPublicInterfaces returns all the public interfaces defined in packages
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
