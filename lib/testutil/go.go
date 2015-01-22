package testutil

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"v.io/tools/lib/collect"
	"v.io/tools/lib/envutil"
	"v.io/tools/lib/util"
)

type taskStatus int

const (
	buildPassed taskStatus = iota
	buildFailed
	testPassed
	testFailed
)

type buildResult struct {
	pkg    string
	status taskStatus
	output string
	time   time.Duration
}

type goBuildOpt interface {
	goBuildOpt()
}

type goCoverageOpt interface {
	goCoverageOpt()
}

type goTestOpt interface {
	goTestOpt()
}

type argsOpt []string
type profilesOpt []string
type timeoutOpt string
type suffixOpt string
type excludedTestsOpt []test

func (argsOpt) goBuildOpt()    {}
func (argsOpt) goCoverageOpt() {}
func (argsOpt) goTestOpt()     {}

func (profilesOpt) goBuildOpt()    {}
func (profilesOpt) goCoverageOpt() {}
func (profilesOpt) goTestOpt()     {}

func (timeoutOpt) goCoverageOpt() {}
func (timeoutOpt) goTestOpt()     {}

func (suffixOpt) goTestOpt() {}

func (excludedTestsOpt) goTestOpt() {}

// goBuild is a helper function for running Go builds.
func goBuild(ctx *util.Context, testName string, pkgs []string, opts ...goBuildOpt) (_ *TestResult, e error) {
	args, profiles := []string{}, []string{}
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case argsOpt:
			args = []string(typedOpt)
		case profilesOpt:
			profiles = []string(typedOpt)
		}
	}

	// Initialize the test.
	cleanup, result, err := initTest(ctx, testName, profiles)
	if err != nil {
		return nil, err
	} else if result != nil {
		return result, nil
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Enumerate the packages to be built.
	pkgList, err := goList(ctx, pkgs)
	if err != nil {
		return nil, err
	}

	// Create a pool of workers.
	numPkgs := len(pkgList)
	tasks := make(chan string, numPkgs)
	taskResults := make(chan buildResult, numPkgs)
	for i := 0; i < runtime.NumCPU(); i++ {
		go buildWorker(ctx, args, tasks, taskResults)
	}

	// Distribute work to workers.
	for _, pkg := range pkgList {
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

// buildWorker builds packages.
func buildWorker(ctx *util.Context, args []string, pkgs <-chan string, results chan<- buildResult) {
	opts := ctx.Run().Opts()
	opts.Verbose = false
	for pkg := range pkgs {
		var out bytes.Buffer
		args := append([]string{"go", "build", "-o", filepath.Join(binDirPath(), path.Base(pkg))}, args...)
		args = append(args, pkg)
		opts.Stdout = &out
		opts.Stderr = &out
		start := time.Now()
		err := ctx.Run().CommandWithOpts(opts, "v23", args...)
		duration := time.Now().Sub(start)
		result := buildResult{
			pkg:    pkg,
			time:   duration,
			output: out.String(),
		}
		if err != nil {
			result.status = buildFailed
		} else {
			result.status = buildPassed
		}
		results <- result
	}
}

type coverageResult struct {
	pkg      string
	coverage *os.File
	output   string
	status   taskStatus
	time     time.Duration
}

const defaultTestCoverageTimeout = "5m"

// goCoverage is a helper function for running Go coverage tests.
func goCoverage(ctx *util.Context, testName string, pkgs []string, opts ...goCoverageOpt) (_ *TestResult, e error) {
	timeout := defaultTestCoverageTimeout
	args, profiles := []string{}, []string{}
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case timeoutOpt:
			timeout = string(typedOpt)
		case argsOpt:
			args = []string(typedOpt)
		case profilesOpt:
			profiles = []string(typedOpt)
		}
	}

	// Initialize the test.
	cleanup, result, err := initTest(ctx, testName, profiles)
	if err != nil {
		return nil, err
	} else if result != nil {
		return result, nil
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Install dependencies.
	if err := installGoCover(ctx); err != nil {
		return nil, err
	}
	if err := installGoCoverCobertura(ctx); err != nil {
		return nil, err
	}
	if err := installGo2XUnit(ctx); err != nil {
		return nil, err
	}

	// Pre-build non-test packages.
	if err := buildTestDeps(ctx, pkgs); err != nil {
		s := createTestSuiteWithFailure("BuildTestDependencies", "TestCoverage", "dependencies build failure", err.Error(), 0)
		if err := createXUnitReport(ctx, testName, []testSuite{*s}); err != nil {
			return nil, err
		}
		return &TestResult{Status: TestFailed}, nil
	}

	// Enumerate the packages for which coverage is to be computed.
	pkgList, err := goList(ctx, pkgs)
	if err != nil {
		return nil, err
	}

	// Create a pool of workers.
	numPkgs := len(pkgList)
	tasks := make(chan string, numPkgs)
	taskResults := make(chan coverageResult, numPkgs)
	for i := 0; i < runtime.NumCPU(); i++ {
		go coverageWorker(ctx, timeout, args, tasks, taskResults)
	}

	// Distribute work to workers.
	for _, pkg := range pkgList {
		tasks <- pkg
	}
	close(tasks)

	// Collect the results.
	//
	// TODO(jsimsa): Gather coverage data using the testCoverage
	// data structure as opposed to a buffer.
	var coverageData bytes.Buffer
	fmt.Fprintf(&coverageData, "mode: set\n")
	allPassed, suites := true, []testSuite{}
	for i := 0; i < numPkgs; i++ {
		result := <-taskResults
		var s *testSuite
		switch result.status {
		case buildFailed:
			s = createTestSuiteWithFailure(result.pkg, "TestCoverage", "build failure", result.output, result.time)
		case testPassed:
			data, err := ioutil.ReadAll(result.coverage)
			if err != nil {
				return nil, err
			}
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if line != "" && strings.Index(line, "mode: set") == -1 {
					fmt.Fprintf(&coverageData, "%s\n", line)
				}
			}
			fallthrough
		case testFailed:
			if strings.Index(result.output, "no test files") == -1 {
				ss, err := testSuiteFromGoTestOutput(ctx, bytes.NewBufferString(result.output))
				if err != nil {
					// Token too long error.
					if !strings.HasSuffix(err.Error(), "token too long") {
						return nil, err
					}
					ss = createTestSuiteWithFailure(result.pkg, "Test", "test output contains lines that are too long to parse", "", result.time)
				}
				s = ss
			}
		}
		if result.coverage != nil {
			result.coverage.Close()
			if err := ctx.Run().RemoveAll(result.coverage.Name()); err != nil {
				return nil, err
			}
		}
		if s != nil {
			if s.Failures > 0 {
				allPassed = false
				Fail(ctx, "%s\n%v\n", result.pkg, result.output)
			} else {
				Pass(ctx, "%s\n", result.pkg)
			}
			suites = append(suites, *s)
		}
	}
	close(taskResults)

	// Create the xUnit and cobertura reports.
	if err := createXUnitReport(ctx, testName, suites); err != nil {
		return nil, err
	}
	coverage, err := coverageFromGoTestOutput(ctx, &coverageData)
	if err != nil {
		return nil, err
	}
	if err := createCoberturaReport(ctx, testName, coverage); err != nil {
		return nil, err
	}
	if !allPassed {
		return &TestResult{Status: TestFailed}, nil
	}
	return &TestResult{Status: TestPassed}, nil
}

// coverageWorker generates test coverage.
func coverageWorker(ctx *util.Context, timeout string, args []string, pkgs <-chan string, results chan<- coverageResult) {
	opts := ctx.Run().Opts()
	opts.Verbose = false
	for pkg := range pkgs {
		// Compute the test coverage.
		var out bytes.Buffer
		coverageFile, err := ioutil.TempFile("", "")
		if err != nil {
			panic(fmt.Sprintf("TempFile() failed: %v", err))
		}
		args := append([]string{
			"go", "test", "-cover", "-coverprofile",
			coverageFile.Name(), "-timeout", timeout, "-v",
		}, args...)
		args = append(args, pkg)
		opts.Stdout = &out
		opts.Stderr = &out
		start := time.Now()
		err = ctx.Run().CommandWithOpts(opts, "v23", args...)
		result := coverageResult{
			pkg:      pkg,
			coverage: coverageFile,
			time:     time.Now().Sub(start),
			output:   out.String(),
		}
		if err != nil {
			if isBuildFailure(err, out.String(), pkg) {
				result.status = buildFailed
			} else {
				result.status = testFailed
			}
		} else {
			result.status = testPassed
		}
		results <- result
	}
}

// goList is a helper function for listing Go packages.
func goList(ctx *util.Context, pkgs []string) ([]string, error) {
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	args := append([]string{"go", "list"}, pkgs...)
	if err := ctx.Run().CommandWithOpts(opts, "v23", args...); err != nil {
		fmt.Fprintln(ctx.Stdout(), out.String())
		return nil, err
	}
	cleanOut := strings.TrimSpace(out.String())
	if cleanOut == "" {
		return nil, nil
	}
	return strings.Split(cleanOut, "\n"), nil
}

type testResult struct {
	pkg    string
	output string
	status taskStatus
	time   time.Duration
}

const defaultTestTimeout = "5m"

type goTestTask struct {
	pkg           string
	excludedTests []string
}

// goTest is a helper function for running Go tests.
func goTest(ctx *util.Context, testName string, pkgs []string, opts ...goTestOpt) (_ *TestResult, e error) {
	timeout := defaultTestTimeout
	args, profiles, suffix, excludedTests := []string{}, []string{}, "", []test{}
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case timeoutOpt:
			timeout = string(typedOpt)
		case argsOpt:
			args = []string(typedOpt)
		case profilesOpt:
			profiles = []string(typedOpt)
		case suffixOpt:
			suffix = string(typedOpt)
		case excludedTestsOpt:
			excludedTests = []test(typedOpt)
		}
	}

	// Initialize the test.
	cleanup, result, err := initTest(ctx, testName, profiles)
	if err != nil {
		return nil, err
	} else if result != nil {
		return result, nil
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Install dependencies.
	if err := installGo2XUnit(ctx); err != nil {
		return nil, err
	}

	// Pre-build non-test packages.
	if err := buildTestDeps(ctx, pkgs); err != nil {
		if len(suffix) != 0 {
			testName += " " + suffix
		}
		s := createTestSuiteWithFailure("BuildTestDependencies", testName, "dependencies build failure", err.Error(), 0)
		if err := createXUnitReport(ctx, testName, []testSuite{*s}); err != nil {
			return nil, err
		}
		return &TestResult{Status: TestFailed}, nil
	}

	// Enumerate the packages to be built.
	pkgList, err := goList(ctx, pkgs)
	if err != nil {
		return nil, err
	}

	// Create a pool of workers.
	numPkgs := len(pkgList)
	tasks := make(chan goTestTask, numPkgs)
	taskResults := make(chan testResult, numPkgs)
	for i := 0; i < runtime.NumCPU(); i++ {
		go testWorker(ctx, timeout, args, tasks, taskResults)
	}

	// Distribute work to workers.
	for _, pkg := range pkgList {
		// Identify the names of tests to exclude.
		testNames := []string{}
		for _, test := range excludedTests {
			if test.pkg == pkg {
				testNames = append(testNames, test.name)
			}
		}
		tasks <- goTestTask{pkg, testNames}
	}
	close(tasks)

	// Collect the results.
	allPassed, suites := true, []testSuite{}
	for i := 0; i < numPkgs; i++ {
		result := <-taskResults
		var s *testSuite
		switch result.status {
		case buildFailed:
			s = createTestSuiteWithFailure(result.pkg, "Test", "build failure", result.output, result.time)
		case testFailed, testPassed:
			if strings.Index(result.output, "no test files") == -1 {
				ss, err := testSuiteFromGoTestOutput(ctx, bytes.NewBufferString(result.output))
				if err != nil {
					// Token too long error.
					if !strings.HasSuffix(err.Error(), "token too long") {
						return nil, err
					}
					ss = createTestSuiteWithFailure(result.pkg, "Test", "test output contains lines that are too long to parse", "", result.time)
				}
				s = ss
			}
		}
		if s != nil {
			if s.Failures > 0 {
				allPassed = false
				Fail(ctx, "%s\n%v\n", result.pkg, result.output)
			} else {
				Pass(ctx, "%s\n", result.pkg)
			}
			newCases := []testCase{}
			for _, c := range s.Cases {
				if len(suffix) != 0 {
					c.Name += " " + suffix
				}
				newCases = append(newCases, c)
			}
			s.Cases = newCases
			suites = append(suites, *s)
		}
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

// testWorker tests packages.
func testWorker(ctx *util.Context, timeout string, args []string, tasks <-chan goTestTask, results chan<- testResult) {
	opts := ctx.Run().Opts()
	opts.Verbose = false
	for task := range tasks {
		// Run the test.
		var out bytes.Buffer
		args := append([]string{"go", "test", "-timeout", timeout, "-v"}, args...)
		if len(task.excludedTests) != 0 {
			// Create the regular expression describing
			// the complement of the set of the tests to
			// be excluded.
			for i := 0; i < len(task.excludedTests); i++ {
				task.excludedTests[i] = fmt.Sprintf("^(%s)", task.excludedTests[i])
			}
			args = append(args, "-run", fmt.Sprintf("[%s]", strings.Join(task.excludedTests, "|")))
		}
		args = append(args, task.pkg)
		opts.Stdout = &out
		opts.Stderr = &out
		start := time.Now()
		err := ctx.Run().CommandWithOpts(opts, "v23", args...)
		result := testResult{
			pkg:    task.pkg,
			time:   time.Now().Sub(start),
			output: out.String(),
		}
		if err != nil {
			if isBuildFailure(err, out.String(), task.pkg) {
				result.status = buildFailed
			} else {
				result.status = testFailed
			}
		} else {
			result.status = testPassed
		}
		results <- result
	}
}

// buildTestDeps builds dependencies for the given test packages
func buildTestDeps(ctx *util.Context, pkgs []string) error {
	fmt.Fprintf(ctx.Stdout(), "building test dependencies ... ")
	args := append([]string{"go", "test", "-i"}, pkgs...)
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stderr = &out
	err := ctx.Run().CommandWithOpts(opts, "v23", args...)
	if err == nil {
		fmt.Fprintf(ctx.Stdout(), "ok\n")
		return nil
	}
	fmt.Fprintf(ctx.Stdout(), "failed\n%s\n", out.String())
	return fmt.Errorf("%v\n%s", err, out.String())
}

func createTestSuiteWithFailure(pkgName, testName, failureMessage, failureOutput string, duration time.Duration) *testSuite {
	s := testSuite{Name: pkgName}
	c := testCase{
		Classname: pkgName,
		Name:      testName,
		Time:      fmt.Sprintf("%.2f", duration.Seconds()),
	}
	s.Tests = 1
	f := testFailure{
		Message: failureMessage,
		Data:    failureOutput,
	}
	c.Failures = append(c.Failures, f)
	s.Failures = 1
	s.Cases = append(s.Cases, c)
	return &s
}

// installGoCover makes sure the "go cover" tool is installed.
//
// TODO(jsimsa): Unify the installation functions by moving the
// gocover-cobertura and go2xunit tools into the third_party
// repository.
func installGoCover(ctx *util.Context) error {
	// Check if the tool exists.
	var out bytes.Buffer
	cmd := exec.Command("go", "tool")
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(&out)
	for scanner.Scan() {
		if scanner.Text() == "cover" {
			return nil
		}
	}
	if scanner.Err() != nil {
		return fmt.Errorf("Scan() failed: %v")
	}
	if err := ctx.Run().Command("v23", "go", "install", "golang.org/x/tools/cmd/cover"); err != nil {
		return err
	}
	return nil
}

// installGoDoc makes sure the "go doc" tool is installed.
func installGoDoc(ctx *util.Context) error {
	// Check if the tool exists.
	if _, err := exec.LookPath("godoc"); err != nil {
		if err := ctx.Run().Command("v23", "go", "install", "golang.org/x/tools/cmd/godoc"); err != nil {
			return err
		}
	}
	return nil
}

// installGoCoverCobertura makes sure the "gocover-cobertura" tool is
// installed.
func installGoCoverCobertura(ctx *util.Context) error {
	root, err := util.VanadiumRoot()
	if err != nil {
		return err
	}
	// Check if the tool exists.
	bin := filepath.Join(root, "environment", "golib", "bin", "gocover-cobertura")
	if _, err := os.Stat(bin); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		opts := ctx.Run().Opts()
		opts.Env["GOPATH"] = filepath.Join(root, "environment", "golib")
		if err := ctx.Run().CommandWithOpts(opts, "v23", "go", "install", "github.com/t-yuki/gocover-cobertura"); err != nil {
			return err
		}
	}
	return nil
}

// installGo2XUnit makes sure the "go2xunit" tool is installed.
func installGo2XUnit(ctx *util.Context) error {
	root, err := util.VanadiumRoot()
	if err != nil {
		return err
	}
	// Check if the tool exists.
	bin := filepath.Join(root, "environment", "golib", "bin", "go2xunit")
	if _, err := os.Stat(bin); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		opts := ctx.Run().Opts()
		opts.Env["GOPATH"] = filepath.Join(root, "environment", "golib")
		if err := ctx.Run().CommandWithOpts(opts, "v23", "go", "install", "bitbucket.org/tebeka/go2xunit"); err != nil {
			return err
		}
	}
	return nil
}

// isBuildFailure checks whether the given error and output indicate a build failure for the given package.
func isBuildFailure(err error, out, pkg string) bool {
	if exitError, ok := err.(*exec.ExitError); ok {
		// Try checking err's process state to determine the exit code.
		// Exit code 2 means build failures.
		if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
			exitCode := status.ExitStatus()
			// A exit code of 2 means build failure.
			if exitCode == 2 {
				return true
			}
			// When the exit code is 1, we need to check the output to distinguish
			// "setup failure" and "test failure".
			if exitCode == 1 {
				// Treat setup failure as build failure.
				if strings.HasPrefix(out, fmt.Sprintf("# %s", pkg)) &&
					strings.HasSuffix(out, "[setup failed]\n") {
					return true
				}
				return false
			}
		}
	}
	// As a fallback, check the output line.
	// If the output starts with "# ${pkg}", then it should be a build failure.
	return strings.HasPrefix(out, fmt.Sprintf("# %s", pkg))
}

// getListenerPID finds the process ID of the process listening on the
// given port. If no process is listening on the given port (or an
// error is encountered), the function returns -1.
func getListenerPID(ctx *util.Context, port string) (int, error) {
	// Make sure "lsof" exists.
	_, err := exec.LookPath("lsof")
	if err != nil {
		return -1, fmt.Errorf(`"lsof" not found in the PATH`)
	}

	// Use "lsof" to find the process ID of the listener.
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	if err := ctx.Run().CommandWithOpts(opts, "lsof", "-i", ":"+port, "-sTCP:LISTEN", "-F", "p"); err != nil {
		// When no listener exists, "lsof" exits with non-zero
		// status.
		return -1, nil
	}

	// Parse the port number.
	pidString := strings.TrimPrefix(strings.TrimSpace(out.String()), "p")
	pid, err := strconv.Atoi(pidString)
	if err != nil {
		return -1, fmt.Errorf("Atoi(%v) failed: %v", pidString, err)
	}

	return pid, nil
}

var thirdPartyPkgs = []string{
	"code.google.com/...",
	"github.com/...",
	"golang.org/...",
	"google.golang.org/...",
}

type test struct {
	pkg  string
	name string
}

type exclusion struct {
	desc    test
	exclude func() bool
}

func isGCE() bool {
	sysuser := os.Getenv("USER")
	return sysuser == "veyron" && runtime.GOOS == "linux"
}

func isDarwin() bool {
	return runtime.GOOS == "darwin"
}

func isNotYosemite() bool {
	if runtime.GOOS != "darwin" {
		return true
	}
	out, err := exec.Command("uname", "-a").Output()
	if err != nil {
		return false
	}
	return !strings.Contains(string(out), "Version 14.")
}

var thirdPartyExclusions = []exclusion{
	// The following test requires an X server, which is not
	// available on GCE.
	exclusion{test{"golang.org/x/mobile/gl/glutil", "TestImage"}, isGCE},
	// The following test requires IPv6, which is not available on
	// GCE.
	exclusion{test{"golang.org/x/net/icmp", "TestPingGoogle"}, isGCE},
	// The following test expects to see "FAIL: TestBar" which
	// causes go2xunit to fail.
	exclusion{test{"golang.org/x/tools/go/ssa/interp", "TestTestmainPackage"}, isGCE},
	// Don't run this test on darwin since it's too awkward to set up
	// dbus at the system level.
	exclusion{test{"github.com/guelfey/go.dbus", "TestSystemBus"}, isDarwin},

	// Don't run this test on mac systems prior to Yosemite since it can
	// crash some machines.
	exclusion{test{"golang.org/x/net", "Test"}, isNotYosemite},

	// Fsnotify tests are flaky on darwin. This begs the question of whether
	// we should be relying on this library at all.
	exclusion{test{"github.com/howeyc/fsnotify", "Test"}, isDarwin},
}

// ExcludedThirdPartyTests returns the set of tests to be excluded from
// the third_party project.
func ExcludedThirdPartyTests() []test {
	return excludedTests(thirdPartyExclusions)
}

func excludedTests(exclusions []exclusion) []test {
	excluded := make([]test, 0, len(exclusions))
	for _, e := range exclusions {
		if e.exclude() {
			excluded = append(excluded, e.desc)
		}
	}
	return excluded
}

// thirdPartyGoBuild runs Go build for third-party projects.
func thirdPartyGoBuild(ctx *util.Context, testName string) (*TestResult, error) {
	return goBuild(ctx, testName, thirdPartyPkgs)
}

// thirdPartyGoTest runs Go tests for the third-party projects.
func thirdPartyGoTest(ctx *util.Context, testName string) (*TestResult, error) {
	pkgs := thirdPartyPkgs
	return goTest(ctx, testName, pkgs, excludedTestsOpt(ExcludedThirdPartyTests()))
}

// thirdPartyGoRace runs Go data-race tests for third-party projects.
func thirdPartyGoRace(ctx *util.Context, testName string) (*TestResult, error) {
	pkgs := thirdPartyPkgs
	args := argsOpt([]string{"-race"})
	return goTest(ctx, testName, pkgs, args, excludedTestsOpt(ExcludedThirdPartyTests()))
}

// vanadiumGoBench runs Go benchmarks for vanadium projects.
func vanadiumGoBench(ctx *util.Context, testName string) (*TestResult, error) {
	pkgs := []string{"v.io/..."}
	args := argsOpt([]string{"-bench", ".", "-run", "XXX"})
	return goTest(ctx, testName, pkgs, args)
}

// vanadiumGoBuild runs Go build for the vanadium projects.
func vanadiumGoBuild(ctx *util.Context, testName string) (*TestResult, error) {
	pkgs := []string{"v.io/..."}
	return goBuild(ctx, testName, pkgs)
}

// vanadiumGoCoverage runs Go coverage tests for vanadium projects.
func vanadiumGoCoverage(ctx *util.Context, testName string) (*TestResult, error) {
	pkgs := []string{"v.io/..."}
	return goCoverage(ctx, testName, pkgs)
}

// vanadiumGoDoc (re)starts the godoc server for vanadium projects.
func vanadiumGoDoc(ctx *util.Context, testName string) (_ *TestResult, e error) {
	root, err := util.VanadiumRoot()
	if err != nil {
		return nil, err
	}

	// Initialize the test.
	cleanup, result, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, err
	} else if result != nil {
		return result, nil
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Install dependencies.
	if err := installGoDoc(ctx); err != nil {
		return nil, err
	}

	// Terminate previous instance of godoc if it is still running.
	godocPort := "8002"
	pid, err := getListenerPID(ctx, godocPort)
	if err != nil {
		return nil, err
	}
	if pid != -1 {
		p, err := os.FindProcess(pid)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(ctx.Stdout(), "kill %d\n", pid)
		if err := p.Kill(); err != nil {
			return nil, err
		}
	}

	// Start a new instance of godoc.
	//
	// Jenkins kills all background processes started by a shell
	// when the shell exits. To prevent Jenkins from doing that,
	// use nil as standard input, redirect output to a file, and
	// set the BUILD_ID environment variable to "dontKillMe".
	godocCmd := exec.Command("godoc", "-analysis=type", "-index", "-http=:"+godocPort)
	godocCmd.Stdin = nil
	fd, err := os.Create(filepath.Join(root, "godoc.out"))
	if err != nil {
		return nil, err
	}
	godocCmd.Stdout = fd
	godocCmd.Stderr = fd
	env := envutil.NewSnapshotFromOS()
	env.Set("BUILD_ID", "dontKillMe")
	env.Set("GOPATH", fmt.Sprintf("%v:%v", filepath.Join(root, "release", "go"), filepath.Join(root, "roadmap", "go")))
	godocCmd.Env = env.Slice()
	fmt.Fprintf(ctx.Stdout(), "%v %v\n", godocCmd.Env, strings.Join(godocCmd.Args, " "))
	if err := godocCmd.Start(); err != nil {
		return nil, err
	}

	return &TestResult{Status: TestPassed}, nil
}

// vanadiumGoTest runs Go tests for vanadium projects.
func vanadiumGoTest(ctx *util.Context, testName string) (*TestResult, error) {
	pkgs := []string{"v.io/..."}
	suffix := suffixOpt(genTestNameSuffix("GoTest"))
	return goTest(ctx, testName, pkgs, suffix)
}

// vanadiumGoRace runs Go data-race tests for vanadium projects.
func vanadiumGoRace(ctx *util.Context, testName string) (*TestResult, error) {
	pkgs := []string{"v.io/..."}
	args := argsOpt([]string{"-race"})
	timeout := timeoutOpt("15m")
	suffix := suffixOpt(genTestNameSuffix("GoRace"))
	return goTest(ctx, testName, pkgs, args, timeout, suffix)
}

func genTestNameSuffix(baseSuffix string) string {
	extraSuffix := runtime.GOOS
	switch extraSuffix {
	case "darwin":
		extraSuffix = "mac"
	case "windows":
		extraSuffix = "win"
	}
	if baseSuffix == "" {
		return fmt.Sprintf("[%s]", extraSuffix)
	}
	return fmt.Sprintf("[%s - %s]", baseSuffix, extraSuffix)
}
