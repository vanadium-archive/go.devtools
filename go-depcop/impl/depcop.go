package impl

import (
	"fmt"
	"go/build"
	"regexp"
	"strings"
)

type DependencyRule struct {
	IsDenyRule        bool
	PackageExpression string
}

type DependencyPolicy struct {
	Incoming, Outgoing []DependencyRule
}

type PackageConfig struct {
	Dependencies DependencyPolicy
	Path         string
}

type DependencyPolicyAction int

const (
	UndecidedPolicyAction DependencyPolicyAction = iota
	ApprovedPolicyAction
	RejectedPolicyAction
)

type DependencyDirection int

const (
	IncomingDependency DependencyDirection = iota
	OutgoingDependency
)

type DependencyRuleReference struct {
	Package, MatchingPackage *build.Package
	Path                     string
	Direction                DependencyDirection
	RuleIndex                int
	RuleSet                  []DependencyRule
}

type dependencyViolationError struct{}

func (*dependencyViolationError) Error() string {
	return "dependency policy violation"
}

func IsDependencyViolation(a error) bool {
	_, ok := a.(*dependencyViolationError)
	return ok
}

func (r DependencyRule) Enforce(p *build.Package) (DependencyPolicyAction, error) {
	if r.PackageExpression == "..." {
		if p.Goroot {
			return UndecidedPolicyAction, nil
		}
		if r.IsDenyRule {
			return RejectedPolicyAction, nil
		}
		return ApprovedPolicyAction, nil
	}

	re := regexp.QuoteMeta(r.PackageExpression)
	if strings.HasSuffix(re, `/\.\.\.`) {
		re = re[:len(re)-len(`/\.\.\.`)] + `(/.*)?`
	}

	if matched, err := regexp.MatchString("^"+re+"$", p.ImportPath); err != nil {
		return UndecidedPolicyAction, err
	} else if matched {
		if r.IsDenyRule {
			return RejectedPolicyAction, nil
		}
		return ApprovedPolicyAction, nil
	}
	return UndecidedPolicyAction, nil
}

func enforceDependencyRulesOnPackage(rules []DependencyRule, p *build.Package) (DependencyPolicyAction, int, error) {
	for i, r := range rules {
		if x, err := r.Enforce(p); err != nil {
			return x, i, err
		} else if x != UndecidedPolicyAction {
			return x, i, nil
		}
	}
	return UndecidedPolicyAction, -1, nil
}

func validateDependencyRelationship(p, x *build.Package, direction DependencyDirection) (DependencyRuleReference, error) {
	it := NewPackageConfigFileIterator(p)

	for it.Advance() {
		c := it.Value()
		ruleSet := c.Dependencies.Incoming
		if direction == OutgoingDependency {
			ruleSet = c.Dependencies.Outgoing
		}

		action, index, err := enforceDependencyRulesOnPackage(ruleSet, x)

		ref := DependencyRuleReference{
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
		case ApprovedPolicyAction:
			return ref, nil
		case RejectedPolicyAction:
			return ref, &dependencyViolationError{}
		}
	}

	if err := it.Err(); err != nil {
		return DependencyRuleReference{}, err
	}

	return DependencyRuleReference{}, nil
}

func printDependencyHierarchy(p *build.Package, visited map[*build.Package]bool, depth int) error {
	for i := 0; i < depth-1; i++ {
		fmt.Print(" │")
	}
	if depth > 0 {
		fmt.Print(" ├─")
	} else {
		fmt.Print("#")
	}

	fmt.Println(p.ImportPath)

	if visited[p] {
		return nil
	}

	visited[p] = true
	for _, dep := range p.Imports {
		pkg, err := ImportPackage(dep)
		if err != nil {
			return err
		}
		if err := printDependencyHierarchy(pkg, visited, depth+1); err != nil {
			return err
		}
	}

	return nil
}

func verifyDependencyHierarchy(p *build.Package, visited map[*build.Package]bool, parent *build.Package, recurse bool) ([]DependencyRuleReference, error) {
	v := []DependencyRuleReference{}

	if parent != nil {
		r, err := validateDependencyRelationship(parent, p, OutgoingDependency)
		if err != nil {
			if IsDependencyViolation(err) {
				v = append(v, r)
			} else {
				return v, err
			}
		}
		r, err = validateDependencyRelationship(p, parent, IncomingDependency)
		if err != nil {
			if IsDependencyViolation(err) {
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
		for _, importPath := range p.Imports {
			dependency, err := ImportPackage(importPath)
			if err == nil {
				var depViolation []DependencyRuleReference
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
