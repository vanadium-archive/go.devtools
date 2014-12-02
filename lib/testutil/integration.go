package testutil

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"veyron.io/tools/lib/envutil"
	"veyron.io/tools/lib/util"
)

const (
	DefaultIntegrationTestTimeout = 2 * time.Minute
)

// binPackages enumerates the Go commands used by veyron integration tests.
//
// TODO(jingjin): port the integration test scripts from shell to Go
// and make them use a build cache to share binaries.
var binPackages = []string{
	"veyron.io/apps/tunnel/tunneld",
	"veyron.io/apps/tunnel/vsh",
	"veyron.io/playground/builder",
	"veyron.io/veyron/veyron/security/agent/agentd",
	"veyron.io/veyron/veyron/security/agent/pingpong",
	"veyron.io/veyron/veyron/services/mgmt/application/applicationd",
	"veyron.io/veyron/veyron/services/mgmt/binary/binaryd",
	"veyron.io/veyron/veyron/services/mgmt/build/buildd",
	"veyron.io/veyron/veyron/services/mgmt/profile/profiled",
	"veyron.io/veyron/veyron/services/mounttable/mounttabled",
	"veyron.io/veyron/veyron/services/proxy/proxyd",
	"veyron.io/veyron/veyron/tools/application",
	"veyron.io/veyron/veyron/tools/binary",
	"veyron.io/veyron/veyron/tools/build",
	"veyron.io/veyron/veyron/tools/debug",
	"veyron.io/veyron/veyron/tools/mounttable",
	"veyron.io/veyron/veyron/tools/principal",
	"veyron.io/veyron/veyron/tools/profile",
	"veyron.io/veyron/veyron/tools/naming/simulator",
	"veyron.io/veyron/veyron2/vdl/vdl",
	"veyron.io/wspr/veyron/services/wsprd",
}

// buildBinaries builds Go binaries enumerated by the binPackages list.
func buildBinaries(ctx *util.Context, testName string) (*TestResult, error) {
	// Create a pool of workers.
	fmt.Fprintf(ctx.Stdout(), "building binaries...\n")
	numPkgs := len(binPackages)
	tasks := make(chan string, numPkgs)
	taskResults := make(chan buildResult, numPkgs)
	for i := 0; i < runtime.NumCPU(); i++ {
		go buildWorker(nil, tasks, taskResults)
	}

	// Distribute work to workers.
	for _, pkg := range binPackages {
		tasks <- pkg
	}
	close(tasks)

	// Collect the results.
	allPassed, suites := true, []testSuite{}
	for i := 0; i < numPkgs; i++ {
		result := <-taskResults
		s := testSuite{Name: result.pkg}
		c := testCase{
			Classname: result.pkg,
			Name:      "Build",
			Time:      fmt.Sprintf("%.2f", result.time.Seconds()),
		}
		if result.status != buildPassed {
			fmt.Fprintf(ctx.Stdout(), "%s ... failed\n%v\n", result.pkg, result.output)
			f := testFailure{
				Message: "build",
				Data:    result.output,
			}
			c.Failures = append(c.Failures, f)
			allPassed = false
			s.Failures++
		} else {
			fmt.Fprintf(ctx.Stdout(), "%s ... ok\n", result.pkg)
		}
		s.Tests++
		s.Cases = append(s.Cases, c)
		suites = append(suites, s)
	}

	// Create the xUnit report.
	close(taskResults)
	if err := createXUnitReport(ctx, testName, suites); err != nil {
		return nil, err
	}
	if !allPassed {
		return &TestResult{Status: TestFailed}, nil
	}
	return &TestResult{Status: TestPassed}, nil
}

// findTestScripts finds all test.sh file from the given root dirs.
func findTestScripts(ctx *util.Context, rootDirs []string) []string {
	if ctx.DryRun() {
		// In "dry run" mode, no test scripts are executed.
		return nil
	}
	matchedFiles := []string{}
	for _, rootDir := range rootDirs {
		filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
			if strings.HasSuffix(path, string(os.PathSeparator)+"test.sh") {
				matchedFiles = append(matchedFiles, path)
			}
			return nil
		})
	}
	return matchedFiles
}

// runTestScripts runs all test.sh scripts found under
// $VEYRON_ROOT/roadmap/go/src and $VEYRON_ROOT/veyron/go/src.
func runTestScripts(ctx *util.Context, testName string) (*TestResult, error) {
	root, err := util.VeyronRoot()
	if err != nil {
		return nil, err
	}

	// Find all test.sh scripts.
	testScripts := findTestScripts(ctx, []string{
		filepath.Join(root, "veyron", "go", "src"),
		filepath.Join(root, "roadmap", "go", "src"),
	})

	// Create a worker pool to run tests in parallel, passing the
	// location of binaries through shell_test_BIN_DIR.
	fmt.Fprintf(ctx.Stdout(), "running tests...\n")
	numTests := len(testScripts)
	tasks := make(chan string, numTests)
	taskResults := make(chan testResult, numTests)
	env := envutil.NewSnapshotFromOS()
	env.Set("shell_test_BIN_DIR", binDirPath())
	for i := 0; i < runtime.NumCPU(); i++ {
		go testScriptWorker(root, env.Map(), tasks, taskResults)
	}

	// Send test scripts to free workers in the pool.
	for _, testScript := range testScripts {
		tasks <- testScript
	}
	close(tasks)

	// Collect the results.
	allPassed, suites := true, []testSuite{}
	for i := 0; i < numTests; i++ {
		result := <-taskResults
		s := testSuite{Name: result.pkg}
		c := testCase{
			Classname: result.pkg,
			Name:      "Integration Test",
			Time:      fmt.Sprintf("%.2f", result.time.Seconds()),
		}
		switch result.status {
		case testFailed:
			fmt.Fprintf(ctx.Stdout(), "%s ... failed\n%v\n", result.pkg, result.output)
			f := testFailure{
				Message: "test",
				Data:    result.output,
			}
			c.Failures = append(c.Failures, f)
			allPassed = false
			s.Failures++
		case testPassed:
			fmt.Fprintf(ctx.Stdout(), "%s ... ok\n", result.pkg)
		}
		s.Tests++
		s.Cases = append(s.Cases, c)
		suites = append(suites, s)
	}
	close(taskResults)

	// Create the xUnit report.
	if err := createXUnitReport(ctx, testName, suites); err != nil {
		return nil, err
	}
	if !allPassed {
		return &TestResult{Status: TestFailed}, nil
	}
	return &TestResult{Status: TestPassed}, nil
}

// testScriptWorker receives tasks from the <tasks> channel, runs
// them, and sends results to the <results> channel.
func testScriptWorker(root string, env map[string]string, tasks <-chan string, results chan<- testResult) {
	var out bytes.Buffer
	ctx := util.NewContext(env, os.Stdin, &out, &out, false, false)
	for script := range tasks {
		start := time.Now()
		err := ctx.Run().TimedCommand(DefaultIntegrationTestTimeout, script)
		pkgName := path.Dir(script)
		if index := strings.Index(pkgName, "veyron.io"); index != -1 {
			pkgName = pkgName[index:]
		}
		result := testResult{
			pkg:    pkgName,
			time:   time.Now().Sub(start),
			output: out.String(),
		}
		if err != nil {
			result.status = testFailed
		} else {
			result.status = testPassed
		}
		results <- result
		out.Reset()
	}
}

// VeyronIntegrationTest runs veyron integration tests.
func VeyronIntegrationTest(ctx *util.Context, testName string) (*TestResult, error) {
	// Initialize the test.
	cleanup, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	// Build all Go binaries used in intergartion test scripts and
	// then run the integration tests. We pre-build the binaries
	// used by multiple test scripts to speed things up.
	if ctx.DryRun() {
		binPackages = nil
	}
	result, err := buildBinaries(ctx, testName)
	if err != nil {
		return nil, err
	}
	if result.Status == TestFailed {
		return result, nil
	}
	result, err = runTestScripts(ctx, testName)
	if err != nil {
		return nil, err
	}
	return result, nil
}
