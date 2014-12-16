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

	"veyron.io/tools/lib/collect"
	"veyron.io/tools/lib/envutil"
	"veyron.io/tools/lib/util"
)

const (
	defaultIntegrationTestTimeout = 2 * time.Minute
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
func (t *testEnv) buildBinaries(ctx *util.Context, testName string) (*TestResult, error) {
	// Create a pool of workers.
	fmt.Fprintf(ctx.Stdout(), "building binaries...\n")
	numPkgs := len(binPackages)
	tasks := make(chan string, numPkgs)
	taskResults := make(chan buildResult, numPkgs)
	for i := 0; i < runtime.NumCPU(); i++ {
		go t.buildWorker(nil, tasks, taskResults)
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
			Fail(ctx, "%s\n%v\n", result.pkg, result.output)
			f := testFailure{
				Message: "build",
				Data:    result.output,
			}
			c.Failures = append(c.Failures, f)
			allPassed = false
			s.Failures++
		} else {
			Pass(ctx, "%s\n", result.pkg)
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

// findIntegrationTests finds all test.sh or testdata/integration_test.go files
// from the given root dirs.
//
// TODO(sjr,jsimsa): Replace with go-based integration tests when available.
func findIntegrationTests(ctx *util.Context, rootDirs []string) []string {
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

// runIntegrationTests runs all integration tests found under
// $VEYRON_ROOT/roadmap/go/src and $VEYRON_ROOT/veyron/go/src.
func (t *testEnv) runIntegrationTests(ctx *util.Context, testName string) (*TestResult, error) {
	root, err := util.VeyronRoot()
	if err != nil {
		return nil, err
	}

	// Find all integration tests.
	testScripts := findIntegrationTests(ctx, []string{
		filepath.Join(root, "veyron", "go", "src"),
		filepath.Join(root, "roadmap", "go", "src"),
		filepath.Join(root, "scripts"),
	})

	// Create a worker pool to run tests in parallel, passing the
	// location of binaries through shell_test_BIN_DIR.
	fmt.Fprintf(ctx.Stdout(), "running tests...\n")
	numTests := len(testScripts)
	tasks := make(chan string, numTests)
	taskResults := make(chan testResult, numTests)
	env := envutil.NewSnapshotFromOS()
	env.Set("shell_test_BIN_DIR", binDirPath())
	env.Set("VEYRON_INTEGRATION_BIN_DIR", binDirPath())
	// TODO(rthellend): When we run these tests in parallel, some of them
	// appear to hang after completing successfully. For now, only run one
	// at a time to confirm.
	//for i := 0; i < runtime.NumCPU(); i++ {
	go t.integrationTestWorker(root, env.Map(), tasks, taskResults)
	//}

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
			Name:      "IntegrationTest",
			Time:      fmt.Sprintf("%.2f", result.time.Seconds()),
		}
		switch result.status {
		case testFailed:
			Fail(ctx, "%s\n%v\n", result.pkg, result.output)
			f := testFailure{
				Message: "test",
				Data:    result.output,
			}
			c.Failures = append(c.Failures, f)
			allPassed = false
			s.Failures++
		case testPassed:
			Pass(ctx, "%s\n", result.pkg)
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

// integrationTestWorker receives tasks from the <tasks> channel, runs
// them, and sends results to the <results> channel.
func (t *testEnv) integrationTestWorker(root string, env map[string]string, tasks <-chan string, results chan<- testResult) {
	var out bytes.Buffer
	env["PATH"] = t.snapshot.Get("PATH")
	ctx := util.NewContext(env, os.Stdin, &out, &out, false, false, false)
	for script := range tasks {
		start := time.Now()
		var args []string
		pkgName := strings.TrimPrefix(path.Dir(script), root)
		if index := strings.Index(pkgName, "veyron.io"); index != -1 {
			pkgName = pkgName[index:]
		}
		result := testResult{}
		switch {
		case strings.HasSuffix(script, ".go"):
			result.pkg = "go." + pkgName
			args = []string{"go", "test", script}
		case strings.HasSuffix(script, ".sh"):
			result.pkg = "shell." + pkgName
			args = []string{"run", "bash", "-x", script}
		default:
			fmt.Fprintf(os.Stderr, "unsupported type of integration test: %v\n", script)
			continue
		}
		err := ctx.Run().TimedCommand(defaultIntegrationTestTimeout, t.veyronBin, args...)
		result.time = time.Now().Sub(start)
		result.output = out.String()
		if err != nil {
			result.status = testFailed
		} else {
			result.status = testPassed
		}
		results <- result
		out.Reset()
	}
}

// veyronIntegrationTest runs veyron integration tests.
func (t *testEnv) veyronIntegrationTest(ctx *util.Context, testName string) (_ *TestResult, e error) {
	// Initialize the test.
	cleanup, err := t.initTest(ctx, testName, []string{"web"})
	if err != nil {
		return nil, err
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Build all Go binaries used in intergartion test scripts and
	// then run the integration tests. We pre-build the binaries
	// used by multiple test scripts to speed things up.
	if ctx.DryRun() {
		binPackages = nil
	}
	result, err := t.buildBinaries(ctx, testName)
	if err != nil {
		return nil, err
	}
	if result.Status == TestFailed {
		return result, nil
	}
	result, err = t.runIntegrationTests(ctx, testName)
	if err != nil {
		return nil, err
	}
	return result, nil
}
