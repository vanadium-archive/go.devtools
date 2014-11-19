package testutil

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"tools/lib/envutil"
	"tools/lib/util"
)

// initGoTest carries out the initial actions for the given Go test.
func initGoTest(ctx *util.Context, testName string, profiles []string) (func(), error) {
	cleanup, err := initTest(ctx, testName, profiles)
	if err != nil {
		return nil, err
	}
	if err := ctx.Run().Command("veyron", "goext", "distclean"); err != nil {
		return nil, err
	}
	return cleanup, nil
}

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

// goBuild is a helper function for running Go builds.
func goBuild(ctx *util.Context, testName string, args, pkgs, profiles []string) (*TestResult, error) {
	// Initialize the test.
	cleanup, err := initGoTest(ctx, testName, profiles)
	if err != nil {
		return nil, err
	}
	defer cleanup()

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
	close(taskResults)

	// Create the xUnit report.
	if err := createXUnitReport(testName, suites); err != nil {
		return nil, err
	}
	if !allPassed {
		return &TestResult{Status: TestFailed}, nil
	}
	return &TestResult{Status: TestPassed}, nil
}

// buildWorker builds packages.
func buildWorker(ctx *util.Context, args []string, pkgs <-chan string, results chan<- buildResult) {
	for pkg := range pkgs {
		var out bytes.Buffer
		args := append([]string{"go", "build"}, args...)
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
func goCoverage(ctx *util.Context, testName string, args, pkgs, profiles []string) (*TestResult, error) {
	// Initialize the test.
	cleanup, err := initGoTest(ctx, testName, profiles)
	if err != nil {
		return nil, err
	}
	defer cleanup()

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
		go coverageWorker(ctx, args, tasks, taskResults)
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
		s := testSuite{Name: result.pkg}
		c := testCase{
			Classname: result.pkg,
			Name:      "TestCoverage",
			Time:      fmt.Sprintf("%.2f", result.time.Seconds()),
		}
		addFailureFn := func(message string) {
			fmt.Fprintf(ctx.Stdout(), "%s ... failed\n%v\n", result.pkg, result.output)
			f := testFailure{
				Message: message,
				Data:    result.output,
			}
			c.Failures = append(c.Failures, f)
			allPassed = false
			s.Failures++
			s.Tests++
			s.Cases = append(s.Cases, c)
		}
		switch result.status {
		case buildFailed:
			addFailureFn("build")
		case testFailed:
			addFailureFn("test")
		case testPassed:
			fmt.Fprintf(ctx.Stdout(), "%s ... ok\n", result.pkg)
			if strings.Index(result.output, "no test files") == -1 {
				ss, err := testSuiteFromGoTestOutput(ctx, bytes.NewBufferString(result.output))
				if err != nil {
					return nil, err
				}
				s = *ss
			}
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
		}
		if result.coverage != nil {
			result.coverage.Close()
			os.Remove(result.coverage.Name())
		}
		suites = append(suites, s)
	}
	close(taskResults)

	// Create the xUnit and cobertura reports.
	if err := createXUnitReport(testName, suites); err != nil {
		return nil, err
	}
	coverage, err := coverageFromGoTestOutput(ctx, &coverageData)
	if err != nil {
		return nil, err
	}
	if err := createCoberturaReport(testName, coverage); err != nil {
		return nil, err
	}
	if !allPassed {
		return &TestResult{Status: TestFailed}, nil
	}
	return &TestResult{Status: TestPassed}, nil
}

// coverageWorker generates test coverage.
func coverageWorker(ctx *util.Context, args []string, pkgs <-chan string, results chan<- coverageResult) {
	for pkg := range pkgs {
		// Compute the test coverage.
		var out bytes.Buffer
		coverageFile, err := ioutil.TempFile("", "")
		if err != nil {
			panic(fmt.Sprintf("TempFile() failed: %v", err))
		}
		args := append([]string{
			"go", "test", "-cover", "-coverprofile",
			coverageFile.Name(), "-timeout", defaultTestCoverageTimeout, "-v",
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
	return strings.Split(strings.TrimSpace(out.String()), "\n"), nil
}

type testResult struct {
	pkg    string
	output string
	status taskStatus
	time   time.Duration
}

const defaultTestTimeout = "5m"

// goTest is a helper function for running Go tests.
func goTest(ctx *util.Context, testName string, args, pkgs, profiles []string) (*TestResult, error) {
	// Initialize the test.
	cleanup, err := initGoTest(ctx, testName, profiles)
	if err != nil {
		return nil, err
	}
	defer cleanup()

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
		go testWorker(ctx, args, tasks, taskResults)
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
			Name:      "Test",
			Time:      fmt.Sprintf("%.2f", result.time.Seconds()),
		}
		addFailureFn := func(message string) {
			fmt.Fprintf(ctx.Stdout(), "%s ... failed\n%v\n", result.pkg, result.output)
			f := testFailure{
				Message: message,
				Data:    result.output,
			}
			c.Failures = append(c.Failures, f)
			allPassed = false
			s.Failures++
			s.Tests++
			s.Cases = append(s.Cases, c)
		}
		switch result.status {
		case buildFailed:
			addFailureFn("build")
		case testFailed:
			addFailureFn("test")
		case testPassed:
			fmt.Fprintf(ctx.Stdout(), "%s ... ok\n", result.pkg)
			if strings.Index(result.output, "no test files") == -1 {
				ss, err := testSuiteFromGoTestOutput(ctx, bytes.NewBufferString(result.output))
				if err != nil {
					return nil, err
				}
				s = *ss
			}
		}
		suites = append(suites, s)
	}
	close(taskResults)

	// Create the xUnit report.
	if err := createXUnitReport(testName, suites); err != nil {
		return nil, err
	}
	if !allPassed {
		return &TestResult{Status: TestFailed}, nil
	}
	return &TestResult{Status: TestPassed}, nil
}

// testWorker tests packages.
func testWorker(ctx *util.Context, args []string, pkgs <-chan string, results chan<- testResult) {
	for pkg := range pkgs {
		// Run the test.
		var out bytes.Buffer
		args := append([]string{"go", "test", "-timeout", defaultTestTimeout, "-v"}, args...)
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

// ThirdPartyGoBuild is a test for the Go build of the third-party projects.
func ThirdPartyGoBuild(ctx *util.Context, testName string) (*TestResult, error) {
	pkgs := []string{"code.google.com/...", "github.com/..."}
	return goBuild(ctx, testName, nil, pkgs, nil)
}

// ThirdPartyGoTest is a test for the Go tests of the third-party projects.
func ThirdPartyGoTest(ctx *util.Context, testName string) (*TestResult, error) {
	// Run the tests excluding TestTestmainPackage from
	// code.google.com/p/go.tools/go/ssa/interp as the package has
	// a test that expects to see FAIL: TestBar which causes
	// go2xunit to fail.
	args := []string{"-run", "[^(TestTestmainPackage)]"}
	pkgs := []string{"code.google.com/...", "github.com/..."}
	return goTest(ctx, testName, args, pkgs, nil)
}

// ThirdPartyGoRace is a test for the Go data-race tests of the third-party projects.
func ThirdPartyGoRace(ctx *util.Context, testName string) (*TestResult, error) {
	// Run the tests excluding TestTestmainPackage from
	// code.google.com/p/go.tools/go/ssa/interp as the package has
	// a test that expects to see FAIL: TestBar which causes
	// go2xunit to fail.
	args := []string{"-race", "-run", "[^(TestTestmainPackage)]"}
	pkgs := []string{"code.google.com/...", "github.com/..."}
	return goTest(ctx, testName, args, pkgs, nil)
}

// ToolsGoBuild is a test for the Go build of the tools project.
func ToolsGoBuild(ctx *util.Context, testName string) (*TestResult, error) {
	pkgs := []string{"tools/..."}
	return goBuild(ctx, testName, nil, pkgs, nil)
}

// ToolsGoCoverage is a test for the Go test coverage of the tools project.
func ToolsGoCoverage(ctx *util.Context, testName string) (*TestResult, error) {
	pkgs := []string{"tools/..."}
	return goCoverage(ctx, testName, nil, pkgs, nil)
}

// ToolsGoTest is a test for the Go tests of the tools project.
func ToolsGoTest(ctx *util.Context, testName string) (*TestResult, error) {
	pkgs := []string{"tools/..."}
	return goTest(ctx, testName, nil, pkgs, nil)
}

// ToolsGoRace is a test for the Go data-race tests of the tools project.
func ToolsGoRace(ctx *util.Context, testName string) (*TestResult, error) {
	args := []string{"-race"}
	pkgs := []string{"tools/..."}
	return goTest(ctx, testName, args, pkgs, nil)
}

// VeyronGoBuild is a test for the Go build of the veyron projects.
func VeyronGoBuild(ctx *util.Context, testName string) (*TestResult, error) {
	args := []string{"-tags", "veyronbluetooth"}
	pkgs := []string{"veyron.io/..."}
	profiles := []string{"proximity"}
	return goBuild(ctx, testName, args, pkgs, profiles)
}

// VeyronGoCoverage is a test for the Go test coverage of the veyron projects.
func VeyronGoCoverage(ctx *util.Context, testName string) (*TestResult, error) {
	pkgs := []string{"veyron.io/..."}
	profiles := []string{"proximity"}
	return goCoverage(ctx, testName, nil, pkgs, profiles)
}

// VeyronGoTest is a test for the Go tests of the veyron projects.
func VeyronGoTest(ctx *util.Context, testName string) (*TestResult, error) {
	args := []string{"-tags", "veyronbluetooth"}
	pkgs := []string{"veyron.io/..."}
	profiles := []string{"proximity"}
	return goTest(ctx, testName, args, pkgs, profiles)
}

// VeyronGoRace is a test for the Go data-race tests of the veyron projects.
func VeyronGoRace(ctx *util.Context, testName string) (*TestResult, error) {
	args := []string{"-race", "-tags", "veyronbluetooth"}
	pkgs := []string{"veyron.io/..."}
	profiles := []string{"proximity"}
	return goTest(ctx, testName, args, pkgs, profiles)
}
