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

	"veyron.io/tools/lib/collect"
	"veyron.io/tools/lib/envutil"
	"veyron.io/tools/lib/util"
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

func (argsOpt) goBuildOpt()    {}
func (argsOpt) goCoverageOpt() {}
func (argsOpt) goTestOpt()     {}

func (profilesOpt) goBuildOpt()    {}
func (profilesOpt) goCoverageOpt() {}
func (profilesOpt) goTestOpt()     {}

func (timeoutOpt) goCoverageOpt() {}
func (timeoutOpt) goTestOpt()     {}

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
	cleanup, err := initTest(ctx, testName, profiles)
	if err != nil {
		return nil, err
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
		go buildWorker(args, tasks, taskResults)
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
func buildWorker(args []string, pkgs <-chan string, results chan<- buildResult) {
	for pkg := range pkgs {
		var out bytes.Buffer
		args := append([]string{"go", "build", "-o", filepath.Join(binDirPath(), path.Base(pkg))}, args...)
		args = append(args, pkg)
		cmd := exec.Command("veyron", args...)
		cmd.Stdout = &out
		cmd.Stderr = &out
		start := time.Now()
		err := cmd.Run()
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
	cleanup, err := initTest(ctx, testName, profiles)
	if err != nil {
		return nil, err
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
		return nil, err
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
		go coverageWorker(timeout, args, tasks, taskResults)
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
func coverageWorker(timeout string, args []string, pkgs <-chan string, results chan<- coverageResult) {
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
		cmd := exec.Command("veyron", args...)
		cmd.Stdout = &out
		cmd.Stderr = &out
		start := time.Now()
		err = cmd.Run()
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
	if err := ctx.Run().CommandWithOpts(opts, "veyron", args...); err != nil {
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

// goTest is a helper function for running Go tests.
func goTest(ctx *util.Context, testName string, pkgs []string, opts ...goTestOpt) (_ *TestResult, e error) {
	timeout := defaultTestTimeout
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
	cleanup, err := initTest(ctx, testName, profiles)
	if err != nil {
		return nil, err
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Install dependencies.
	if err := installGo2XUnit(ctx); err != nil {
		return nil, err
	}

	// Pre-build non-test packages.
	if err := buildTestDeps(ctx, pkgs); err != nil {
		return nil, err
	}

	// Enumerate the packages to be built.
	pkgList, err := goList(ctx, pkgs)
	if err != nil {
		return nil, err
	}

	// Create a pool of workers.
	numPkgs := len(pkgList)
	tasks := make(chan string, numPkgs)
	taskResults := make(chan testResult, numPkgs)
	for i := 0; i < runtime.NumCPU(); i++ {
		go testWorker(timeout, args, tasks, taskResults)
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
func testWorker(timeout string, args []string, pkgs <-chan string, results chan<- testResult) {
	for pkg := range pkgs {
		// Run the test.
		var out bytes.Buffer
		args := append([]string{"go", "test", "-timeout", timeout, "-v"}, args...)
		args = append(args, pkg)
		cmd := exec.Command("veyron", args...)
		cmd.Stdout = &out
		cmd.Stderr = &out
		start := time.Now()
		err := cmd.Run()
		result := testResult{
			pkg:    pkg,
			time:   time.Now().Sub(start),
			output: out.String(),
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

// buildTestDeps builds dependencies for the given test packages
func buildTestDeps(ctx *util.Context, pkgs []string) error {
	fmt.Fprintf(ctx.Stdout(), "building test dependencies ... ")
	args := append([]string{"go", "test", "-i"}, pkgs...)
	err := ctx.Run().Command("veyron", args...)
	if err == nil {
		fmt.Fprintf(ctx.Stdout(), "ok\n")
	} else {
		fmt.Fprintf(ctx.Stdout(), "failed\n")
	}
	return err
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
	if err := ctx.Run().Command("veyron", "go", "install", "code.google.com/p/go.tools/cmd/cover"); err != nil {
		return err
	}
	return nil
}

// installGoDoc makes sure the "go doc" tool is installed.
func installGoDoc(ctx *util.Context) error {
	// Check if the tool exists.
	if _, err := exec.LookPath("godoc"); err != nil {
		if err := ctx.Run().Command("veyron", "go", "install", "code.google.com/p/go.tools/cmd/godoc"); err != nil {
			return err
		}
	}
	return nil
}

// installGoCoverCobertura makes sure the "gocover-cobertura" tool is
// installed.
func installGoCoverCobertura(ctx *util.Context) error {
	root, err := util.VeyronRoot()
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
		env := envutil.NewSnapshotFromOS()
		env.Set("GOPATH", filepath.Join(root, "environment", "golib"))
		opts.Env = env.Map()
		if err := ctx.Run().CommandWithOpts(opts, "veyron", "go", "install", "github.com/t-yuki/gocover-cobertura"); err != nil {
			return err
		}
	}
	return nil
}

// installGo2XUnit makes sure the "go2xunit" tool is installed.
func installGo2XUnit(ctx *util.Context) error {
	root, err := util.VeyronRoot()
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
		env := envutil.NewSnapshotFromOS()
		env.Set("GOPATH", filepath.Join(root, "environment", "golib"))
		opts.Env = env.Map()
		if err := ctx.Run().CommandWithOpts(opts, "veyron", "go", "install", "bitbucket.org/tebeka/go2xunit"); err != nil {
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
			return status.ExitStatus() == 2
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

// ThirdPartyGoBuild runs Go build for third-party projects.
func ThirdPartyGoBuild(ctx *util.Context, testName string) (*TestResult, error) {
	pkgs := []string{"code.google.com/...", "github.com/..."}
	return goBuild(ctx, testName, pkgs)
}

// ThirdPartyGoTest runs Go tests for the third-party projects.
func ThirdPartyGoTest(ctx *util.Context, testName string) (*TestResult, error) {
	// Run the tests excluding TestTestmainPackage from
	// code.google.com/p/go.tools/go/ssa/interp as the package has
	// a test that expects to see FAIL: TestBar which causes
	// go2xunit to fail.
	pkgs := []string{"code.google.com/...", "github.com/..."}
	args := argsOpt([]string{"-run", "[^(TestTestmainPackage)]"})
	return goTest(ctx, testName, pkgs, args)
}

// ThirdPartyGoRace runs Go data-race tests for third-party projects.
func ThirdPartyGoRace(ctx *util.Context, testName string) (*TestResult, error) {
	// Run the tests excluding TestTestmainPackage from
	// code.google.com/p/go.tools/go/ssa/interp as the package has
	// a test that expects to see FAIL: TestBar which causes
	// go2xunit to fail.
	pkgs := []string{"code.google.com/...", "github.com/..."}
	args := argsOpt([]string{"-race", "-run", "[^(TestTestmainPackage)]"})
	return goTest(ctx, testName, pkgs, args)
}

// VeyronGoBench runs Go benchmarks for veyron projects.
func VeyronGoBench(ctx *util.Context, testName string) (*TestResult, error) {
	pkgs := []string{"veyron.io/..."}
	args := argsOpt([]string{"-tags", "veyronbluetooth", "-bench", ".", "-run", "XXX"})
	profiles := profilesOpt([]string{"proximity"})
	return goTest(ctx, testName, pkgs, args, profiles)
}

// VeyronGoBuild runs Go build for the veyron projects.
func VeyronGoBuild(ctx *util.Context, testName string) (*TestResult, error) {
	pkgs := []string{"veyron.io/..."}
	args := argsOpt([]string{"-tags", "veyronbluetooth"})
	profiles := profilesOpt([]string{"proximity"})
	return goBuild(ctx, testName, pkgs, args, profiles)
}

// VeyronGoCoverage runs Go coverage tests for veyron projects.
func VeyronGoCoverage(ctx *util.Context, testName string) (*TestResult, error) {
	pkgs := []string{"veyron.io/..."}
	profiles := profilesOpt([]string{"proximity"})
	return goCoverage(ctx, testName, pkgs, profiles)
}

// VeyronGoDoc (re)starts the godoc server for veyron projects.
func VeyronGoDoc(ctx *util.Context, testName string) (_ *TestResult, e error) {
	root, err := util.VeyronRoot()
	if err != nil {
		return nil, err
	}

	// Initialize the test.
	cleanup, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, err
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
	env.Set("GOPATH", fmt.Sprintf("%v:%v", filepath.Join(root, "veyron", "go"), filepath.Join(root, "roadmap", "go")))
	godocCmd.Env = env.Slice()
	fmt.Fprintf(ctx.Stdout(), "%v %v\n", godocCmd.Env, strings.Join(godocCmd.Args, " "))
	if err := godocCmd.Start(); err != nil {
		return nil, err
	}

	return &TestResult{Status: TestPassed}, nil
}

// VeyronGoTest runs Go tests for veyron projects.
func VeyronGoTest(ctx *util.Context, testName string) (*TestResult, error) {
	pkgs := []string{"veyron.io/..."}
	args := argsOpt([]string{"-tags", "veyronbluetooth"})
	profiles := profilesOpt([]string{"proximity"})
	return goTest(ctx, testName, pkgs, args, profiles)
}

// VeyronGoRace runs Go data-race tests for veyron projects.
func VeyronGoRace(ctx *util.Context, testName string) (*TestResult, error) {
	pkgs := []string{"veyron.io/..."}
	args := argsOpt([]string{"-race", "-tags", "veyronbluetooth"})
	profiles := profilesOpt([]string{"proximity"})
	timeout := timeoutOpt("7m")
	return goTest(ctx, testName, pkgs, timeout, args, profiles)
}
