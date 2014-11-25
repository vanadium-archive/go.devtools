package testutil

import (
	"fmt"
	"sort"
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

// RunProjectTests runs all tests associated with the given project.
func RunProjectTests(ctx *util.Context, project string) (map[string]*TestResult, error) {
	// Parse tests and dependencies from config file.
	var config util.CommonConfig
	if err := util.LoadConfig("common", &config); err != nil {
		return nil, err
	}
	tests, err := projectTests(ctx, config.ProjectTests, project)
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
			if err := runTests(ctx, []string{test}, results); err != nil {
				return nil, err
			}
			continue run
		}
		// The following line should be never reached.
		return nil, fmt.Errorf("erroneous test running logic")
	}

	return results, nil
}

var testFunctions = map[string]func(*util.Context, string) (*TestResult, error){
	"third_party-go-build":                  ThirdPartyGoBuild,
	"third_party-go-test":                   ThirdPartyGoTest,
	"third_party-go-race":                   ThirdPartyGoRace,
	"veyron-browser-test":                   VeyronBrowserTest,
	"veyron-go-bench":                       VeyronGoBench,
	"veyron-go-build":                       VeyronGoBuild,
	"veyron-go-cover":                       VeyronGoCoverage,
	"veyron-go-doc":                         VeyronGoDoc,
	"veyron-go-test":                        VeyronGoTest,
	"veyron-go-race":                        VeyronGoRace,
	"veyron-integration-test":               VeyronIntegrationTest,
	"veyron-javascript-build-extension":     VeyronJSBuildExtension,
	"veyron-javascript-build-nacl-compiler": VeyronJSBuildNaClCompiler,
	"veyron-javascript-doc":                 VeyronJSDoc,
	"veyron-javascript-test-intergration":   VeyronJSIntegrationTest,
	"veyron-javascript-test-unit":           VeyronJSUnitTest,
	"veyron-javascript-vdl":                 VeyronJSVdlTest,
	"veyron-javascript-vom":                 VeyronJSVomTest,
	"veyron-presubmit-poll":                 VeyronPresubmitPoll,
	"veyron-presubmit-test":                 VeyronPresubmitTest,
	"veyron-tutorial":                       VeyronTutorial,
	"veyron-vdl":                            VeyronVDL,
}

// RunTests executes the given tests and reports the test results.
func RunTests(ctx *util.Context, tests []string) (map[string]*TestResult, error) {
	results := make(map[string]*TestResult, len(tests))
	for _, test := range tests {
		results[test] = &TestResult{}
	}
	if err := runTests(ctx, tests, results); err != nil {
		return nil, err
	}
	return results, nil
}

// runTests runs the given tests, populating the results map.
func runTests(ctx *util.Context, tests []string, results map[string]*TestResult) error {
	for _, test := range tests {
		testFn, ok := testFunctions[test]
		if ok {
			fmt.Fprintf(ctx.Stdout(), "##### Running test %q #####\n", test)
			result, err := testFn(ctx, test)
			if err != nil {
				return err
			}
			results[test] = result
			fmt.Fprintf(ctx.Stdout(), "##### %s #####\n", results[test].Status)
			continue
		}
		testScript, err := util.TestScriptFile(test)
		if err != nil {
			return err
		}
		opts := ctx.Run().Opts()
		env := envutil.NewSnapshotFromOS()
		env.Set("VEYRON_NO_UPDATE", "1")
		opts.Env = env.Map()
		fmt.Fprintf(ctx.Stdout(), "##### Running test %q #####\n", test)
		if err := ctx.Run().TimedCommandWithOpts(DefaultTestTimeout, opts, testScript); err != nil {
			if err == runutil.CommandTimedOutErr {
				results[test].Status = TestTimedOut
			} else {
				results[test].Status = TestFailed
			}
		} else {
			results[test].Status = TestPassed
		}
		fmt.Fprintf(ctx.Stdout(), "##### %s #####\n", results[test].Status)
	}
	return nil
}

// projectTest returns all the tests for the given project.
func projectTests(ctx *util.Context, projects map[string][]string, project string) ([]string, error) {
	tests, ok := projects[project]
	if !ok {
		fmt.Fprintf(ctx.Stdout(), "project %q entry not found; not running any tests.\n", project)
		return nil, nil
	}
	return tests, nil
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
