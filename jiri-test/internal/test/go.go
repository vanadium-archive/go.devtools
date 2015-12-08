// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"v.io/jiri/collect"
	"v.io/jiri/jiri"
	"v.io/jiri/profiles"
	"v.io/jiri/project"
	"v.io/jiri/runutil"
	"v.io/jiri/tool"
	"v.io/jiri/util"
	"v.io/x/devtools/internal/goutil"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/xunit"
	"v.io/x/lib/host"
	"v.io/x/lib/set"
)

type taskStatus int

const (
	buildPassed taskStatus = iota
	buildFailed
	testPassed
	testFailed
	testTimedout
)

const (
	escNewline = "&#xA;"
	escTab     = "&#x9;"
)

const timeoutDelay = 2 * time.Minute

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

type funcMatcherOpt struct{ funcMatcher }

type argsOpt []string
type exclusionsOpt []exclusion
type jiriGoOpt []string
type nonTestArgsOpt []string
type numWorkersOpt int
type suppressTestOutputOpt bool
type pkgsOpt []string
type suffixOpt string
type timeoutOpt string

func (argsOpt) goBuildOpt()              {}
func (argsOpt) goCoverageOpt()           {}
func (argsOpt) goTestOpt()               {}
func (exclusionsOpt) goTestOpt()         {}
func (funcMatcherOpt) goTestOpt()        {}
func (jiriGoOpt) Opt()                   {}
func (jiriGoOpt) goBuildOpt()            {}
func (jiriGoOpt) goCoverageOpt()         {}
func (jiriGoOpt) goTestOpt()             {}
func (nonTestArgsOpt) goTestOpt()        {}
func (numWorkersOpt) goTestOpt()         {}
func (pkgsOpt) goBuildOpt()              {}
func (pkgsOpt) goCoverageOpt()           {}
func (pkgsOpt) goTestOpt()               {}
func (suffixOpt) goTestOpt()             {}
func (timeoutOpt) goCoverageOpt()        {}
func (timeoutOpt) goTestOpt()            {}
func (MergePoliciesOpt) goBuildOpt()     {}
func (MergePoliciesOpt) goCoverageOpt()  {}
func (MergePoliciesOpt) goTestOpt()      {}
func (suppressTestOutputOpt) goTestOpt() {}

func goListOpts(opts []Opt) []string {
	var ret []string
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case MergePoliciesOpt:
			ret = append(ret, "-merge-policies="+profiles.MergePolicies(typedOpt).String())
		case jiriGoOpt:
			ret = append(ret, typedOpt...)
		}
	}
	return ret
}

func optsFromGoCoverage(opts []goCoverageOpt) []Opt {
	var r []Opt
	for _, o := range opts {
		if v, ok := o.(Opt); ok {
			r = append(r, v)
		}
	}
	return r
}

func optsFromGoTest(opts []goTestOpt) []Opt {
	var r []Opt
	for _, o := range opts {
		if v, ok := o.(Opt); ok {
			r = append(r, v)
		}
	}
	return r
}

// goBuild is a helper function for running Go builds.
func goBuild(jirix *jiri.X, testName string, opts ...goBuildOpt) (_ *test.Result, e error) {
	var buildArgs, pkgs, goFlags []string
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case argsOpt:
			buildArgs = []string(typedOpt)
		case pkgsOpt:
			pkgs = []string(typedOpt)
		case jiriGoOpt:
			goFlags = []string(typedOpt)
		}
	}

	// For better performance, we don't call goutil.List to get all packages and
	// distribute those packages to build workers. Instead, we use "go build"
	// to build "top level" packages stored in "pkgs" which is much faster.
	allPassed, suites := true, []xunit.TestSuite{}
	s := jirix.NewSeq()
	for _, pkg := range pkgs {
		// Build package.
		// The "leveldb" tag is needed to compile the levelDB-based
		// storage engine for the groups service. See v.io/i/632 for more
		// details.
		args := []string{"go"}
		args = append(args, goFlags...)
		args = append(args, "build", "-v", "-tags=leveldb")
		args = append(args, buildArgs...)
		args = append(args, pkg)
		var out bytes.Buffer
		stdout := io.MultiWriter(&out, jirix.Stdout())
		stderr := io.MultiWriter(&out, jirix.Stdout())
		if err := s.Capture(stdout, stderr).Last("jiri", args...); err == nil {
			continue
		}

		// Parse build output to get failed packages and generate xunit test cases
		// for them.
		allPassed = false
		s := xunit.TestSuite{Name: pkg}
		curPkg := ""
		curOutputLines := []string{}
		seenPkgs := map[string]struct{}{}
		scanner := bufio.NewScanner(bytes.NewReader(out.Bytes()))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "# ") {
				if curPkg != "" {
					processBuildOutput(curPkg, curOutputLines, &s, seenPkgs)
				}
				curPkg = line[2:]
				curOutputLines = nil
			} else {
				curOutputLines = append(curOutputLines, line)
			}
		}
		processBuildOutput(curPkg, curOutputLines, &s, seenPkgs)
		suites = append(suites, s)
	}

	// Create the xUnit report when some builds failed.
	if !allPassed {
		if err := xunit.CreateReport(jirix, testName, suites); err != nil {
			return nil, err
		}
		return &test.Result{Status: test.Failed}, nil
	}
	return &test.Result{Status: test.Passed}, nil
}

func processBuildOutput(pkg string, outputLines []string, suite *xunit.TestSuite, seenPkgs map[string]struct{}) {
	if len(outputLines) == 1 && strings.HasPrefix(outputLines[0], "link: warning") {
		return
	}
	if _, ok := seenPkgs[pkg]; ok {
		return
	}
	seenPkgs[pkg] = struct{}{}
	c := xunit.TestCase{
		Classname: pkg,
		Name:      "Build",
	}
	c.Failures = append(c.Failures, xunit.Failure{
		Message: "build failure",
		Data:    strings.Join(outputLines, "\n"),
	})
	suite.Tests++
	suite.Failures++
	suite.Cases = append(suite.Cases, c)
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
func goCoverage(jirix *jiri.X, testName string, opts ...goCoverageOpt) (_ *test.Result, e error) {
	timeout := defaultTestCoverageTimeout
	var args, pkgs, goFlags []string
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case timeoutOpt:
			timeout = string(typedOpt)
		case argsOpt:
			args = []string(typedOpt)
		case pkgsOpt:
			pkgs = []string(typedOpt)
		case jiriGoOpt:
			goFlags = []string(typedOpt)
		}
	}

	s := jirix.NewSeq()

	// Install required tools.
	goInstall := []string{"go"}
	goInstall = append(goInstall, goFlags...)
	goInstall = append(goInstall, "install", "golang.org/x/tools/cmd/cover", "github.com/t-yuki/gocover-cobertura", "bitbucket.org/tebeka/go2xunit")
	if err := s.Last("jiri", goInstall...); err != nil {
		return nil, newInternalError(err, "install coverage tools")
	}

	// Build dependencies of test packages.
	if err := buildTestDeps(jirix, pkgs, goFlags); err != nil {
		if err := xunit.CreateFailureReport(jirix, testName, "BuildTestDependencies", "TestCoverage", "dependencies build failure", err.Error()); err != nil {
			return nil, err
		}
		return &test.Result{Status: test.Failed}, nil
	}

	// Enumerate the packages for which coverage is to be computed.
	fmt.Fprintf(jirix.Stdout(), "listing test packages and functions ... ")
	pkgList, err := goutil.List(jirix.Context, goListOpts(optsFromGoCoverage(opts)), pkgs...)
	if err != nil {
		fmt.Fprintf(jirix.Stdout(), "failed\n%s\n", err.Error())
		if err := xunit.CreateFailureReport(jirix, testName, "ListPackages", "TestCoverage", "listing package failure", err.Error()); err != nil {
			return nil, err
		}
		return &test.Result{Status: test.Failed}, nil
	}
	fmt.Fprintf(jirix.Stdout(), "ok\n")

	// Create a pool of workers.
	numPkgs := len(pkgList)
	tasks := make(chan string, numPkgs)
	taskResults := make(chan coverageResult, numPkgs)
	for i := 0; i < runtime.NumCPU(); i++ {
		go coverageWorker(jirix, timeout, args, tasks, taskResults)
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
	allPassed, suites := true, []xunit.TestSuite{}
	for i := 0; i < numPkgs; i++ {
		result := <-taskResults
		var s *xunit.TestSuite
		switch result.status {
		case buildFailed:
			s = xunit.CreateTestSuiteWithFailure(result.pkg, "TestCoverage", "build failure", result.output, result.time)
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
				ss, err := xunit.TestSuitesFromGoTestOutput(jirix, bytes.NewBufferString(result.output))
				if err != nil {
					// Token too long error.
					if !strings.HasSuffix(err.Error(), "token too long") {
						return nil, err
					}
					s = xunit.CreateTestSuiteWithFailure(result.pkg, "Test", "test output contains lines that are too long to parse", "", result.time)
				} else {
					if len(ss) > 1 {
						return nil, fmt.Errorf("too many testsuites: %d", len(ss))
					}
					s = ss[0]
				}
			}
		}
		if result.coverage != nil {
			result.coverage.Close()
			if err := jirix.NewSeq().RemoveAll(result.coverage.Name()).Done(); err != nil {
				return nil, err
			}
		}
		if s != nil {
			if s.Failures > 0 {
				allPassed = false
				test.Fail(jirix.Context, "%s\n%v\n", result.pkg, result.output)
			} else {
				test.Pass(jirix.Context, "%s\n", result.pkg)
			}
			suites = append(suites, *s)
		}
	}
	close(taskResults)

	// Create the xUnit and cobertura reports.
	if err := xunit.CreateReport(jirix, testName, suites); err != nil {
		return nil, err
	}
	coverage, err := coverageFromGoTestOutput(jirix, &coverageData)
	if err != nil {
		return nil, err
	}
	if err := createCoberturaReport(jirix, testName, coverage); err != nil {
		return nil, err
	}
	if !allPassed {
		return &test.Result{Status: test.Failed}, nil
	}
	return &test.Result{Status: test.Passed}, nil
}

// coverageWorker generates test coverage.
func coverageWorker(jirix *jiri.X, timeout string, args []string, pkgs <-chan string, results chan<- coverageResult) {
	s := jirix.NewSeq()
	for pkg := range pkgs {
		// Compute the test coverage.
		var out bytes.Buffer
		coverageFile, err := ioutil.TempFile("", "")
		if err != nil {
			panic(fmt.Sprintf("TempFile() failed: %v", err))
		}
		args := append([]string{"go", "test", "-tags=leveldb", "-cover", "-coverprofile",
			coverageFile.Name(), "-timeout", timeout, "-v",
		}, args...)
		args = append(args, pkg)
		start := time.Now()
		err = s.Capture(&out, &out).Verbose(false).Last("jiri", args...)
		result := coverageResult{
			pkg:      pkg,
			coverage: coverageFile,
			time:     time.Now().Sub(start),
			output:   out.String(),
		}
		if err != nil {
			oe := runutil.GetOriginalError(err)
			if isBuildFailure(oe, out.String(), pkg) {
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

// funcMatcher is the interface for determing if functions in the loaded ast
// of a package match a certain criteria.
type funcMatcher interface {
	match(*ast.FuncDecl) (bool, string)
}

type matchGoTestFunc struct {
	testNameRE *regexp.Regexp
}

func (t *matchGoTestFunc) match(fn *ast.FuncDecl) (bool, string) {
	name := fn.Name.String()
	// TODO(cnicolaou): match on signature, not just name.
	return t.testNameRE.MatchString(name), name
}
func (t *matchGoTestFunc) goTestOpt() {}

type matchV23TestFunc struct {
	testNameRE *regexp.Regexp
}

func (t *matchV23TestFunc) match(fn *ast.FuncDecl) (bool, string) {
	name := fn.Name.String()
	if !t.testNameRE.MatchString(name) {
		return false, name
	}
	sig := fn.Type
	if len(sig.Params.List) != 1 || sig.Results != nil {
		return false, name
	}
	typ := sig.Params.List[0].Type
	star, ok := typ.(*ast.StarExpr)
	if !ok {
		return false, name
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return false, name
	}
	return pkgIdent.Name == "testing" && sel.Sel.Name == "T", name
}

func (t *matchV23TestFunc) goTestOpt() {}

var (
	goTestNameRE          = regexp.MustCompile("^Test.*")
	goBenchNameRE         = regexp.MustCompile("^Benchmark.*")
	integrationTestNameRE = regexp.MustCompile("^TestV23.*")
)

// goListPackagesAndFuncs is a helper function for listing Go
// packages and obtaining lists of function names that are matched
// by the matcher interface.
func goListPackagesAndFuncs(jirix *jiri.X, opts []Opt, pkgs []string, matcher funcMatcher) ([]string, map[string][]string, error) {
	fmt.Fprintf(jirix.Stdout(), "listing test packages and functions ... ")

	ch, err := profiles.NewConfigHelper(jirix, profiles.UseProfiles, ManifestFilename)
	if err != nil {
		return nil, nil, err
	}
	ch.MergeEnvFromProfiles(profiles.JiriMergePolicies(), profiles.NativeTarget(), "jiri")
	pkgList, err := goutil.List(jirix.Context, goListOpts(opts), pkgs...)
	if err != nil {
		fmt.Fprintf(jirix.Stdout(), "failed\n%s\n", err.Error())
		return nil, nil, err
	}

	matched := map[string][]string{}
	pkgsWithTests := []string{}

	buildContext := build.Default
	buildContext.GOPATH = ch.Get("GOPATH")
	for _, pkg := range pkgList {
		pi, err := buildContext.Import(pkg, ".", build.ImportMode(0))
		if err != nil {
			fmt.Fprintf(jirix.Stdout(), "failed\n%s\n", err.Error())
			return nil, nil, err
		}
		testFiles := append(pi.TestGoFiles, pi.XTestGoFiles...)
		fset := token.NewFileSet() // positions are relative to fset
		for _, testFile := range testFiles {
			file := filepath.Join(pi.Dir, testFile)
			testAST, err := parser.ParseFile(fset, file, nil, parser.Mode(0))
			if err != nil {
				fmt.Fprintf(jirix.Stdout(), "failed\n%s\n", err.Error())
				return nil, nil, err
			}
			for _, decl := range testAST.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				if ok, result := matcher.match(fn); ok {
					matched[pkg] = append(matched[pkg], result)
				}
			}
		}
		if len(matched[pkg]) > 0 {
			pkgsWithTests = append(pkgsWithTests, pkg)
		}
	}

	fmt.Fprintf(jirix.Stdout(), "ok\n")
	return pkgsWithTests, matched, nil
}

// filterExcludedTests filters out excluded tests returning an
// indication of whether this package should be included in test runs
// and a list of the specific tests that should be run (which if nil
// means running all of the tests), and a list of the skipped tests.
func filterExcludedTests(pkg string, testNames []string, exclusions []exclusion) (bool, []string, []string) {
	var excluded []string
	for _, name := range testNames {
		for _, exclusion := range exclusions {
			if exclusion.exclude && exclusion.pkgRE.MatchString(pkg) && exclusion.nameRE.MatchString(name) {
				excluded = append(excluded, name)
				break
			}
		}
	}
	if len(excluded) == 0 {
		// Run all of the tests, none are to be skipped/excluded.
		return true, testNames, nil
	}

	remaining := []string{}
	for _, name := range testNames {
		found := false
		for _, exclude := range excluded {
			if name == exclude {
				found = true
				break
			}
		}
		if !found {
			remaining = append(remaining, name)
		}
	}
	return len(remaining) > 0, remaining, excluded
}

type testResult struct {
	pkg      string
	output   string
	excluded []string
	status   taskStatus
	time     time.Duration
}

const defaultTestTimeout = "20m"

type goTestTask struct {
	pkg string
	// specificTests enumerates the tests to run.
	// Tests are passed to -run as a regex or'ing each item in the slice.
	specificTests []string
	// excludedTests enumerates the tests that are to be excluded as a result
	// of exclusion rules.
	excludedTests []string
}

// goTestAndReport runs goTest and writes an xml report.
func goTestAndReport(jirix *jiri.X, testName string, opts ...goTestOpt) (_ *test.Result, e error) {
	res, suites, err := goTest(jirix, testName, opts...)
	if err != nil {
		return nil, err
	}
	// Create the xUnit report.
	return res, xunit.CreateReport(jirix, testName, suites)
}

// goTest is a helper function for running Go tests.
func goTest(jirix *jiri.X, testName string, opts ...goTestOpt) (_ *test.Result, _ []xunit.TestSuite, e error) {
	timeout := defaultTestTimeout
	var args, pkgs, goFlags []string
	var exclusions []exclusion
	var suffix string
	var matcher funcMatcher
	matcher = &matchGoTestFunc{testNameRE: goTestNameRE}
	numWorkers := runtime.GOMAXPROCS(0)
	var nonTestArgs nonTestArgsOpt
	suppressOutput := false
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case timeoutOpt:
			timeout = string(typedOpt)
		case argsOpt:
			args = []string(typedOpt)
		case suffixOpt:
			suffix = string(typedOpt)
		case exclusionsOpt:
			exclusions = []exclusion(typedOpt)
		case nonTestArgsOpt:
			nonTestArgs = typedOpt
		case funcMatcherOpt:
			matcher = typedOpt
		case pkgsOpt:
			pkgs = []string(typedOpt)
		case suppressTestOutputOpt:
			suppressOutput = bool(typedOpt)
		case numWorkersOpt:
			numWorkers = int(typedOpt)
			if numWorkers < 1 {
				numWorkers = 1
			}
		case jiriGoOpt:
			goFlags = []string(typedOpt)
		}
	}

	// TODO(cnicolaou): this gets run for every test case, which is going
	// to be pretty slow. We should refactor so that it only gets run once.
	// Install required tools.
	goInstall := []string{"go"}
	goInstall = append(goInstall, goFlags...)
	goInstall = append(goInstall, "install", "bitbucket.org/tebeka/go2xunit")
	if err := jirix.NewSeq().Last("jiri", goInstall...); err != nil {
		return nil, nil, newInternalError(err, "install-go2xunit")
	}

	// Build dependencies of test packages.
	if err := buildTestDeps(jirix, pkgs, goFlags); err != nil {
		originalTestName := testName
		if len(suffix) != 0 {
			testName += " " + suffix
		}
		failureSuite := xunit.CreateTestSuiteWithFailure("BuildTestDependencies", originalTestName, "dependencies build failure", err.Error(), 0)
		return &test.Result{Status: test.Failed}, []xunit.TestSuite{*failureSuite}, nil
	}

	// Enumerate the packages to be built and tests to be executed.
	pkgList, pkgAndFuncList, err := goListPackagesAndFuncs(jirix, optsFromGoTest(opts), pkgs, matcher)
	if err != nil {
		originalTestName := testName
		if len(suffix) != 0 {
			testName += " " + suffix
		}
		failureSuite := xunit.CreateTestSuiteWithFailure("goListPackagesAndFuncs", originalTestName, "package pasing failure", err.Error(), 0)
		return &test.Result{Status: test.Failed}, []xunit.TestSuite{*failureSuite}, nil
	}

	// Create a pool of workers.
	numPkgs := len(pkgList)
	tasks := make(chan goTestTask, numPkgs)
	taskResults := make(chan testResult, numPkgs)

	fmt.Fprintf(jirix.Stdout(), "running tests using %d workers...\n", numWorkers)
	fmt.Fprintf(jirix.Stdout(), "running tests concurrently...\n")
	staggeredWorker := func() {
		delay := time.Duration(rand.Int63n(30*1000)) * time.Millisecond
		if jirix.Verbose() {
			fmt.Fprintf(jirix.Stdout(), "staggering start of test worker by %s\n", delay)
		}
		time.Sleep(delay)
		testWorker(jirix, timeout, args, nonTestArgs, tasks, taskResults)
	}
	for i := 0; i < numWorkers; i++ {
		if numWorkers > 1 {
			go staggeredWorker()
		} else {
			go testWorker(jirix, timeout, args, nonTestArgs, tasks, taskResults)
		}
	}

	// Distribute work to workers.
	for _, pkg := range pkgList {
		testThisPkg, specificTests, excludedTests := filterExcludedTests(pkg, pkgAndFuncList[pkg], exclusions)
		if testThisPkg {
			tasks <- goTestTask{pkg, specificTests, excludedTests}
		} else {
			taskResults <- testResult{
				pkg:      pkg,
				output:   "package excluded",
				excluded: excludedTests,
				status:   testPassed,
			}
		}
	}
	close(tasks)

	// Collect the results.

	// excludedTests are a result of exclusion rules in this tool.
	excludedTests := map[string][]string{}
	// skippedTests are a result of testing.Skip calls in the actual
	// tests.
	skippedTests := map[string][]string{}
	allPassed, suites := true, []xunit.TestSuite{}
	for i := 0; i < numPkgs; i++ {
		result := <-taskResults
		var ss []*xunit.TestSuite
		switch result.status {
		case buildFailed:
			ss = append(ss, xunit.CreateTestSuiteWithFailure(result.pkg, "Test", "build failure", result.output, result.time))
		case testTimedout:
			ss = append(ss, xunit.CreateTestSuiteWithFailure(result.pkg, "Test", fmt.Sprintf("test timed out after %s", timeout), "", result.time))
		case testFailed, testPassed:
			if strings.Index(result.output, "no test files") == -1 &&
				strings.Index(result.output, "package excluded") == -1 {
				if testName == "vanadium-go-bench" {
					// TODO(jsimsa): The go2xunit tool used for parsing output
					// of Go tests ignores output of Go benchmarks. We dump
					// output of benchmarks to stdout to persist this
					// information in the console logs of our CI. This is a
					// temporary solution until someone finds the enthusiasm to
					// implement benchmark output parsing, tracking and
					// graphing.
					fmt.Fprintf(jirix.Stdout(), result.output)
				}
				// Escape test output to make sure go2xunit can process it.
				var escapedOutput bytes.Buffer
				if err := xml.EscapeText(&escapedOutput, []byte(result.output)); err != nil {
					msg := fmt.Sprintf("failed to escape test output:\n%s\n", result.output)
					ss = append(ss, xunit.CreateTestSuiteWithFailure(result.pkg, "Test", msg, "", result.time))
				} else {
					// xml.EscapeTest also escapes newlines and tabs.
					// We want to keep them unescaped so that go2xunit can correctly parse
					// the output.
					output := strings.Replace(escapedOutput.String(), escNewline, "\n", -1)
					output = strings.Replace(output, escTab, "\t", -1)
					var err error
					if ss, err = xunit.TestSuitesFromGoTestOutput(jirix, bytes.NewBufferString(output)); err != nil {
						errMsg := ""
						if strings.Contains(err.Error(), "package build failed") {
							// Package build failure.
							errMsg = "failed to build package"
						} else if strings.HasSuffix(err.Error(), "token too long") {
							// Token too long error.
							errMsg = "test output contains lines that are too long to parse"
						}
						if errMsg != "" {
							ss = append(ss, xunit.CreateTestSuiteWithFailure(result.pkg, "Test", errMsg, output, result.time))
						} else {
							return nil, suites, err
						}
					}
				}
				for _, ts := range ss {
					if ts.Skip > 0 {
						for _, c := range ts.Cases {
							if c.Skipped != nil {
								skippedTests[result.pkg] = append(skippedTests[result.pkg], c.Name)
							}
						}
					}
				}
			}
			if len(result.excluded) > 0 {
				excludedTests[result.pkg] = result.excluded
			}
		}
		for _, s := range ss {
			if s.Failures > 0 {
				allPassed = false
			}
			// There are times, generally when running tests that fail from
			// within tests that expect those failures, that we want to
			// supress the output from the test to prevent other tools (e.g.
			// go2xunit from seeing it).
			if !suppressOutput {
				if s.Failures > 0 {
					if result.status == testTimedout {
						test.Fail(jirix.Context, "[TIMED OUT after %s] %s\n", timeout, result.pkg)
					} else {
						test.Fail(jirix.Context, "%s\n%v\n", result.pkg, result.output)
					}
				} else {
					test.Pass(jirix.Context, "%s\n", result.pkg)
				}
				if s.Skip > 0 {
					test.Pass(jirix.Context, "%s (skipped tests: %v)\n", result.pkg, skippedTests[result.pkg])
				}
			}
			newCases := []xunit.TestCase{}
			for _, c := range s.Cases {
				if len(suffix) != 0 {
					c.Name += " " + suffix
				}
				newCases = append(newCases, c)
			}
			s.Cases = newCases
			suites = append(suites, *s)
		}
		if excluded := excludedTests[result.pkg]; excluded != nil && !suppressOutput {
			test.Pass(jirix.Context, "%s (excluded tests: %v)\n", result.pkg, excluded)
		}
	}
	close(taskResults)

	testResult := &test.Result{
		Status:        test.Passed,
		ExcludedTests: excludedTests,
		SkippedTests:  skippedTests,
	}
	if !allPassed {
		// We don't set testResult.Status to TimedOut when any pkgs timed out so
		// that the final test report contains individual test failures/timeouts.
		// If testResult.Status is set to TimedOut, the upstream code will generate
		// a test report that only has a single failed test case saying the whole
		// test timed out. This behavior is useful for other tests (e.g. js tests)
		// but not here.
		testResult.Status = test.Failed
	}
	return testResult, suites, nil
}

// testWorker tests packages.
func testWorker(jirix *jiri.X, timeout string, args, nonTestArgs []string, tasks <-chan goTestTask, results chan<- testResult) {
	s := jirix.NewSeq()
	for task := range tasks {
		// Run the test.
		//
		// The "leveldb" tag is needed to compile the levelDB-based
		// storage engine for the groups service. See v.io/i/632 for more
		// details.
		taskArgs := append([]string{"go", "test", "-tags=leveldb", "-timeout", timeout, "-v"}, args...)

		// Use the -run command-line flag to identify the specific tests to run.
		// If this flag is already set, make sure to override it.
		testsExpr := fmt.Sprintf("^(%s)$", strings.Join(task.specificTests, "|"))
		found := false
		for i, arg := range taskArgs {
			switch {
			case arg == "-run" || arg == "--run":
				taskArgs[i+1] = testsExpr
				found = true
				break
			case strings.HasPrefix(arg, "-run=") || strings.HasPrefix(arg, "--run="):
				taskArgs[i] = fmt.Sprintf("-run=%s", testsExpr)
				found = true
				break
			}
		}
		if !found {
			taskArgs = append(taskArgs, "-run", testsExpr)
		}

		taskArgs = append(taskArgs, task.pkg)
		taskArgs = append(taskArgs, nonTestArgs...)
		var out bytes.Buffer
		start := time.Now()
		timeoutDuration, err := time.ParseDuration(timeout)
		if err != nil {
			results <- testResult{
				status:   testFailed,
				pkg:      task.pkg,
				output:   fmt.Sprintf("time.ParseDuration(%s) failed: %v", timeout, err),
				excluded: task.excludedTests,
			}
			continue
		}
		err = s.Capture(&out, &out).Timeout(timeoutDuration+time.Minute).Verbose(false).Last("jiri", taskArgs...)
		result := testResult{
			pkg:      task.pkg,
			time:     time.Now().Sub(start),
			output:   out.String(),
			excluded: task.excludedTests,
		}
		if err != nil {
			oe := runutil.GetOriginalError(err)
			if isBuildFailure(oe, out.String(), task.pkg) {
				result.status = buildFailed
			} else if runutil.IsTimeout(err) {
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

// buildTestDeps builds dependencies for the given test packages
func buildTestDeps(jirix *jiri.X, pkgs []string, jiriGoFlags []string) error {
	fmt.Fprintf(jirix.Stdout(), "building test dependencies ... ")
	// The "leveldb" tag is needed to compile the levelDB-based storage
	// engine for the groups service. See v.io/i/632 for more details.
	args := []string{"go"}
	args = append(args, jiriGoFlags...)
	args = append(args, "test", "-tags=leveldb", "-i")
	args = append(args, pkgs...)
	var out bytes.Buffer
	if err := jirix.NewSeq().Capture(nil, &out).Last("jiri", args...); err != nil {
		fmt.Fprintf(jirix.Stdout(), "failed\n%s\n", out.String())
		return fmt.Errorf("%v\n%s", err, out.String())
	}
	fmt.Fprintf(jirix.Stdout(), "ok\n")
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
func getListenerPID(jirix *jiri.X, port string) (int, error) {
	// Make sure "lsof" exists.
	_, err := exec.LookPath("lsof")
	if err != nil {
		return -1, fmt.Errorf(`"lsof" not found in the PATH`)
	}

	// Use "lsof" to find the process ID of the listener.
	var out bytes.Buffer
	if err := jirix.NewSeq().Capture(&out, &out).
		Last("lsof", "-i", ":"+port, "-sTCP:LISTEN", "-F", "p"); err != nil {
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

type exclusion struct {
	exclude bool
	nameRE  *regexp.Regexp
	pkgRE   *regexp.Regexp
}

// newExclusion is the exclusion factory.
func newExclusion(pkg, name string, exclude bool) exclusion {
	return exclusion{
		exclude: exclude,
		nameRE:  regexp.MustCompile(name),
		pkgRE:   regexp.MustCompile(pkg),
	}
}

var (
	goExclusions            []exclusion
	goRaceExclusions        []exclusion
	goIntegrationExclusions []exclusion
)

func init() {
	goExclusions = []exclusion{
		// This test triggers a bug in go 1.4.1 garbage collector.
		//
		// https://github.com/veyron/release-issues/issues/1494
		newExclusion("v.io/x/ref/runtime/internal/rpc/stream/vc", "TestConcurrentFlows", isDarwin() && is386()),
		// TODO(jingjin): re-enable this test when the following issue is resolved.
		// https://github.com/vanadium/issues/issues/639
		newExclusion("v.io/x/ref/services/device", "TestV23DeviceManagerMultiUser", isDarwin()),
		// The fsnotify package tests are flaky on darwin. This begs the
		// question of whether we should be relying on this library at
		// all.
		newExclusion("github.com/howeyc/fsnotify", ".*", isDarwin()),
		// This test relies on timing, which results in flakiness on GCE.
		newExclusion("google.golang.org/appengine/internal", "TestDelayedLogFlushing", isCI()),
		// The crypto/ssh TestValidTerminalMode is flakey on Jenkins and
		// sometimes fails when getting a pty.
		newExclusion("golang.org/x/crypto/ssh/test", "TestValidTerminalMode", isCI()),
		// The following tests require ICMP socket permissions which are not enabled
		// by default on linux.
		newExclusion("golang.org/x/net/icmp", "TestPingGoogle", isCI()),
		newExclusion("golang.org/x/net/icmp", "TestNonPrivilegedPing", isCI()),
		// This test has proven flaky under go1.5
		newExclusion("golang.org/x/net/netutil", "TestLimitListener", isCI()),
		// Don't run this test on mac systems prior to Yosemite since it
		// can crash some machines.
		newExclusion("golang.org/x/net/ipv6", ".*", !isYosemite()),
		// This test fails, seemingly because of xml name space changes.
		newExclusion("golang.org/x/net/webdav", "TestMultistatusWriter", isCI()),
		// The following test is way out of date and doesn't work any more.
		newExclusion("golang.org/x/tools", "TestCheck", true),
		// The following two tests use too much memory.
		newExclusion("golang.org/x/tools/go/loader", "TestStdlib", true),
		newExclusion("golang.org/x/tools/go/ssa", "TestStdlib", true),
		// The following test expects to see "FAIL: TestBar" which causes
		// go2xunit to fail.
		newExclusion("golang.org/x/tools/go/ssa/interp", "TestTestmainPackage", true),
		// More broken tests.
		//
		// TODO(jsimsa): Provide more descriptive message.
		newExclusion("golang.org/x/tools/go/types", "TestCheck", true),
		newExclusion("golang.org/x/tools/refactor/lexical", "TestStdlib", true),
		newExclusion("golang.org/x/tools/refactor/importgraph", "TestBuild", true),
		// The godoc test does some really stupid string matching where it doesn't want
		// cmd/gc to appear, but we have v.io/x/ref/cmd/gclogs.
		newExclusion("golang.org/x/tools/cmd/godoc", "TestWeb", true),
		// The mysql tests require a connection to a MySQL database.
		newExclusion("github.com/go-sql-driver/mysql", ".*", true),
		// The gorp tests require a connection to a SQL database, configured
		// through various environment variables.
		newExclusion("github.com/go-gorp/gorp", ".*", true),
		// The check.v1 tests contain flakey benchmark tests which sometimes do
		// not complete, and sometimes complete with unexpected times.
		newExclusion("gopkg.in/check.v1", ".*", true),
		// The tests depend on a c library.
		newExclusion("code.google.com/p/rsc/...", ".*", true),
	}

	// Tests excluded only when running under --race flag.
	goRaceExclusions = []exclusion{
		// This test takes too long in --race mode.
		newExclusion("v.io/x/devtools/v23", "TestV23Generate", true),
		// These third_party tests are flaky on Go1.5 with -race
		newExclusion("golang.org/x/crypto/ssh", ".*", true),
		newExclusion("github.com/paypal/gatt", "TestServing", true),
	}

	// Tests excluded only when running integration tests (with --v23.tests flag).
	goIntegrationExclusions = []exclusion{}
}

// ExcludedTests returns the set of tests to be excluded from the
// tests executed when testing the Vanadium project.
func ExcludedTests() []string {
	return excludedTests(goExclusions)
}

// ExcludedRaceTests returns the set of race tests to be excluded from
// the tests executed when testing the Vanadium project.
func ExcludedRaceTests() []string {
	return excludedTests(goRaceExclusions)
}

// ExcludedIntegrationTests returns the set of integration tests to be excluded
// from the tests executed when testing the Vanadium project.
func ExcludedIntegrationTests() []string {
	return excludedTests(goIntegrationExclusions)
}

func excludedTests(exclusions []exclusion) []string {
	excluded := make([]string, 0, len(exclusions))
	for _, e := range exclusions {
		if e.exclude {
			excluded = append(excluded, fmt.Sprintf("pkg: %v, name: %v", e.pkgRE.String(), e.nameRE.String()))
		}
	}
	return excluded
}

// validateAgainstDefaultPackages makes sure that the packages requested
// via opts are amongst the defaults assuming that all of the defaults are
// specified in <pkg>/... form and returns one of each of the goBuildOpt,
// goCoverageOpt and goTestOpt options.
// If no packages are requested, the defaults are returned.
// TODO(cnicolaou): ideally there'd be one piece of code that understands
//   go package specifications that could be used here.
func validateAgainstDefaultPackages(jirix *jiri.X, opts []Opt, defaults []string) (pkgsOpt, error) {

	optPkgs := []string{}
	for _, opt := range opts {
		switch v := opt.(type) {
		case PkgsOpt:
			optPkgs = []string(v)
		}
	}

	if len(optPkgs) == 0 {
		defsOpt := pkgsOpt(defaults)
		return defsOpt, nil
	}

	defPkgs, err := goutil.List(jirix.Context, goListOpts(opts), defaults...)
	if err != nil {
		return nil, err
	}

	pkgs, err := goutil.List(jirix.Context, goListOpts(opts), optPkgs...)
	if err != nil {
		return nil, err
	}

	for _, p := range pkgs {
		found := false
		for _, d := range defPkgs {
			if p == d {
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("requested packages %v is not one of %v", p, defaults)
		}
	}
	po := pkgsOpt(pkgs)
	return po, nil
}

// getNumWorkersOpt gets the NumWorkersOpt from the given Opt slice
func getNumWorkersOpt(opts []Opt) numWorkersOpt {
	for _, opt := range opts {
		switch v := opt.(type) {
		case NumWorkersOpt:
			return numWorkersOpt(v)
		}
	}
	return numWorkersOpt(runtime.NumCPU())
}

// thirdPartyGoBuild runs Go build for third-party projects.
func thirdPartyGoBuild(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(jirix, testName, []string{"base"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Build the third-party Go packages.
	pkgs, err := thirdPartyPkgs(jirix)
	if err != nil {
		return nil, err
	}
	_, err = validateAgainstDefaultPackages(jirix, opts, pkgs)
	if err != nil {
		return nil, err
	}

	// Get packages options. If unset, use "pkgs" above as the default.
	optPkgs := []string{}
	for _, opt := range opts {
		switch v := opt.(type) {
		case PkgsOpt:
			optPkgs = []string(v)
		}
	}
	if len(optPkgs) == 0 {
		optPkgs = pkgs
	}

	return goBuild(jirix, testName, pkgsOpt(optPkgs))
}

// thirdPartyGoTest runs Go tests for the third-party projects.
func thirdPartyGoTest(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(jirix, testName, []string{"base"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Test the third-party Go packages.
	pkgs, err := thirdPartyPkgs(jirix)
	if err != nil {
		return nil, err
	}
	validatedPkgs, err := validateAgainstDefaultPackages(jirix, opts, pkgs)
	if err != nil {
		return nil, err
	}
	suffix := suffixOpt(genTestNameSuffix("GoTest"))
	return goTestAndReport(jirix, testName, suffix, exclusionsOpt(goExclusions), validatedPkgs)
}

// thirdPartyGoRace runs Go data-race tests for third-party projects.
func thirdPartyGoRace(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(jirix, testName, []string{"base"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Test the third-party Go packages for data races.
	pkgs, err := thirdPartyPkgs(jirix)
	if err != nil {
		return nil, err
	}
	validatedPkgs, err := validateAgainstDefaultPackages(jirix, opts, pkgs)
	if err != nil {
		return nil, err
	}
	partPkgs, err := identifyPackagesToTest(jirix, testName, opts, validatedPkgs)
	if err != nil {
		return nil, err
	}
	args := argsOpt([]string{"-race"})
	exclusions := append(goExclusions, goRaceExclusions...)
	suffix := suffixOpt(genTestNameSuffix("GoRace"))
	return goTestAndReport(jirix, testName, suffix, args, timeoutOpt("1h"), exclusionsOpt(exclusions), partPkgs)
}

// thirdPartyPkgs returns a list of Go expressions that describe all
// third-party packages.
func thirdPartyPkgs(jirix *jiri.X) ([]string, error) {
	thirdPartyDir := filepath.Join(jirix.Root, "third_party", "go", "src")
	fileInfos, err := ioutil.ReadDir(thirdPartyDir)
	if err != nil {
		return nil, fmt.Errorf("ReadDir(%v) failed: %v", thirdPartyDir, err)
	}

	pkgs := []string{}
	for _, fileInfo := range fileInfos {
		if fileInfo.IsDir() {
			pkgs = append(pkgs, fileInfo.Name()+"/...")
		}
	}
	return pkgs, nil
}

// vanadiumCopyright checks the copyright for vanadium projects.
func vanadiumCopyright(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(jirix, testName, nil)
	if err != nil {
		return nil, newInternalError(err, "init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Run the jiri copyright check.
	var out bytes.Buffer
	if err := jirix.NewSeq().Capture(&out, &out).
		Last("jiri", "copyright", "check"); err != nil {
		report := fmt.Sprintf(`%v

To fix the above copyright violations run "jiri copyright fix" and commit the changes.
`, out.String())
		if err := xunit.CreateFailureReport(jirix, testName, "RunCopyright", "CheckCopyright", "copyright check failure", report); err != nil {
			return nil, err
		}
		fmt.Fprintf(jirix.Stderr(), "%v", report)
		return &test.Result{Status: test.Failed}, nil
	}
	return &test.Result{Status: test.Passed}, nil
}

// vanadiumGoAPI checks the public Go api for vanadium projects.
func vanadiumGoAPI(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(jirix, testName, nil)
	if err != nil {
		return nil, newInternalError(err, "init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Run the jiri api check.
	var out bytes.Buffer
	if err := jirix.NewSeq().Capture(&out, &out).
		Last("jiri", "api", "check"); err != nil {
		report := fmt.Sprintf("error running 'jiri api check': %v", err)
		if err := xunit.CreateFailureReport(jirix, testName, "RunV23API", "CheckGoAPI", "failed to run the api check tool", report); err != nil {
			return &test.Result{Status: test.Failed}, nil
		}
	}

	output := out.String()
	if len(output) != 0 {
		report := fmt.Sprintf(`%v

If the above changes to public Go API are intentional, run "jiri api fix",
to update the corresponding .api files and commit the changes.
`, out.String())
		if err := xunit.CreateFailureReport(jirix, testName, "RunV23API", "CheckGoAPI", "public api check failure", report); err != nil {
			return nil, err
		}
		fmt.Fprintf(jirix.Stderr(), "%v", report)
		return &test.Result{Status: test.Failed}, nil
	}
	return &test.Result{Status: test.Passed}, nil
}

// vanadiumGoBench runs Go benchmarks for vanadium projects.
func vanadiumGoBench(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(jirix, testName, []string{"base"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Benchmark the Vanadium Go packages.
	pkgs, err := validateAgainstDefaultPackages(jirix, opts, []string{"v.io/..."})
	if err != nil {
		return nil, err
	}
	args := argsOpt([]string{"-bench", "."})
	matcher := funcMatcherOpt{&matchGoTestFunc{testNameRE: goBenchNameRE}}
	timeout := timeoutOpt("1h")
	return goTestAndReport(jirix, testName, args, matcher, timeout, pkgs)
}

// vanadiumGoBuild runs Go build for the vanadium projects.
func vanadiumGoBuild(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(jirix, testName, []string{"base"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}

	// Validate packages.
	defer collect.Error(func() error { return cleanup() }, &e)
	_, err = validateAgainstDefaultPackages(jirix, opts, []string{"v.io/..."})
	if err != nil {
		return nil, err
	}

	// Get packages options. If unset, use "v.io/..." as the default.
	optPkgs := []string{}
	for _, opt := range opts {
		switch v := opt.(type) {
		case PkgsOpt:
			optPkgs = []string(v)
		}
	}
	if len(optPkgs) == 0 {
		optPkgs = []string{"v.io/..."}
	}
	return goBuild(jirix, testName, pkgsOpt(optPkgs))
}

// vanadiumGoCoverage runs Go coverage tests for vanadium projects.
func vanadiumGoCoverage(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(jirix, testName, []string{"base"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Compute coverage for Vanadium Go packages.
	pkgs, err := validateAgainstDefaultPackages(jirix, opts, []string{"v.io/..."})
	if err != nil {
		return nil, err
	}
	return goCoverage(jirix, testName, pkgs)
}

// vanadiumGoDepcop runs Go dependency checks for vanadium projects.
func vanadiumGoDepcop(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(jirix, testName, []string{"base"})
	if err != nil {
		return nil, newInternalError(err, "init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	s := jirix.NewSeq()

	// Build the godepcop tool in a temporary directory.
	tmpDir, err := s.TempDir("", "godepcop-test")
	if err != nil {
		return nil, newInternalError(err, "godepcop-build")
	}
	defer collect.Error(func() error { return jirix.NewSeq().RemoveAll(tmpDir).Done() }, &e)
	binary := filepath.Join(tmpDir, "godepcop")
	if err := s.Last("jiri", "go", "build", "-o", binary, "v.io/x/devtools/godepcop"); err != nil {
		return nil, newInternalError(err, "godepcop-build")
	}

	// Run the godepcop tool.
	var out bytes.Buffer
	if err := s.Capture(&out, &out).Last("jiri", "run", binary, "check", "v.io/..."); err != nil {
		if err := xunit.CreateFailureReport(jirix, testName, "RunGoDepcop", "CheckDependencies", "dependencies check failure", out.String()); err != nil {
			return nil, err
		}
		fmt.Fprintf(jirix.Stderr(), "%v", out.String())
		return &test.Result{Status: test.Failed}, nil
	}
	return &test.Result{Status: test.Passed}, nil
}

// vanadiumGoFormat runs Go format check for vanadium projects.
func vanadiumGoFormat(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(jirix, testName, []string{"base"})
	if err != nil {
		return nil, newInternalError(err, "init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Run the gofmt tool.
	var out bytes.Buffer
	if err := jirix.NewSeq().Capture(&out, &out).
		Last("jiri", "go", "fmt", "v.io/..."); err != nil {
		report := fmt.Sprintf(`The following files do not adhere to the Go formatting conventions:
%v
To resolve this problem, run "gofmt -w <file>" for each of them and commit the changes.
`, out.String())
		if err := xunit.CreateFailureReport(jirix, testName, "RunGoFmt", "CheckFormat", "format check failure", report); err != nil {
			return nil, err
		}
		fmt.Fprintf(jirix.Stderr(), "%v", report)
		return &test.Result{Status: test.Failed}, nil
	}
	return &test.Result{Status: test.Passed}, nil
}

// goGenerateDiff represents a file in a CL whose content does not match that
// expected by vanadiumGoGenerate.
type goGenerateDiff struct {
	path string
	diff string
}

// vanadiumGoGenerate checks that files created by 'go generate' are
// up-to-date.
func vanadiumGoGenerate(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(jirix, testName, []string{"base"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	s := jirix.NewSeq()

	pkgs, err := validateAgainstDefaultPackages(jirix, opts, []string{"v.io/..."})
	if err != nil {
		return nil, err
	}
	pkgStr := strings.Join([]string(pkgs), " ")
	fmt.Fprintf(jirix.Stdout(), "NOTE: This test checks that files created by 'go generate' are up-to-date.\nIf it fails, regenerate them using 'jiri go generate %s'.\n", pkgStr)

	// Stash any uncommitted changes and defer functions that undo any
	// changes created by this function and then unstash the original
	// uncommitted changes.
	projects, err := project.LocalProjects(jirix, false)
	if err != nil {
		return nil, err
	}
	for _, project := range projects {
		if err := s.Pushd(project.Path).Error(); err != nil {
			return nil, err
		}
		stashed, err := jirix.Git().Stash()
		if err != nil {
			return nil, err
		}
		// Take a copy, otherwise the defer below will refer to 'project' by
		// reference, causing all the defer blocks to refer to the same
		// project.
		localProject := project
		defer collect.Error(func() error {
			if err := jirix.NewSeq().Chdir(localProject.Path).Done(); err != nil {
				return err
			}
			if err := jirix.Git().Reset("HEAD"); err != nil {
				return err
			}
			if stashed {
				return jirix.Git().StashPop()
			}
			return nil
		}, &e)
	}
	s.Done() // undo the pushd's

	// Check if 'go generate' creates any changes.
	args := append([]string{"go", "generate"}, []string(pkgs)...)
	if err := s.Last("jiri", args...); err != nil {
		return nil, newInternalError(err, "Go Generate")
	}
	dirtyFiles := []goGenerateDiff{}
	if currentDir, err := os.Getwd(); err != nil {
		return nil, err
	} else {
		defer collect.Error(func() error {
			return jirix.NewSeq().Chdir(currentDir).Done()
		}, &e)
	}
	for _, project := range projects {
		files, err := jirix.Git(tool.RootDirOpt(project.Path)).FilesWithUncommittedChanges()
		if err != nil {
			return nil, err
		}
		if len(files) > 0 {
			if err := s.Pushd(project.Path).Error(); err != nil {
				return nil, err
			}
			for _, file := range files {
				var diff string
				var out bytes.Buffer
				if err := s.Capture(&out, nil).Last("git", "diff", file); err != nil {
					fmt.Fprintf(jirix.Stderr(), "git diff failed, no diff will be available for %s: %v\n", file, err)
					diff = fmt.Sprintf("<not available: %v>", err)
				} else {
					diff = out.String()
				}
				fullPath := filepath.Join(project.Path, file)
				fullPath = strings.TrimPrefix(fullPath, jirix.Root+string(filepath.Separator))
				dirtyFile := goGenerateDiff{
					path: fullPath,
					diff: diff,
				}
				dirtyFiles = append(dirtyFiles, dirtyFile)
			}
		}
	}
	s.Done() // undo the pushd's

	if len(dirtyFiles) != 0 {
		fmt.Fprintf(jirix.Stdout(), "\nThe following go generated files are not up-to-date:\n")
		for _, dirtyFile := range dirtyFiles {
			fmt.Fprintf(jirix.Stdout(), "\t* %s\n", dirtyFile.path)
		}
		fmt.Fprintln(jirix.Stdout())
		// Generate xUnit report.
		suites := []xunit.TestSuite{}
		for _, dirtyFile := range dirtyFiles {
			fmt.Fprintf(jirix.Stdout(), "Diff for %s:\n%s\n", dirtyFile.path, dirtyFile.diff)
			s := xunit.CreateTestSuiteWithFailure("GoGenerate", dirtyFile.path, "go generate failure", fmt.Sprintf("Outdated file: %s\nDiff: %s\n", dirtyFile.path, dirtyFile.diff), 0)
			suites = append(suites, *s)
		}
		if err := xunit.CreateReport(jirix, testName, suites); err != nil {
			return nil, err
		}
		return &test.Result{Status: test.Failed}, nil
	}
	return &test.Result{Status: test.Passed}, nil
}

// vanadiumGoRace runs Go data-race tests for vanadium projects.
func vanadiumGoRace(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(jirix, testName, []string{"base"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	pkgs, err := validateAgainstDefaultPackages(jirix, opts, []string{"v.io/..."})
	if err != nil {
		return nil, err
	}
	partPkgs, err := identifyPackagesToTest(jirix, testName, opts, pkgs)
	if err != nil {
		return nil, err
	}
	exclusions := append(goExclusions, goRaceExclusions...)
	args := argsOpt([]string{"-race"})
	timeout := timeoutOpt("30m")
	suffix := suffixOpt(genTestNameSuffix("GoRace"))
	return goTestAndReport(jirix, testName, args, timeout, suffix, exclusionsOpt(exclusions), partPkgs)
}

// identifyPackagesToTest returns a slice of packages to test using the
// following algorithm:
// - The part index is stored in the "P" environment variable. If it is not
//   defined, return all packages.
// - If the part index is found, return the corresponding packages read and
//   processed from the config file. Note that for a test T with N parts, we
//   only specify the packages for the first N-1 parts in the config file. The
//   last part will automatically include all the packages that are not found
//   in the first N-1 parts.
func identifyPackagesToTest(jirix *jiri.X, testName string, opts []Opt, allPkgs []string) (pkgsOpt, error) {
	// Read config file to get the part.
	config, err := util.LoadConfig(jirix)
	if err != nil {
		return nil, err
	}
	parts := config.TestParts(testName)
	if len(parts) == 0 {
		return pkgsOpt(allPkgs), nil
	}

	// Get part index from optionals.
	index := -1
	for _, opt := range opts {
		switch v := opt.(type) {
		case PartOpt:
			index = int(v)
		}
	}
	if index == -1 {
		return pkgsOpt(allPkgs), nil
	}

	// Get packages specified in test-parts before the current index.
	existingPartsPkgs := map[string]struct{}{}
	for i := 0; i < index; i++ {
		curPkgs, err := getPkgsFromSpec(jirix, opts, parts[i])
		if err != nil {
			return nil, err
		}
		set.String.Union(existingPartsPkgs, set.String.FromSlice(curPkgs))
	}

	// Get packages for the current index.
	pkgs, err := goutil.List(jirix.Context, goListOpts(opts), allPkgs...)
	if err != nil {
		return nil, err
	}
	if index < len(parts) {
		curPkgs, err := getPkgsFromSpec(jirix, opts, parts[index])
		if err != nil {
			return nil, err
		}
		pkgs = curPkgs
	}

	// Exclude "existingPartsPkgs" from "pkgs".
	rest := []string{}
	for _, pkg := range pkgs {
		if _, ok := existingPartsPkgs[pkg]; !ok {
			rest = append(rest, pkg)
		}
	}
	return pkgsOpt(rest), nil
}

// getPkgsFromSpec parses the given pkgSpec (a common-separated pkg names) and
// returns a union of all expanded packages.
// TODO(jingjin): test this function.
func getPkgsFromSpec(jirix *jiri.X, opts []Opt, pkgSpec string) ([]string, error) {
	expandedPkgs := map[string]struct{}{}
	pkgs := strings.Split(pkgSpec, ",")
	for _, pkg := range pkgs {
		curPkgs, err := goutil.List(jirix.Context, goListOpts(opts), pkg)
		if err != nil {
			return nil, err
		}
		set.String.Union(expandedPkgs, set.String.FromSlice(curPkgs))
	}
	return set.String.ToSlice(expandedPkgs), nil
}

// vanadiumGoVet runs go vet checks for vanadium projects.
func vanadiumGoVet(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(jirix, testName, []string{"base"})
	if err != nil {
		return nil, newInternalError(err, "init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	s := jirix.NewSeq()

	// Install the go vet tool.
	if err := s.Last("jiri", "go", "install", "golang.org/x/tools/cmd/vet"); err != nil {
		return nil, newInternalError(err, "install-go-vet")
	}

	// Run the go vet tool.
	var out bytes.Buffer
	if err := s.Capture(&out, &out).Last("jiri", "go", "vet", "v.io/..."); err != nil {
		if err := xunit.CreateFailureReport(jirix, testName, "RunGoVet", "GoVetChecks", "go vet check failure", out.String()); err != nil {
			return nil, err
		}
		fmt.Fprintf(jirix.Stderr(), "%v", out.String())
		return &test.Result{Status: test.Failed}, nil
	}
	return &test.Result{Status: test.Passed}, nil
}

// vanadiumGoTest runs Go tests for vanadium projects.
func vanadiumGoTest(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(jirix, testName, []string{"base"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Test the Vanadium Go packages.
	pkgs, err := validateAgainstDefaultPackages(jirix, opts, []string{"v.io/..."})
	if err != nil {
		return nil, err
	}
	args := argsOpt([]string{})
	suffix := suffixOpt(genTestNameSuffix("GoTest"))
	return goTestAndReport(jirix, testName, suffix, exclusionsOpt(goExclusions), getNumWorkersOpt(opts), pkgs, args)
}

// vanadiumIntegrationTest runs integration tests for Vanadium
// projects.
func vanadiumIntegrationTest(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	// We need a shorter root/tmp dir to keep the length of unix domain socket
	// path under limit (108 for linux and 104 for darwin).
	shorterRootDir := filepath.Join(os.Getenv("HOME"), "tmp", "vit")
	cleanup, err := initTest(jirix, testName, []string{"base"}, rootDirOpt(shorterRootDir))
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	pkgs, err := validateAgainstDefaultPackages(jirix, opts, []string{"v.io/..."})
	if err != nil {
		return nil, err
	}
	suffix := suffixOpt(genTestNameSuffix("V23Test"))
	nonTestArgs := nonTestArgsOpt([]string{"-v23.tests"})
	matcher := funcMatcherOpt{&matchV23TestFunc{testNameRE: integrationTestNameRE}}
	env := jirix.Env()
	env["V23_BIN_DIR"] = binDirPath()
	newCtx := jirix.Clone(tool.ContextOpts{Env: env})
	return goTestAndReport(newCtx, testName, suffix, getNumWorkersOpt(opts), nonTestArgs, matcher, exclusionsOpt(goIntegrationExclusions), pkgs)
}

// binOrder determines if the regression tests use
// new binaries for the selected binSet and old binaries for
// everything else, or the opposite.
type binOrder string

const (
	binSetOld  = binOrder("old")
	binSetNew  = binOrder("new")
	binSetBoth = binOrder("")
)

// regressionDate is just a time.Time but we define a new type
// so we can Marshal and Unmarshal it from JSON easily.
// We also allow both YYYY-MM-DD and a relative number
// of days before today as valid representations.
type regressionDate time.Time

func (d *regressionDate) UnmarshalJSON(in []byte) error {
	str := string(in)
	if t, err := time.Parse("\"2006-01-02\"", str); err == nil {
		*d = regressionDate(t)
		return nil
	}
	if days, err := strconv.ParseUint(string(in), 10, 32); err == nil {
		*d = regressionDate(time.Now().AddDate(0, 0, -int(days)))
		return nil
	}
	return fmt.Errorf("Could not parse date as either YYYY-MM-DD or a number of days: %s", str)
}
func (d *regressionDate) MarshalJSON() ([]byte, error) {
	return []byte((*time.Time)(d).Format("\"2006-01-02\"")), nil
}

type binSet struct {
	Name     string   `json:"name"`
	Order    binOrder `json:"order,omitempty"`
	Binaries []string `json:"binaries"`
}

type regressionTestConfig struct {
	// Dates to test binaries against.
	AgainstDates []regressionDate `json:"againstDates"`
	// If binaries for any given date are missing, go back up to this many
	// days in search for existing binaries.
	DatesGrace int `json:"datesGrace"`
	// Sets of binaries to hold at different dates.
	Sets []binSet `json:"sets"`
	// Regexp defining tests to run.
	Tests string `json:"tests"`
}

func defaultRegressionConfig() *regressionTestConfig {
	config := &regressionTestConfig{
		DatesGrace: 3,
		Sets: []binSet{
			{
				Name:     "agent-only",
				Binaries: []string{"agentd"},
			},
			{
				Name: "prod-services",
				Binaries: []string{
					"agentd",
					"deviced",
					"applicationd",
					"binaryd",
					"identityd",
					"proxyd",
					"mounttabled",
				},
			},
		},
		// By default we only run TestV23Hello.* because there are often
		// changes to flags command line interfaces that often break other
		// tests.  In the future we may be more strict about compatibility
		// for command line utilities and add more tests here.
		Tests: "^TestV23Hello.*",
	}
	now := time.Now()
	for _, days := range []int{1, 5} {
		config.AgainstDates = append(config.AgainstDates,
			regressionDate(now.AddDate(0, 0, -days)))
	}
	return config
}

// vanadiumRegressionTest runs integration tests for Vanadium projects
// using different versions of Vanadium binaries.
func vanadiumRegressionTest(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	var config *regressionTestConfig
	if configStr := os.Getenv("V23_REGTEST_CONFIG"); configStr != "" {
		config = &regressionTestConfig{}
		if err := json.Unmarshal([]byte(configStr), config); err != nil {
			return nil, fmt.Errorf("Unmarshal(%q) failed: %v", configStr, err)
		}
	} else {
		config = defaultRegressionConfig()
	}

	configBytes, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(jirix.Stdout(), "Using config:\n%s\n", string(configBytes))

	// Initialize the test.
	cleanup, err := initTest(jirix, testName, []string{"base"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	pkgs, err := validateAgainstDefaultPackages(jirix, opts, []string{"v.io/..."})
	if err != nil {
		return nil, err
	}
	globalOpts := []goTestOpt{
		getNumWorkersOpt(opts),
		nonTestArgsOpt([]string{"-v23.tests"}),
		funcMatcherOpt{&matchV23TestFunc{testNameRE: regexp.MustCompile(config.Tests)}},
		pkgs,
	}

	s := jirix.NewSeq()

	// Build all v.io binaries.  We are going to check the binaries at head
	// against those from a previous date.
	//
	// The "leveldb" tag is needed to compile the levelDB-based storage
	// engine for the groups service. See v.io/i/632 for more details.
	if err := s.Last("jiri", "go", "install", "-tags=leveldb", "v.io/..."); err != nil {
		return nil, newInternalError(err, "Install")
	}
	newDir := filepath.Join(jirix.Root, "release", "go", "bin")
	outDir := filepath.Join(regTestBinDirPath(), "bin")

	tmpDir, err := s.TempDir("", "")
	if err != nil {
		return nil, err
	}
	defer collect.Error(func() error { return jirix.NewSeq().RemoveAll(tmpDir).Done() }, &e)
	vbinaryBin := filepath.Join(tmpDir, "vbinary")
	if err := s.Last("jiri", "go", "build", "-o", vbinaryBin, "v.io/x/devtools/vbinary"); err != nil {
		return nil, err
	}
	available, err := listAvailableVanadiumBinaries(jirix, vbinaryBin)
	if err != nil {
		return nil, err
	}
	out := &test.Result{Status: test.Passed}
	suites := []xunit.TestSuite{}
	for _, againstDate := range config.AgainstDates {
		againstTime := time.Time(againstDate)
		var againstDateStr string
		for i := 0; i <= config.DatesGrace; i++ {
			againstDateStr = againstTime.Format("2006-01-02")
			if bytes.Contains(available, []byte(runtime.GOOS+"_"+runtime.GOARCH+"/"+againstDateStr)) {
				if i > 0 {
					fmt.Fprintf(jirix.Stdout(), "no snapshot found for %s; using %s instead\n", time.Time(againstDate).Format("2006-01-02"), againstDateStr)
				}
				break
			}
			if i == config.DatesGrace {
				fmt.Fprintf(jirix.Stdout(), "#### Skipping tests for %s, no snapshot found (grace: %d) ####\n", againstDateStr, config.DatesGrace)
				return nil, fmt.Errorf("no snapshot found for %s (grace: %d)", againstDateStr, config.DatesGrace)
			}
			againstTime = againstTime.AddDate(0, 0, -1)
		}
		oldDir, err := downloadVanadiumBinaries(jirix, vbinaryBin, againstTime)
		if err == noSnapshotErr {
			fmt.Fprintf(jirix.Stdout(), "#### Skipping tests for %s, no snapshot ####\n", againstDateStr)
			return nil, fmt.Errorf("no snapshot found for %s", againstDateStr)
		} else if err != nil {
			return nil, err
		}

		env := jirix.Env()
		env["V23_BIN_DIR"] = outDir
		env["V23_REGTEST_DATE"] = againstDateStr
		newCtx := jirix.Clone(tool.ContextOpts{Env: env})

		for _, set := range config.Sets {
			for _, order := range []binOrder{binSetOld, binSetNew} {
				if set.Order != binSetBoth && set.Order != order {
					continue
				}
				if err := prepareRegressionBinaries(jirix, oldDir, newDir, outDir, set.Binaries, order); err != nil {
					return nil, err
				}
				suffix := fmt.Sprintf("Regression(%s, %s, %s)", againstDateStr, set.Name, order)
				suffixOpt := suffixOpt(genTestNameSuffix(suffix))
				localOpts := append([]goTestOpt{suffixOpt}, globalOpts...)
				fmt.Fprintf(jirix.Stdout(), "#### Running %s ####\n", suffix)
				result, cursuites, err := goTest(newCtx, testName, localOpts...)
				if err != nil {
					return nil, err
				}
				suites = append(suites, cursuites...)
				if result.Status != test.Passed {
					out.Status = test.Failed
				}
				mergeTestSet(out.ExcludedTests, result.ExcludedTests)
				mergeTestSet(out.SkippedTests, result.SkippedTests)
			}
		}
	}
	return out, xunit.CreateReport(jirix, testName, suites)
}

func mergeTestSet(into map[string][]string, from map[string][]string) {
	for k, v := range from {
		into[k] = append(into[k], v...)
	}
}

// noSnapshotErr is returned from downloadVanadiumBinaries when there were no
// binaries for the given date.
var noSnapshotErr = fmt.Errorf("no snapshots for specified date.")

func listAvailableVanadiumBinaries(jirix *jiri.X, bin string) ([]byte, error) {
	args := []string{
		"-key-file", os.Getenv("V23_KEY_FILE"),
		"list",
	}
	var out bytes.Buffer
	if err := jirix.NewSeq().Capture(&out, nil).Last(bin, args...); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func downloadVanadiumBinaries(jirix *jiri.X, bin string, date time.Time) (binDir string, e error) {
	dateStr := date.Format("2006-01-02")
	binDir = filepath.Join(regTestBinDirPath(), dateStr)
	s := jirix.NewSeq()
	if _, err := s.Stat(binDir); err == nil {
		return binDir, nil
	} else if !runutil.IsNotExist(err) {
		return "", err
	}
	args := []string{
		"-date-prefix", dateStr,
		"-key-file", os.Getenv("V23_KEY_FILE"),
		"download",
		"-attempts=3",
		"-output-dir", binDir,
	}
	if err := s.Last(bin, args...); err != nil {
		exiterr, ok := err.(*exec.ExitError)
		if !ok {
			return "", err
		}
		status, ok := exiterr.Sys().(syscall.WaitStatus)
		if !ok {
			return "", err
		}
		if status.ExitStatus() == util.NoSnapshotExitCode {
			return "", noSnapshotErr
		}
		return "", err
	}
	return binDir, nil
}

// prepareRegressionBinaries assembles binaries into the directory out by taking
// binaries from in1 and in2.  Binaries in the list take1 will be taken
// from 1, other will be taken from 2.
func prepareRegressionBinaries(jirix *jiri.X, in1, in2, out string, targetBinaries []string, order binOrder) error {
	s := jirix.NewSeq()
	if err := s.
		RemoveAll(out).
		MkdirAll(out, os.FileMode(0755)).Done(); err != nil {
		return err
	}
	if order != binSetNew {
		in1, in2 = in2, in1
	}
	take2 := set.String.FromSlice(targetBinaries)
	binaries := make(map[string]string)

	// First take everything from in1.
	fileInfos, err := ioutil.ReadDir(in1)
	if err != nil {
		return fmt.Errorf("ReadDir(%v) failed: %v", in1, err)
	}
	for _, fileInfo := range fileInfos {
		name := fileInfo.Name()
		binaries[name] = filepath.Join(in1, name)
	}

	// Now take things from in2 if they are in take2, or were missing from in1.
	fileInfos, err = ioutil.ReadDir(in2)
	if err != nil {
		return fmt.Errorf("ReadDir(%v) failed: %v", in2, err)
	}
	for _, fileInfo := range fileInfos {
		name := fileInfo.Name()
		_, inSet := take2[name]
		if inSet || binaries[name] == "" {
			binaries[name] = filepath.Join(in2, name)
		}
	}

	// We want to print some info in sorted order for easy reading.
	sortedBinaries := make([]string, 0, len(binaries))
	for name := range binaries {
		sortedBinaries = append(sortedBinaries, name)
	}
	sort.Strings(sortedBinaries)

	fmt.Fprintf(jirix.Stdout(), "Using binaries from %s and %s out of %s\n", in1, in2, out)
	for _, name := range sortedBinaries {
		src := binaries[name]
		dst := filepath.Join(out, name)
		if err := s.Symlink(src, dst).Done(); err != nil {
			return err
		}
	}

	return nil
}

func genTestNameSuffix(baseSuffix string) string {
	suffixParts := []string{}
	suffixParts = append(suffixParts, runtime.GOOS)
	arch := os.Getenv("GOARCH")
	if arch == "" {
		var err error
		arch, err = host.Arch()
		if err != nil {
			arch = "amd64"
		}
	}
	suffixParts = append(suffixParts, arch)
	suffix := strings.Join(suffixParts, ",")

	if baseSuffix == "" {
		return fmt.Sprintf("[%s]", suffix)
	}
	return fmt.Sprintf("[%s - %s]", baseSuffix, suffix)
}
