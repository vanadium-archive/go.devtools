package testutil

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"v.io/tools/lib/util"
)

type TestStatus int

type TestResult struct {
	Status          TestStatus
	TimeoutValue    time.Duration       // Used when Status == TestTimedOut
	MergeConflictCL string              // Used when Status == TestFailedMergeConflict
	ExcludedTests   map[string][]string // Tests that are excluded within packages keyed by package name
	SkippedTests    map[string][]string // Tests that are skipped within packages keyed by package name
}

const (
	TestPending TestStatus = iota
	TestSkipped
	TestPassed
	TestFailed
	TestFailedMergeConflict
	TestTimedOut
)

func (s TestStatus) String() string {
	switch s {
	case TestSkipped:
		return "SKIPPED"
	case TestPassed:
		return "PASSED"
	case TestFailed:
		return "FAILED"
	case TestFailedMergeConflict:
		return "MERGE CONFLICT"
	case TestTimedOut:
		return "TIMED OUT"
	default:
		return "UNKNOWN"
	}
}

// testNode represents a node of a test dependency graph.
type testNode struct {
	// deps is a list of its dependencies.
	deps []string
	// visited determines whether a DFS exploration of the test
	// dependency graph has visited this test.
	visited bool
	// stack determines whether this test is on the search stack
	// of a DFS exploration of the test dependency graph.
	stack bool
}

// testDepGraph captures the test dependency graph.
type testDepGraph map[string]*testNode

// DefaultTestTimeout identified the maximum time each test is allowed
// to run before being forcefully terminated.
var DefaultTestTimeout = 10 * time.Minute

var testMock = func(*util.Context, string) (*TestResult, error) {
	return &TestResult{Status: TestPassed}, nil
}

var testFunctions = map[string]func(*util.Context, string) (*TestResult, error){
	"ignore-this":                     testMock,
	"third_party-go-build":            thirdPartyGoBuild,
	"third_party-go-test":             thirdPartyGoTest,
	"third_party-go-race":             thirdPartyGoRace,
	"vanadium-chat-shell-test":        vanadiumChatShellTest,
	"vanadium-chat-web-test":          vanadiumChatWebTest,
	"vanadium-namespace-browser-test": vanadiumNamespaceBrowserTest,
	"vanadium-go-bench":               vanadiumGoBench,
	"vanadium-go-build":               vanadiumGoBuild,
	"vanadium-go-cover":               vanadiumGoCoverage,
	"vanadium-go-doc":                 vanadiumGoDoc,
	"vanadium-go-generate":            vanadiumGoGenerate,
	"vanadium-go-race":                vanadiumGoRace,
	"vanadium-go-test":                vanadiumGoTest,
	"vanadium-go-vdl":                 vanadiumGoVDL,
	"vanadium-integration-test":       vanadiumIntegrationTest,
	"vanadium-integration-test-new":   vanadiumNewIntegrationTest,
	"vanadium-js-build-extension":     vanadiumJSBuildExtension,
	"vanadium-js-doc":                 vanadiumJSDoc,
	"vanadium-js-browser-integration": vanadiumJSBrowserIntegration,
	"vanadium-js-node-integration":    vanadiumJSNodeIntegration,
	"vanadium-js-unit":                vanadiumJSUnit,
	"vanadium-js-vdl":                 vanadiumJSVdl,
	"vanadium-js-vom":                 vanadiumJSVom,
	"vanadium-postsubmit-poll":        vanadiumPostsubmitPoll,
	"vanadium-presubmit-poll":         vanadiumPresubmitPoll,
	"vanadium-presubmit-result":       vanadiumPresubmitResult,
	"vanadium-presubmit-test":         vanadiumPresubmitTest,
	"vanadium-prod-services-test":     vanadiumProdServicesTest,
	"vanadium-www-site":               vanadiumWWWSite,
	"vanadium-www-tutorials":          vanadiumWWWTutorials,
}

func newTestContext(ctx *util.Context, env map[string]string) *util.Context {
	tmpEnv := map[string]string{}
	for key, value := range ctx.Env() {
		tmpEnv[key] = value
	}
	for key, value := range env {
		tmpEnv[key] = value
	}
	return util.NewContext(tmpEnv, ctx.Stdin(), ctx.Stdout(), ctx.Stderr(), ctx.Color(), ctx.DryRun(), ctx.Verbose())
}

// RunProjectTests runs all tests associated with the given projects.
func RunProjectTests(ctx *util.Context, env map[string]string, projects []string) (map[string]*TestResult, error) {
	testCtx := newTestContext(ctx, env)

	// Parse tests and dependencies from config file.
	var config util.Config
	if err := util.LoadConfig("common", &config); err != nil {
		return nil, err
	}
	tests := config.ProjectTests(projects)
	if len(tests) == 0 {
		return nil, nil
	}
	sort.Strings(tests)
	graph, err := createTestDepGraph(&config, tests)
	if err != nil {
		return nil, err
	}

	// Run tests.
	//
	// TODO(jingjin) parallelize the top-level scheduling loop so
	// that tests do not need to run serially.
	results := make(map[string]*TestResult, len(tests))
	for _, test := range tests {
		results[test] = &TestResult{}
	}
run:
	for i := 0; i < len(graph); i++ {
		// Find a test that can execute.
		for _, test := range tests {
			result, node := results[test], graph[test]
			if result.Status != TestPending {
				continue
			}
			ready := true
			for _, dep := range node.deps {
				switch results[dep].Status {
				case TestSkipped, TestFailed, TestTimedOut:
					results[test].Status = TestSkipped
					continue run
				case TestPending:
					ready = false
					break
				}
			}
			if !ready {
				continue
			}
			if err := runTests(testCtx, []string{test}, results); err != nil {
				return nil, err
			}
			continue run
		}
		// The following line should be never reached.
		return nil, fmt.Errorf("erroneous test running logic")
	}

	return results, nil
}

// RunTests executes the given tests and reports the test results.
func RunTests(ctx *util.Context, env map[string]string, tests []string) (map[string]*TestResult, error) {
	results := make(map[string]*TestResult, len(tests))
	for _, test := range tests {
		results[test] = &TestResult{}
	}
	testCtx := newTestContext(ctx, env)
	if err := runTests(testCtx, tests, results); err != nil {
		return nil, err
	}
	return results, nil
}

// TestList returns a list of all tests known by the testutil package.
func TestList() ([]string, error) {
	result := []string{}
	for name := range testFunctions {
		if !strings.HasPrefix(name, "ignore") {
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result, nil
}

// runTests runs the given tests, populating the results map.
//
// TODO(jingjin): move the logic that wraps internal errors in a
// TestResult and creates an xUnit report if one does not exists from
// the test function to this wrapper.
func runTests(ctx *util.Context, tests []string, results map[string]*TestResult) error {
	for _, test := range tests {
		testFn, ok := testFunctions[test]
		if !ok {
			return fmt.Errorf("test %v does not exist", test)
		}
		fmt.Fprintf(ctx.Stdout(), "##### Running test %q #####\n", test)
		result, err := testFn(ctx, test)
		if err != nil {
			return err
		}
		results[test] = result
		fmt.Fprintf(ctx.Stdout(), "##### %s #####\n", results[test].Status)
	}
	return nil
}

// createTestDepGraph creates a test dependency graph given a map of
// dependencies and a list of tests.
func createTestDepGraph(config *util.Config, tests []string) (testDepGraph, error) {
	// For the given list of tests, build a map from the test name
	// to its testInfo object using the dependency data extracted
	// from the given dependency config data "dep".
	depGraph := testDepGraph{}
	for _, test := range tests {
		// Make sure the test dependencies are included in <tests>.
		deps := []string{}
		for _, curDep := range config.TestDependencies(test) {
			isDepInTests := false
			for _, test := range tests {
				if curDep == test {
					isDepInTests = true
					break
				}
			}
			if isDepInTests {
				deps = append(deps, curDep)
			}
		}
		depGraph[test] = &testNode{
			deps: deps,
		}
	}

	// Detect dependency loop using depth-first search.
	for name, info := range depGraph {
		if info.visited {
			continue
		}
		if findCycle(name, depGraph) {
			return nil, fmt.Errorf("found dependency loop: %v", depGraph)
		}
	}
	return depGraph, nil
}

// findCycle checks whether there are any cycles in the test
// dependency graph.
func findCycle(name string, depGraph testDepGraph) bool {
	node := depGraph[name]
	node.visited = true
	node.stack = true
	for _, dep := range node.deps {
		depNode := depGraph[dep]
		if depNode.stack {
			return true
		}
		if depNode.visited {
			continue
		}
		if findCycle(dep, depGraph) {
			return true
		}
	}
	node.stack = false
	return false
}
