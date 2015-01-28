package main

import (
	"fmt"
	"go/build"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type dependencyRule struct {
	IsDenyRule        bool
	PackageExpression string
}

type dependencyPolicy struct {
	Incoming, Outgoing []dependencyRule
}

type packageConfig struct {
	Dependencies dependencyPolicy
	Path         string
}

type dependencyPolicyAction int

const (
	undecidedPolicyAction dependencyPolicyAction = iota
	approvedPolicyAction
	rejectedPolicyAction
)

type dependencyDirection int

const (
	incomingDependency dependencyDirection = iota
	outgoingDependency
)

type dependencyRuleReference struct {
	Package, MatchingPackage *build.Package
	InternalPackage          bool
	Path                     string
	Direction                dependencyDirection
	RuleIndex                int
	RuleSet                  []dependencyRule
}

type dependencyViolationError struct{}

func (*dependencyViolationError) Error() string {
	return "dependency policy violation"
}

func isDependencyViolation(a error) bool {
	_, ok := a.(*dependencyViolationError)
	return ok
}

func (r dependencyRule) enforce(p *build.Package) (dependencyPolicyAction, error) {
	if r.PackageExpression == "..." {
		if p.Goroot {
			return undecidedPolicyAction, nil
		}
		if r.IsDenyRule {
			return rejectedPolicyAction, nil
		}
		return approvedPolicyAction, nil
	}

	re := regexp.QuoteMeta(r.PackageExpression)
	if strings.HasSuffix(re, `/\.\.\.`) {
		re = re[:len(re)-len(`/\.\.\.`)] + `(/.*)?`
	}

	if matched, err := regexp.MatchString("^"+re+"$", p.ImportPath); err != nil {
		return undecidedPolicyAction, err
	} else if matched {
		if r.IsDenyRule {
			return rejectedPolicyAction, nil
		}
		return approvedPolicyAction, nil
	}
	return undecidedPolicyAction, nil
}

func collectDirs(root, suffix string, dirs map[string]bool) error {
	path := filepath.Join(root, suffix)
	fis, err := ioutil.ReadDir(path)
	if err != nil {
		return fmt.Errorf("ReadDir(%v) failed: %v", path, err)
	}
	for _, fi := range fis {
		if fi.IsDir() {
			suffix2 := filepath.Join(suffix, fi.Name())
			dirs[suffix2] = true
			collectDirs(root, suffix2, dirs)
		}
	}
	return nil
}

func computeIncomingDependencies() (map[string]map[string]bool, error) {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		return nil, fmt.Errorf("GOPATH is not set")
	}
	dirs := strings.Split(gopath, ":")
	allDirs := map[string]bool{}
	for _, dir := range dirs {
		if err := collectDirs(filepath.Join(dir, "src"), "", allDirs); err != nil {
			return nil, err
		}
	}
	allDeps := map[string]map[string]bool{}
	for dir, _ := range allDirs {
		allDeps[dir] = map[string]bool{}
	}
	for dir, _ := range allDirs {
		mode := build.ImportMode(0)
		pkg, err := build.Import(dir, "", mode)
		if err != nil {
			fmt.Errorf("Import(%v, %v) failed: %v", dir, mode, err)
		}
		imports := pkg.Imports
		if includeTestsFlag {
			imports = append(imports, pkg.TestImports...)
		}
		for _, dep := range imports {
			if deps, ok := allDeps[dep]; ok {
				deps[dir] = true
			}
		}
	}
	return allDeps, nil
}

func enforceDependencyRulesOnPackage(rules []dependencyRule, p *build.Package) (dependencyPolicyAction, int, error) {
	for i, r := range rules {
		if x, err := r.enforce(p); err != nil {
			return x, i, err
		} else if x != undecidedPolicyAction {
			return x, i, nil
		}
	}
	return undecidedPolicyAction, -1, nil
}

func validateDependencyRelationship(p, x *build.Package, direction dependencyDirection) (dependencyRuleReference, error) {
	it := newPackageConfigFileIterator(p)

	for it.Advance() {
		c := it.Value()
		ruleSet := c.Dependencies.Outgoing
		if direction == incomingDependency {
			ruleSet = c.Dependencies.Incoming
		}

		action, index, err := enforceDependencyRulesOnPackage(ruleSet, x)
		ref := dependencyRuleReference{
			Package:         p,
			MatchingPackage: x,
			Path:            c.Path,
			Direction:       direction,
			RuleIndex:       index,
			RuleSet:         ruleSet,
		}

		if err != nil {
			return ref, err
		}

		switch action {
		case approvedPolicyAction:
			return ref, nil
		case rejectedPolicyAction:
			return ref, &dependencyViolationError{}
		}

		if direction == incomingDependency {
			pkgConfDir := filepath.Dir(c.Path)
			pkgName := filepath.Base(pkgConfDir)
			if pkgName == "internal" {
				internalPackagePrefix := filepath.Dir(pkgConfDir)
				if internalPackagePrefix != x.Dir && !strings.HasPrefix(x.Dir, internalPackagePrefix+"/") {
					return dependencyRuleReference{
						Package:         p,
						MatchingPackage: x,
						Path:            c.Path,
						Direction:       direction,
						InternalPackage: true,
					}, &dependencyViolationError{}
				}
			}
		}
	}

	if err := it.Err(); err != nil {
		return dependencyRuleReference{}, err
	}

	return dependencyRuleReference{}, nil
}

func printDependencyHierarchy(stdout io.Writer, p *build.Package, visited map[*build.Package]bool, depth int) error {
	if prettyFlag {
		for i := 0; i < depth-1; i++ {
			fmt.Fprintf(stdout, " │")
		}
		if depth > 0 {
			fmt.Fprintf(stdout, " ├─")
		} else {
			fmt.Fprintf(stdout, "#")
		}
		fmt.Fprintln(stdout, p.ImportPath)
	} else {
		if depth > 0 {
			fmt.Fprintln(stdout, p.ImportPath)
		}
	}

	if visited[p] || (!transitiveFlag && depth == 1) {
		return nil
	}

	visited[p] = true
	imports := p.Imports
	if includeTestsFlag {
		imports = append(imports, p.TestImports...)
	}
	for _, dep := range imports {
		pkg, err := importPackage(dep)
		if err != nil {
			return err
		}
		if gorootFlag || !pkg.Goroot {
			if err := printDependencyHierarchy(stdout, pkg, visited, depth+1); err != nil {
				return err
			}
		}
	}

	return nil
}

func verifyDependencyHierarchy(p *build.Package, visited map[*build.Package]bool, parent *build.Package, recurse bool) ([]dependencyRuleReference, error) {
	v := []dependencyRuleReference{}

	if parent != nil {
		r, err := validateDependencyRelationship(parent, p, outgoingDependency)
		if err != nil {
			if isDependencyViolation(err) {
				v = append(v, r)
			} else {
				return v, err
			}
		}
		r, err = validateDependencyRelationship(p, parent, incomingDependency)
		if err != nil {
			if isDependencyViolation(err) {
				v = append(v, r)
			} else {
				return v, err
			}
		}
	}

	if visited[p] {
		return nil, nil
	}
	visited[p] = true
	if parent == nil || recurse {
		imports := p.Imports
		if includeTestsFlag {
			imports = append(imports, p.TestImports...)
		}
		for _, importPath := range imports {
			dependency, err := importPackage(importPath)
			if err == nil {
				var depViolation []dependencyRuleReference
				depViolation, err = verifyDependencyHierarchy(dependency, visited, p, recurse)
				if depViolation != nil {
					v = append(v, depViolation...)
				}
			}
			if err != nil {
				return v, err
			}
		}
	}
	return v, nil
}
