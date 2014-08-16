package impl

import (
	"go/build"
	"testing"
)

func TestEnforceDependencyRulesOnPackage(t *testing.T) {
	var dependencyRuleTests = []dependencyRuleTest{
		{DependencyPolicy{
			Outgoing: []DependencyRule{
				{false, "veyron2/..."},
				{true, "..."},
			},
		}, []dependencyRuleTestCase{
			{true, &build.Package{ImportPath: "veyron/test"}, RejectedPolicyAction, 1},
			{true, &build.Package{ImportPath: "veyron2/test"}, ApprovedPolicyAction, 0},
			{true, &build.Package{ImportPath: "fmt", Goroot: true}, UndecidedPolicyAction, 0xc0de},
			{false, &build.Package{ImportPath: "cool-veyron-app"}, UndecidedPolicyAction, 0xcafe},
			{false, &build.Package{ImportPath: "veyron2/testing"}, UndecidedPolicyAction, 42},
			{false, &build.Package{ImportPath: "veyron/testing"}, UndecidedPolicyAction, 0},
		}},
		{DependencyPolicy{
			Outgoing: []DependencyRule{
				DependencyRule{true, "syscall"},
			},
		}, []dependencyRuleTestCase{
			{true, &build.Package{ImportPath: "syscall"}, RejectedPolicyAction, 0},
			{true, &build.Package{ImportPath: "fmt", Goroot: true}, UndecidedPolicyAction, -1},
		}},
	}

	for _, test := range dependencyRuleTests {
		for _, tc := range test.testCases {
			action, index, err := enforceDependencyRulesOnPackage(test.policy.ruleSet(tc.outgoing), tc.pkg)
			if err != nil {
				t.Fatal("error enforcing dependency:", err)
			}
			if action != tc.action || (action != UndecidedPolicyAction && index != tc.index) {
				t.Fatalf("failed to %s %q on rule %d; actual: %s on rule %d", tc.action, tc.pkg.ImportPath, tc.index, action, index)
			}
		}
	}
}

type dependencyRuleTestCase struct {
	outgoing bool
	pkg      *build.Package
	action   DependencyPolicyAction
	index    int
}
type dependencyRuleTest struct {
	policy    DependencyPolicy
	testCases []dependencyRuleTestCase
}

func TestPackages(t *testing.T) {
	var packageTests = []packageTest{
		{"tools/go-depcop/impl/internal/testpackages/test-a", false},
		{"tools/go-depcop/impl/internal/testpackages/test-b", true},
		{"tools/go-depcop/impl/internal/testpackages/test-c", true},
		{"tools/go-depcop/impl/internal/testpackages/test-c/child", false},
		{"tools/go-depcop/impl/internal/testpackages/import-C", false},
		{"tools/go-depcop/impl/internal/testpackages/import-unsafe", false},
		{"tools/go-depcop/impl/internal/testpackages/test-internal", false},
		{"tools/go-depcop/impl/internal/testpackages/test-internal/child", false},
		{"tools/go-depcop/impl/internal/testpackages/test-internal/internal/child", false},
		{"tools/go-depcop/impl/internal/testpackages/test-internal-fail", true},
	}

	for _, test := range packageTests {
		p, err := ImportPackage(test.name)
		if err != nil {
			t.Fatal("error loading package:", err)
		}

		v, err := verifyDependencyHierarchy(p, map[*build.Package]bool{}, nil, false)
		if err != nil {
			t.Fatal("error:", err)
		}

		if test.fail && len(v) == 0 {
			t.Fatalf("%q was expected to fail dependency check but did not", test.name)
		} else if !test.fail && len(v) > 0 {
			t.Fatalf("%q was expected to pass dependency check but did not", test.name)
		}
	}
}

type packageTest struct {
	name string
	fail bool
}

func (a DependencyPolicyAction) String() string {
	return []string{"ignore", "approve", "reject"}[int(a)]
}

func (policy *DependencyPolicy) ruleSet(outgoing bool) []DependencyRule {
	if outgoing {
		return policy.Outgoing
	}
	return policy.Incoming
}
