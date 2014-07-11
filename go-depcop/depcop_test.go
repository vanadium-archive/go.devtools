package main

import (
	"go/build"
	"testing"
)

// TODO: add tests for traversing the directory hierarchy

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

func (a DependencyPolicyAction) String() string {
	return []string{"ignore", "approve", "reject"}[int(a)]
}

func (policy *DependencyPolicy) ruleSet(outgoing bool) []DependencyRule {
	if outgoing {
		return policy.Outgoing
	}
	return policy.Incoming
}

func TestEnforceDependencyRulesOnPackage(t *testing.T) {
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
