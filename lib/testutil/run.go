package testutil

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"v.io/tools/lib/runutil"
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

const (
	// numLinesToOutput identifies the number of lines to be included in
	// the error messsage of an xUnit report.
	numLinesToOutput = 50
	// gsPrefix identifies the prefix of a Google Storage location where
	// test results are stored.
	gsPrefix = "gs://vanadium-test-results/v0/"
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

var testMock = func(*util.Context, string, ...TestOpt) (*TestResult, error) {
	return &TestResult{Status: TestPassed}, nil
}

var testFunctions = map[string]func(*util.Context, string, ...TestOpt) (*TestResult, error){
	// TODO(jsimsa,cnicolaou): consider getting rid of the vanadium- prefix.
	"ignore-this":                     testMock,
	"third_party-go-build":            thirdPartyGoBuild,
	"third_party-go-test":             thirdPartyGoTest,
	"third_party-go-race":             thirdPartyGoRace,
	"vanadium-android-test":           vanadiumAndroidTest,
	"vanadium-android-build":          vanadiumAndroidBuild,
	"vanadium-chat-shell-test":        vanadiumChatShellTest,
	"vanadium-chat-web-test":          vanadiumChatWebTest,
	"vanadium-go-bench":               vanadiumGoBench,
	"vanadium-go-build":               vanadiumGoBuild,
	"vanadium-go-cover":               vanadiumGoCoverage,
	"vanadium-go-doc":                 vanadiumGoDoc,
	"vanadium-go-generate":            vanadiumGoGenerate,
	"vanadium-go-race":                vanadiumGoRace,
	"vanadium-go-test":                vanadiumGoTest,
	"vanadium-go-vdl":                 vanadiumGoVDL,
	"vanadium-go-ipc-stress":          vanadiumGoIPCStress,
	"vanadium-integration-test":       vanadiumIntegrationTest,
	"vanadium-js-build-extension":     vanadiumJSBuildExtension,
	"vanadium-js-doc":                 vanadiumJSDoc,
	"vanadium-js-browser-integration": vanadiumJSBrowserIntegration,
	"vanadium-js-node-integration":    vanadiumJSNodeIntegration,
	"vanadium-js-unit":                vanadiumJSUnit,
	"vanadium-js-vdl":                 vanadiumJSVdl,
	"vanadium-js-vom":                 vanadiumJSVom,
	"vanadium-namespace-browser-test": vanadiumNamespaceBrowserTest,
	"vanadium-pipe2browser-test":      vanadiumPipe2BrowserTest,
	"vanadium-playground-test":        vanadiumPlaygroundTest,
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

type TestOpt interface {
	TestOpt()
}

// PrefixOpt is an option that specifies the location where to
// store test results.
type PrefixOpt string

func (PrefixOpt) TestOpt() {}

// PkgsOpt is an option that specifies which Go tests to run using a
// list of Go package expressions.
type PkgsOpt []string

func (PkgsOpt) TestOpt() {}

// ShortOpt is an option that specifies whether to run short tests
// only in VanadiumGoTest and VanadiumGoRace.
type ShortOpt bool

func (ShortOpt) TestOpt() {}

// RunProjectTests runs all tests associated with the given projects.
func RunProjectTests(ctx *util.Context, env map[string]string, projects []string, opts ...TestOpt) (map[string]*TestResult, error) {
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
			if err := runTests(testCtx, []string{test}, results, opts...); err != nil {
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
func RunTests(ctx *util.Context, env map[string]string, tests []string, opts ...TestOpt) (map[string]*TestResult, error) {
	results := make(map[string]*TestResult, len(tests))
	for _, test := range tests {
		results[test] = &TestResult{}
	}
	testCtx := newTestContext(ctx, env)
	if err := runTests(testCtx, tests, results, opts...); err != nil {
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
func runTests(ctx *util.Context, tests []string, results map[string]*TestResult, opts ...TestOpt) error {
	path := ""
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case PrefixOpt:
			path = gsPrefix + string(typedOpt)
		}
	}

	for _, test := range tests {
		testFn, ok := testFunctions[test]
		if !ok {
			return fmt.Errorf("test %v does not exist", test)
		}
		fmt.Fprintf(ctx.Stdout(), "##### Running test %q #####\n", test)

		// Create a 1MB buffer to capture the test function output.
		var out bytes.Buffer
		const largeBufferSize = 1 << 20
		out.Grow(largeBufferSize)
		runOpts := ctx.Run().Opts()
		stdout := io.MultiWriter(&out, runOpts.Stdout)
		stderr := io.MultiWriter(&out, runOpts.Stderr)
		newCtx := util.NewContext(ctx.Env(), ctx.Stdin(), stdout, stderr, ctx.Color(), ctx.DryRun(), ctx.Verbose())

		// Run the test and collect the test results.
		result, err := testFn(newCtx, test, opts...)
		if err != nil {
			r, err := generateXUnitReportForError(newCtx, test, err, out.String())
			if err != nil {
				return err
			}
			result = r
		}
		if path != "" {
			if err := persistTestData(ctx, result, &out, test, path); err != nil {
				fmt.Fprintf(ctx.Stderr(), "failed to store test results: %v\n", err)
			}
		}
		results[test] = result
		fmt.Fprintf(ctx.Stdout(), "##### %s #####\n", results[test].Status)
	}
	return nil
}

// persistTestData uploads test data to Google Storage.
func persistTestData(ctx *util.Context, result *TestResult, output *bytes.Buffer, test, path string) error {
	// Write test data to a temporary directory.
	tmpDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		return err
	}
	defer ctx.Run().RemoveAll(tmpDir)
	conf := struct {
		Arch string
		OS   string
	}{
		Arch: runtime.GOARCH,
		OS:   runtime.GOOS,
	}
	{
		bytes, err := json.Marshal(conf)
		if err != nil {
			return fmt.Errorf("Marshal(%v) failed: %v", err)
		}
		confFile := filepath.Join(tmpDir, "conf")
		if err := ctx.Run().WriteFile(confFile, bytes, os.FileMode(0600)); err != nil {
			return err
		}
	}
	{
		bytes, err := json.Marshal(result)
		if err != nil {
			return fmt.Errorf("Marshal(%v) failed: %v", err)
		}
		resultFile := filepath.Join(tmpDir, "result")
		if err := ctx.Run().WriteFile(resultFile, bytes, os.FileMode(0600)); err != nil {
			return err
		}
	}
	outputFile := filepath.Join(tmpDir, "output")
	if err := ctx.Run().WriteFile(outputFile, output.Bytes(), os.FileMode(0600)); err != nil {
		return err
	}
	// Upload test data to Google Storage.
	{
		args := []string{"cp", "-q", "-m", filepath.Join(tmpDir, "*"), path + "/" + test}
		if err := ctx.Run().Command("gsutil", args...); err != nil {
			return err
		}
	}
	{
		args := []string{"cp", "-q", XUnitReportPath(test), path + "/" + test + "/" + "xunit.xml"}
		if err := ctx.Run().Command("gsutil", args...); err != nil {
			return err
		}
	}
	return nil
}

// generateXUnitReportForError generates an xUnit test report for the
// given (internal) error.
func generateXUnitReportForError(ctx *util.Context, test string, err error, output string) (*TestResult, error) {
	xUnitFilePath := XUnitReportPath(test)

	// Only create the report when the xUnit file doesn't exist, is
	// invalid, or exist but doesn't have failed test cases.
	createXUnitFile := false
	if _, err := os.Stat(xUnitFilePath); err != nil {
		if os.IsNotExist(err) {
			createXUnitFile = true
		} else {
			return nil, fmt.Errorf("Stat(%s) failed: %v", xUnitFilePath, err)
		}
	} else {
		bytes, err := ioutil.ReadFile(xUnitFilePath)
		if err != nil {
			return nil, fmt.Errorf("ReadFile(%s) failed: %v", xUnitFilePath, err)
		}
		var existingSuites testSuites
		if err := xml.Unmarshal(bytes, &existingSuites); err != nil {
			createXUnitFile = true
		} else {
			createXUnitFile = true
			for _, curSuite := range existingSuites.Suites {
				if curSuite.Failures > 0 || curSuite.Errors > 0 {
					createXUnitFile = false
					break
				}
			}
		}
	}

	if createXUnitFile {
		errType := "Internal Error"
		internalErr, ok := err.(internalTestError)
		if ok {
			errType = internalErr.name
		}
		// Create a test suite to encapsulate the error. Include last
		// <numLinesToOutput> lines of the output in the error message.
		lines := strings.Split(output, "\n")
		startLine := int(math.Max(0, float64(len(lines)-numLinesToOutput)))
		consoleOutput := "......\n" + strings.Join(lines[startLine:], "\n")
		errMsg := fmt.Sprintf("Error message:\n%s\n\nConsole output:\n%s\n", internalErr.Error(), consoleOutput)
		s := createTestSuiteWithFailure(test, errType, errType, errMsg, 0)
		suites := []testSuite{*s}

		if err := createXUnitReport(ctx, test, suites); err != nil {
			return nil, err
		}

		if internalErr == runutil.CommandTimedOutErr {
			return &TestResult{Status: TestTimedOut}, nil
		}
	}
	return &TestResult{Status: TestFailed}, nil
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
