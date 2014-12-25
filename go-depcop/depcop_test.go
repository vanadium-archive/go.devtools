package main

import (
	"go/build"
	"os"
	"path/filepath"
	"testing"
)

func init() {
	ctx.BuildTags = []string{"testpackage"}
}

func TestEnforceDependencyRulesOnPackage(t *testing.T) {
	var dependencyRuleTests = []dependencyRuleTest{
		{dependencyPolicy{
			Outgoing: []dependencyRule{
				{false, "veyron2/..."},
				{true, "..."},
			},
		}, []dependencyRuleTestCase{
			{true, &build.Package{ImportPath: "veyron/test"}, rejectedPolicyAction, 1},
			{true, &build.Package{ImportPath: "veyron2/test"}, approvedPolicyAction, 0},
			{true, &build.Package{ImportPath: "fmt", Goroot: true}, undecidedPolicyAction, 0xc0de},
			{false, &build.Package{ImportPath: "cool-veyron-app"}, undecidedPolicyAction, 0xcafe},
			{false, &build.Package{ImportPath: "veyron2/testing"}, undecidedPolicyAction, 42},
			{false, &build.Package{ImportPath: "veyron/testing"}, undecidedPolicyAction, 0},
		}},
		{dependencyPolicy{
			Outgoing: []dependencyRule{
				dependencyRule{true, "syscall"},
			},
		}, []dependencyRuleTestCase{
			{true, &build.Package{ImportPath: "syscall"}, rejectedPolicyAction, 0},
			{true, &build.Package{ImportPath: "fmt", Goroot: true}, undecidedPolicyAction, -1},
		}},
	}

	for _, test := range dependencyRuleTests {
		for _, tc := range test.testCases {
			action, index, err := enforceDependencyRulesOnPackage(test.policy.ruleSet(tc.outgoing), tc.pkg)
			if err != nil {
				t.Fatal("error enforcing dependency:", err)
			}
			if action != tc.action || (action != undecidedPolicyAction && index != tc.index) {
				t.Fatalf("failed to %s %q on rule %d; actual: %s on rule %d", tc.action, tc.pkg.ImportPath, tc.index, action, index)
			}
		}
	}
}

type dependencyRuleTestCase struct {
	outgoing bool
	pkg      *build.Package
	action   dependencyPolicyAction
	index    int
}
type dependencyRuleTest struct {
	policy    dependencyPolicy
	testCases []dependencyRuleTestCase
}

func TestVerifyDependency(t *testing.T) {
	var packageTests = []packageTest{
		{"veyron.io/tools/go-depcop/testdata/test-a", false},
		{"veyron.io/tools/go-depcop/testdata/test-b", true},
		{"veyron.io/tools/go-depcop/testdata/test-c", true},
		{"veyron.io/tools/go-depcop/testdata/test-c/child", false},
		{"veyron.io/tools/go-depcop/testdata/import-C", false},
		{"veyron.io/tools/go-depcop/testdata/import-unsafe", false},
		{"veyron.io/tools/go-depcop/testdata/test-internal", false},
		{"veyron.io/tools/go-depcop/testdata/test-internal/child", false},
		{"veyron.io/tools/go-depcop/testdata/test-internal/internal/child", false},
		{"veyron.io/tools/go-depcop/testdata/test-internal-fail", true},
	}

	for _, test := range packageTests {
		p, err := importPackage(test.name)
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
			t.Fatalf("%q was expected to pass dependency check but did not: %v", test.name, v)
		}
	}
}

type packageTest struct {
	name string
	fail bool
}

func (a dependencyPolicyAction) string() string {
	return []string{"ignore", "approve", "reject"}[int(a)]
}

func (policy *dependencyPolicy) ruleSet(outgoing bool) []dependencyRule {
	if outgoing {
		return policy.Outgoing
	}
	return policy.Incoming
}

func TestComputeIncomingDependency(t *testing.T) {
	root := os.Getenv("VANADIUM_ROOT")
	if root == "" {
		t.Fatalf("VANADIUM_ROOT not set")
	}
	oldPath := os.Getenv("GOPATH")
	defer os.Setenv("GOPATH", oldPath)
	if err := os.Setenv("GOPATH", filepath.Join(root, "veyron", "go")); err != nil {
		t.Fatalf("Setenv(%v, %v) failed: %v", "GOPATH", filepath.Join(root, "veyron", "go"))
	}
	allDeps, err := computeIncomingDependencies()
	if err != nil {
		t.Fatalf("%v\n", err)
	}
	this, that := "veyron.io/tools/lib/version", "veyron.io/tools/go-depcop"
	if deps, ok := allDeps[this]; !ok {
		t.Fatalf("no incoming dependencies for %v", this)
	} else {
		if _, ok := deps[that]; !ok {
			t.Fatalf("missing incoming dependency for %v -> %v", that, this)
		}
	}
}
