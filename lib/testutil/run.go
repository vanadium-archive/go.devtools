package testutil

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"veyron.io/tools/lib/envutil"
	"veyron.io/tools/lib/runutil"
	"veyron.io/tools/lib/util"
)

type TestStatus int

type TestResult struct {
	Status TestStatus
}

const (
	TestPending TestStatus = iota
	TestSkipped
	TestPassed
	TestFailed
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

type testEnv struct {
	snapshot      *envutil.Snapshot
	veyronBin     string
	testFunctions map[string]func(*util.Context, string) (*TestResult, error)
}

func newTestEnv(snapshot *envutil.Snapshot) (*testEnv, error) {
	if snapshot == nil {
		snapshot = envutil.NewSnapshotFromOS()
	}
	bin, err := snapshot.LookPath("veyron")
	if err != nil {
		return nil, err
	}
	t := &testEnv{
		snapshot:  snapshot,
		veyronBin: bin,
	}
	t.testFunctions = map[string]func(*util.Context, string) (*TestResult, error){
		"ignore-this":                                testMock,
		"third_party-go-build":                       t.thirdPartyGoBuild,
		"third_party-go-test":                        t.thirdPartyGoTest,
		"third_party-go-race":                        t.thirdPartyGoRace,
		"veyron-browser-test":                        t.veyronBrowserTest,
		"veyron-go-bench":                            t.veyronGoBench,
		"veyron-go-build":                            t.veyronGoBuild,
		"veyron-go-cover":                            t.veyronGoCoverage,
		"veyron-go-doc":                              t.veyronGoDoc,
		"veyron-go-test":                             t.veyronGoTest,
		"veyron-go-race":                             t.veyronGoRace,
		"veyron-integration-test":                    t.veyronIntegrationTest,
		"veyron-javascript-build-extension":          t.veyronJSBuildExtension,
		"veyron-javascript-doc":                      t.veyronJSDoc,
		"veyron-javascript-browser-integration-test": t.veyronJSBrowserIntegrationTest,
		"veyron-javascript-node-integration-test":    t.veyronJSNodeIntegrationTest,
		"veyron-javascript-unit-test":                t.veyronJSUnitTest,
		"veyron-javascript-vdl":                      t.veyronJSVdlTest,
		"veyron-javascript-vom":                      t.veyronJSVomTest,
		"veyron-presubmit-poll":                      t.veyronPresubmitPoll,
		"veyron-presubmit-test":                      t.veyronPresubmitTest,
		"veyron-prod-services-test":                  t.veyronProdServicesTest,
		"veyron-tutorial":                            t.veyronTutorial,
		"veyron-vdl":                                 t.veyronVDL,
		"veyron-www":                                 t.veyronWWW,
	}
	return t, nil
}

func (t *testEnv) setEnv(key, value string) {
	t.snapshot.Set(key, value)
}

func (t *testEnv) setTestEnv(opts runutil.Opts) runutil.Opts {
	opts.Env = t.snapshot.Map()
	return opts
}

// RunProjectTests runs all tests associated with the given projects.
func RunProjectTests(ctx *util.Context, snapshot *envutil.Snapshot, projects []string) (map[string]*TestResult, error) {
	curTestEnv, err := newTestEnv(snapshot)
	if err != nil {
		return nil, err
	}

	// Parse tests and dependencies from config file.
	var config util.CommonConfig
	if err := util.LoadConfig("common", &config); err != nil {
		return nil, err
	}
	tests, err := projectTests(ctx, config.ProjectTests, projects)
	if err != nil {
		return nil, err
	}
	if len(tests) == 0 {
		return nil, nil
	}
	sort.Strings(tests)
	graph, err := createTestDepGraph(config.TestDependencies, tests)
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
			if err := curTestEnv.runTests(ctx, []string{test}, results); err != nil {
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
func RunTests(ctx *util.Context, snapshot *envutil.Snapshot, tests []string) (map[string]*TestResult, error) {
	curTestEnv, err := newTestEnv(snapshot)
	if err != nil {
		return nil, err
	}

	results := make(map[string]*TestResult, len(tests))
	for _, test := range tests {
		results[test] = &TestResult{}
	}
	if err := curTestEnv.runTests(ctx, tests, results); err != nil {
		return nil, err
	}
	return results, nil
}

// TestList returns a list of all tests known by the testutil package.
func TestList() ([]string, error) {
	result := []string{}
	dummyTestEnv, err := newTestEnv(nil)
	if err != nil {
		return nil, err
	}
	for name := range dummyTestEnv.testFunctions {
		if !strings.HasPrefix(name, "ignore") {
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result, nil
}

// runTests runs the given tests, populating the results map.
func (t *testEnv) runTests(ctx *util.Context, tests []string, results map[string]*TestResult) error {
	for _, test := range tests {
		testFn, ok := t.testFunctions[test]
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

// projectTest returns all the tests for the given projects.
func projectTests(ctx *util.Context, projectsMap map[string][]string, projects []string) ([]string, error) {
	tests := map[string]struct{}{}
	for _, project := range projects {
		if projectTests, ok := projectsMap[project]; ok {
			for _, t := range projectTests {
				tests[t] = struct{}{}
			}
		}
	}
	if len(tests) == 0 {
		fmt.Fprintf(ctx.Stdout(), "no tests found for projects %v.\n", projects)
		return nil, nil
	}
	sortedTests := []string{}
	for test := range tests {
		sortedTests = append(sortedTests, test)
	}
	sort.Strings(sortedTests)
	return sortedTests, nil
}

// createTestDepGraph creates a test dependency graph given a map of
// dependencies and a list of tests.
func createTestDepGraph(testDeps map[string][]string, tests []string) (testDepGraph, error) {
	// For the given list of tests, build a map from the test name
	// to its testInfo object using the dependency data extracted
	// from the given dependency config data "dep".
	depGraph := testDepGraph{}
	for _, test := range tests {
		depTests := []string{}
		if deps, ok := testDeps[test]; ok {
			depTests = deps
		}
		// Make sure the tests in depTests are in the given
		// "tests".
		deps := []string{}
		for _, curDep := range depTests {
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
