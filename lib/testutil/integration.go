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

	"v.io/tools/lib/collect"
	"v.io/tools/lib/envutil"
	"v.io/tools/lib/runutil"
	"v.io/tools/lib/util"
)

const (
	defaultIntegrationTestTimeout = 10 * time.Minute
)

// binPackages enumerates the Go commands used by vanadium integration tests.
//
// TODO(jingjin): port the integration test scripts from shell to Go
// and make them use a build cache to share binaries.
var binPackages = []string{
	"v.io/apps/tunnel/tunneld",
	"v.io/apps/tunnel/vsh",
	"v.io/core/veyron/security/agent/agentd",
	"v.io/core/veyron/security/agent/pingpong",
	"v.io/core/veyron/services/mgmt/application/applicationd",
	"v.io/core/veyron/services/mgmt/binary/binaryd",
	"v.io/core/veyron/services/mgmt/build/buildd",
	"v.io/core/veyron/services/mgmt/profile/profiled",
	"v.io/core/veyron/services/mounttable/mounttabled",
	"v.io/core/veyron/services/proxy/proxyd",
	"v.io/core/veyron/tools/application",
	"v.io/core/veyron/tools/binary",
	"v.io/core/veyron/tools/build",
	"v.io/core/veyron/tools/debug",
	"v.io/core/veyron/tools/mounttable",
	"v.io/core/veyron/tools/principal",
	"v.io/core/veyron/tools/profile",
	"v.io/core/veyron/tools/naming/simulator",
	"v.io/core/veyron2/vdl/vdl",
	"v.io/wspr/veyron/services/wsprd",
}

type goIntegrationOpt interface {
	goIntegrationOpt()
}

func (suffixOpt) goIntegrationOpt() {}

// buildBinaries builds Go binaries enumerated by the binPackages list.
func buildBinaries(ctx *util.Context, testName string, opts ...goIntegrationOpt) (*TestResult, error) {
	suffix := ""
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case suffixOpt:
			suffix = string(typedOpt)
		}
	}

	// Create a pool of workers.
	fmt.Fprintf(ctx.Stdout(), "building binaries...\n")
	numPkgs := len(binPackages)
	tasks := make(chan string, numPkgs)
	taskResults := make(chan buildResult, numPkgs)
	for i := 0; i < runtime.NumCPU(); i++ {
		go buildWorker(ctx, nil, tasks, taskResults)
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
		testCaseName := "Build"
		if suffix != "" {
			testCaseName += (" " + suffix)
		}
		c := testCase{
			Classname: result.pkg,
			Name:      testCaseName,
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

func findIntegrationTests(ctx *util.Context, rootDirs []string, testNamePredicate func(string) bool) []string {
	if ctx.DryRun() {
		// In "dry run" mode, no test scripts are executed.
		return nil
	}
	matchedFiles := []string{}
	for _, rootDir := range rootDirs {
		filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
			if testNamePredicate(path) {
				matchedFiles = append(matchedFiles, path)
			}
			return nil
		})
	}
	return matchedFiles
}

// runIntegrationTests runs all integration tests found under
// $VANADIUM_ROOT/roadmap/go/src and $VANADIUM_ROOT/release/go/src.
func runIntegrationTests(ctx *util.Context, testName string, testNamePredicate func(string) bool, opts ...goIntegrationOpt) (*TestResult, error) {
	suffix := ""
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case suffixOpt:
			suffix = string(typedOpt)
		}
	}

	root, err := util.VanadiumRoot()
	if err != nil {
		return nil, err
	}

	// Find all integration tests.
	testScripts := findIntegrationTests(ctx, []string{
		filepath.Join(root, "release", "go", "src"),
		filepath.Join(root, "roadmap", "go", "src"),
		filepath.Join(root, "scripts"),
	}, testNamePredicate)

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
	go integrationTestWorker(ctx, root, env.Map(), tasks, taskResults)
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
		testCaseName := "IntegrationTest"
		if suffix != "" {
			testCaseName += (" " + suffix)
		}
		c := testCase{
			Classname: result.pkg,
			Name:      testCaseName,
			Time:      fmt.Sprintf("%.2f", result.time.Seconds()),
		}
		switch result.status {
		case testFailed, testTimedout:
			Fail(ctx, "%s\n%v\n", result.pkg, result.output)
			f := testFailure{
				Message: "test",
				Data:    result.output,
			}
			if result.status == testTimedout {
				f.Message = fmt.Sprintf("timed out after %s", defaultIntegrationTestTimeout)
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
func integrationTestWorker(ctx *util.Context, root string, env map[string]string, tasks <-chan string, results chan<- testResult) {
	opts := ctx.Run().Opts()
	opts.Verbose = false
	for script := range tasks {
		var out bytes.Buffer
		start := time.Now()
		var args []string
		pkgName := strings.TrimPrefix(path.Dir(script), root)
		if index := strings.Index(pkgName, "v.io"); index != -1 {
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
		opts.Stdout = &out
		opts.Stderr = &out
		err := ctx.Run().TimedCommandWithOpts(defaultIntegrationTestTimeout, opts, "v23", args...)
		result.time = time.Now().Sub(start)
		result.output = out.String()
		if err != nil {
			if err == runutil.CommandTimedOutErr {
				result.status = testTimedout
			} else {
				result.status = testFailed
			}
		} else {
			result.status = testPassed
		}
		results <- result
	}
}

// vanadiumIntegrationTestOld runs vanadium integration tests.
func vanadiumIntegrationTestOld(ctx *util.Context, testName string, _ ...TestOpt) (_ *TestResult, e error) {
	// Initialize the test.
	cleanup, err := initTest(ctx, testName, []string{"web"})
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Build all Go binaries used in intergartion test scripts and
	// then run the integration tests. We pre-build the binaries
	// used by multiple test scripts to speed things up.
	if ctx.DryRun() {
		binPackages = nil
	}
	suffix := suffixOpt(genTestNameSuffix(""))
	result, err := buildBinaries(ctx, testName, suffix)
	if err != nil {
		return nil, err
	}
	if result.Status == TestFailed {
		return result, nil
	}
	result, err = runIntegrationTests(ctx, testName, func(path string) bool {
		return strings.HasSuffix(path, string(os.PathSeparator)+"test.sh")
	}, suffix)
	if err != nil {
		return nil, err
	}
	return result, nil
}
