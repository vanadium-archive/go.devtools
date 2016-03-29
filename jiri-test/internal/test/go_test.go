// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"v.io/jiri"
	"v.io/jiri/jiritest"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/xunit"
	"v.io/x/devtools/tooldata"
)

// failuresMatch checks whether the given test failures match. The Message
// field must match exactly, whereas fs1's Data field must contain
// fs2's; fs1 is the 'got' and fs2 the 'want'.
func failuresMatch(fs1, fs2 []xunit.Failure) bool {
	if len(fs1) != len(fs2) {
		return false
	}
	for i := 0; i < len(fs1); i++ {
		if fs1[i].Message != fs2[i].Message {
			return false
		}
		if !strings.Contains(fs1[i].Data, fs2[i].Data) {
			return false
		}
	}
	return true
}

// caseMatch checks whether the given test cases match modulo their
// execution time.
func caseMatch(c1, c2 xunit.TestCase) bool {
	// Test names can have a CPU count appended to them (e.g. TestFoo-12)
	// so we take care to strip that out when comparing with
	// expected results.
	ncpu := runtime.NumCPU()
	re := regexp.MustCompile(fmt.Sprintf("(.*)-%d(.*)", ncpu))
	stripNumCPU := func(s string) string {
		parts := re.FindStringSubmatch(s)
		switch len(parts) {
		case 3:
			return strings.TrimRight(parts[1]+parts[2], " ")
		default:
			return s
		}
	}

	if stripNumCPU(c1.Name) != stripNumCPU(c2.Name) {
		return false
	}
	if c1.Classname != c2.Classname {
		return false
	}
	if !reflect.DeepEqual(c1.Errors, c2.Errors) {
		return false
	}
	if !failuresMatch(c1.Failures, c2.Failures) {
		return false
	}
	return true
}

// coverageMatch checks whether the given test coverages match modulo
// their timestamps and sources.
func coverageMatch(c1, c2 testCoverage) bool {
	if c1.BranchRate != c2.BranchRate {
		return false
	}
	if c1.LineRate != c2.LineRate {
		return false
	}
	if !reflect.DeepEqual(c1.Packages, c2.Packages) {
		return false
	}
	return true
}

// suiteMatch checks whether the given test suites match modulo their
// execution time.
func suiteMatch(s1, s2 xunit.TestSuite) bool {
	if s1.Name != s2.Name {
		return false
	}
	if s1.Errors != s2.Errors {
		return false
	}
	if s1.Failures != s2.Failures {
		return false
	}
	if s1.Skip != s2.Skip {
		return false
	}
	if s1.Tests != s2.Tests {
		return false
	}
	if len(s1.Cases) != len(s2.Cases) {
		return false
	}
	for i := 0; i < len(s1.Cases); i++ {
		found := false
		for j := 0; j < len(s2.Cases); j++ {
			if caseMatch(s1.Cases[i], s2.Cases[j]) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// suitesMatch checks whether the given test suites match modulo their
// execution time.
func suitesMatch(ss1, ss2 xunit.TestSuites) bool {
	if len(ss1.Suites) != len(ss2.Suites) {
		return false
	}
	for i := 0; i < len(ss1.Suites); i++ {
		if !suiteMatch(ss1.Suites[i], ss2.Suites[i]) {
			return false
		}
	}
	return true
}

var (
	wantBuild = xunit.TestSuites{
		Suites: []xunit.TestSuite{
			xunit.TestSuite{
				Name: "v.io/x/devtools/jiri-test/internal/test/testdata/foo2",
				Cases: []xunit.TestCase{
					xunit.TestCase{
						Classname: "v.io/x/devtools/jiri-test/internal/test/testdata/foo2",
						Name:      "Build",
						Failures: []xunit.Failure{
							xunit.Failure{
								Message: "build failure",
								Data:    "missing return at end of function",
							},
						},
					},
				},
				Tests:    1,
				Failures: 1,
			},
		},
	}
	wantTest = xunit.TestSuites{
		Suites: []xunit.TestSuite{
			xunit.TestSuite{
				Name: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
				Cases: []xunit.TestCase{
					xunit.TestCase{
						Classname: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
						Name:      "Test1",
					},
					xunit.TestCase{
						Classname: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
						Name:      "Test2",
					},
					xunit.TestCase{
						Classname: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
						Name:      "Test3",
					},
					xunit.TestCase{
						Classname: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
						Name:      "TestV23",
					},
					xunit.TestCase{
						Classname: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
						Name:      "TestV23B",
					},
					xunit.TestCase{
						Classname: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
						Name:      "TestV23Hello",
					},
				},
				Tests: 6,
				Skip:  3,
			},
		},
	}
	wantV23Test = xunit.TestSuites{
		Suites: []xunit.TestSuite{
			xunit.TestSuite{
				Name: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
				Cases: []xunit.TestCase{
					xunit.TestCase{
						Classname: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
						Name:      "TestV23",
					},
					xunit.TestCase{
						Classname: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
						Name:      "TestV23B",
					},
					xunit.TestCase{
						Classname: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
						Name:      "TestV23Hello",
					},
				},
				Tests: 3,
				Skip:  0,
			},
		},
	}
	wantV23TestWithExcludedTests = xunit.TestSuites{
		Suites: []xunit.TestSuite{
			xunit.TestSuite{
				Name: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
				Cases: []xunit.TestCase{
					xunit.TestCase{
						Classname: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
						Name:      "TestV23",
					},
					xunit.TestCase{
						Classname: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
						Name:      "TestV23Hello",
					},
				},
				Tests: 2,
				Skip:  0,
			},
		},
	}
	wantRegressionTest = xunit.TestSuites{
		Suites: []xunit.TestSuite{
			xunit.TestSuite{
				Name: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
				Cases: []xunit.TestCase{
					xunit.TestCase{
						Classname: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
						Name:      "TestV23Hello",
					},
				},
				Tests: 1,
				Skip:  0,
			},
		},
	}
	wantTestWithSuffix = xunit.TestSuites{
		Suites: []xunit.TestSuite{
			xunit.TestSuite{
				Name: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
				Cases: []xunit.TestCase{
					xunit.TestCase{
						Classname: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
						Name:      "Test1 [Suffix]",
					},
					xunit.TestCase{
						Classname: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
						Name:      "Test2 [Suffix]",
					},
					xunit.TestCase{
						Classname: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
						Name:      "Test3 [Suffix]",
					},
					xunit.TestCase{
						Classname: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
						Name:      "TestV23 [Suffix]",
					},
					xunit.TestCase{
						Classname: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
						Name:      "TestV23B [Suffix]",
					},
					xunit.TestCase{
						Classname: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
						Name:      "TestV23Hello [Suffix]",
					},
				},
				Tests: 6,
				Skip:  3,
			},
		},
	}
	wantTestWithExcludedTests = xunit.TestSuites{
		Suites: []xunit.TestSuite{
			xunit.TestSuite{
				Name: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
				Cases: []xunit.TestCase{
					xunit.TestCase{
						Classname: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
						Name:      "Test1",
					},
					xunit.TestCase{
						Classname: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
						Name:      "TestV23",
					},
					xunit.TestCase{
						Classname: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
						Name:      "TestV23B",
					},
					xunit.TestCase{
						Classname: "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
						Name:      "TestV23Hello",
					},
				},
				Tests: 4,
				Skip:  3,
			},
		},
	}
	wantExcludedPackage = xunit.TestSuites{
		Suites: []xunit.TestSuite{},
	}
	wantTestWithTimeout = xunit.TestSuites{
		Suites: []xunit.TestSuite{
			xunit.TestSuite{
				Name: "v.io/x/devtools/jiri-test/internal/test/testdata/foo_timeout",
				Cases: []xunit.TestCase{
					xunit.TestCase{
						Classname: "v.io/x/devtools/jiri-test/internal/test/testdata/foo_timeout",
						Name:      "TestWithSleep",
						Failures: []xunit.Failure{
							xunit.Failure{
								Message: "error",
								Data:    "test timed out after 1s",
							},
						},
					},
				},
				Tests:    1,
				Failures: 1,
			},
		},
	}
	wantCoverage = testCoverage{
		LineRate:   0,
		BranchRate: 0,
		Packages: []testCoveragePkg{
			testCoveragePkg{
				Name:       "v.io/x/devtools/jiri-test/internal/test/testdata/foo",
				LineRate:   0,
				BranchRate: 0,
				Complexity: 0,
				Classes: []testCoverageClass{
					testCoverageClass{
						Name:       "-",
						Filename:   "v.io/x/devtools/jiri-test/internal/test/testdata/foo/foo.go",
						LineRate:   0,
						BranchRate: 0,
						Complexity: 0,
						Methods: []testCoverageMethod{
							testCoverageMethod{
								Name:       "Foo",
								LineRate:   0,
								BranchRate: 0,
								Signature:  "",
								Lines: []testCoverageLine{
									testCoverageLine{Number: 7, Hits: 1},
									testCoverageLine{Number: 8, Hits: 1},
									testCoverageLine{Number: 9, Hits: 1},
								},
							},
						},
					},
				},
			},
		},
	}
)

var skipProfiles = jiriGoOpt([]string{"-skip-profiles"})

// TestGoBuild checks the Go build based test logic.
func TestGoBuild(t *testing.T) {
	fake, cleanupFake := jiritest.NewFakeJiriRoot(t)
	defer cleanupFake()

	if err := tooldata.SaveConfig(fake.X, tooldata.NewConfig()); err != nil {
		t.Fatal(err)
	}

	testName := "test-go-build"
	cleanupTest, err := initTestImpl(fake.X, false, false, false, testName, nil, "")
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer cleanupTest()

	// This package will pass.
	{
		pkgName := "v.io/x/devtools/jiri-test/internal/test/testdata/foo"
		result, err := goBuild(fake.X, testName, pkgsOpt([]string{pkgName}), skipProfiles)
		if err != nil {
			t.Fatalf("%v", err)
		}
		if got, want := result.Status, test.Passed; got != want {
			t.Fatalf("unexpected result: got %s, want %s", got, want)
		}

		// When test passes, there shouldn't be any xunit report.
		xUnitFile := xunit.ReportPath(testName)
		if _, err := os.Stat(xUnitFile); err == nil {
			t.Fatalf("want no xunit report, but got one %q", xUnitFile)
		}
	}

	// This package will fail.
	{
		pkgName := "v.io/x/devtools/jiri-test/internal/test/testdata/foo2"
		result, err := goBuild(fake.X, testName, pkgsOpt([]string{pkgName}), skipProfiles)
		if err != nil {
			t.Fatalf("%v", err)
		}
		if got, want := result.Status, test.Failed; got != want {
			t.Fatalf("unexpected result: got %s, want %s", got, want)
		}

		// Check the xUnit report.
		xUnitFile := xunit.ReportPath(testName)
		data, err := ioutil.ReadFile(xUnitFile)
		if err != nil {
			t.Fatalf("ReadFile(%v) failed: %v", xUnitFile, err)
		}
		defer os.RemoveAll(xUnitFile)
		var gotBuild xunit.TestSuites
		if err := xml.Unmarshal(data, &gotBuild); err != nil {
			t.Fatalf("Unmarshal() failed: %v\n%v", err, string(data))
		}
		if !suitesMatch(gotBuild, wantBuild) {
			t.Fatalf("unexpected result:\ngot\n%#v\nwant\n%#v", gotBuild, wantBuild)
		}
	}
}

// TestGoCoverage checks the Go test coverage based test logic.
func TestGoCoverage(t *testing.T) {
	jirix := newJiriXWithRealRoot(t)
	testName, pkgName := "test-go-coverage", "v.io/x/devtools/jiri-test/internal/test/testdata/foo"
	cleanupTest, err := initTestImpl(jirix, false, false, false, testName, nil, "")
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer cleanupTest()

	result, err := goCoverage(jirix, testName, pkgsOpt([]string{pkgName}), skipProfiles)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got, want := result.Status, test.Passed; got != want {
		t.Fatalf("unexpected result: got %s, want %s", got, want)
	}

	// Check the xUnit report.
	xUnitFile := xunit.ReportPath(testName)
	data, err := ioutil.ReadFile(xUnitFile)
	if err != nil {
		t.Fatalf("ReadFile(%v) failed: %v", xUnitFile, err)
	}
	defer os.RemoveAll(xUnitFile)
	var gotTest xunit.TestSuites
	if err := xml.Unmarshal(data, &gotTest); err != nil {
		t.Fatalf("Unmarshal() failed: %v\n%v", err, string(data))
	}
	if !suitesMatch(gotTest, wantTest) {
		t.Fatalf("unexpected result:\ngot\n%v\nwant\n%v", gotTest, wantTest)
	}

	// Check the cobertura report.
	coberturaFile := coberturaReportPath(testName)
	data, err = ioutil.ReadFile(coberturaFile)
	if err != nil {
		t.Fatalf("ReadFile(%v) failed: %v", coberturaFile, err)
	}
	var gotCoverage testCoverage
	if err := xml.Unmarshal(data, &gotCoverage); err != nil {
		t.Fatalf("Unmarshal() failed: %v\n%v", err, string(data))
	}
	if !coverageMatch(gotCoverage, wantCoverage) {
		t.Fatalf("unexpected result:\ngot\n%v\nwant\n%v", gotCoverage, wantCoverage)
	}
}

// TestGoTest checks the Go test based test logic.
func TestGoTest(t *testing.T) {
	runGoTest(t, "", nil, wantTest, test.Passed, "foo")
}

// TestGoTestWithSuffix checks the suffix mode of Go test based test
// logic.
func TestGoTestWithSuffix(t *testing.T) {
	runGoTest(t, "[Suffix]", nil, wantTestWithSuffix, test.Passed, "foo")
}

// TestGoTestWithExcludedTests checks the excluded test mode of Go
// test based test logic.
func TestGoTestWithExcludedTests(t *testing.T) {
	exclusions := []exclusion{
		newExclusion("v.io/x/devtools/jiri-test/internal/test/testdata/foo", "Test1", false),
		newExclusion("v.io/x/devtools/jiri-test/internal/test/testdata/foo", "Test2", true),
		newExclusion("v.io/x/devtools/jiri-test/internal/test/testdata/foo", "Test3", true),
	}
	runGoTest(t, "", exclusions, wantTestWithExcludedTests, test.Passed, "foo")
}

func TestGoTestWithExcludedTestsWithWildcards(t *testing.T) {
	exclusions := []exclusion{
		newExclusion("v.io/x/devtools/jiri-test/internal/test/testdata/foo", "Test[23]$", true),
	}
	runGoTest(t, "", exclusions, wantTestWithExcludedTests, test.Passed, "foo")
}

func TestGoTestExcludedPackage(t *testing.T) {
	exclusions := []exclusion{
		newExclusion("v.io/x/devtools/jiri-test/internal/test/testdata/foo", ".*", true),
	}
	runGoTest(t, "", exclusions, wantExcludedPackage, test.Passed, "foo")
}

func TestGoTestWithTimeout(t *testing.T) {
	runGoTest(t, "", nil, wantTestWithTimeout, test.Failed, "foo_timeout", timeoutOpt("1s"))
}

func TestGoTestV23(t *testing.T) {
	runGoTest(t, "", nil, wantV23Test, test.Passed, "foo", funcMatcherOpt{&matchV23TestFunc{testNameRE: integrationTestNameRE}}, nonTestArgsOpt([]string{"--v23.tests"}))
}

func TestGoTestV23WithExcludedTests(t *testing.T) {
	exclusions := []exclusion{
		newExclusion("v.io/x/devtools/jiri-test/internal/test/testdata/foo", "TestV23B", true),
	}
	runGoTest(t, "", exclusions, wantV23TestWithExcludedTests, test.Passed, "foo", funcMatcherOpt{&matchV23TestFunc{testNameRE: integrationTestNameRE}}, nonTestArgsOpt([]string{"--v23.tests"}))
}

func TestRegressionTest(t *testing.T) {
	config := defaultRegressionConfig()
	runGoTest(t, "", nil, wantRegressionTest, test.Passed, "foo", funcMatcherOpt{&matchV23TestFunc{testNameRE: regexp.MustCompile(config.Tests)}}, nonTestArgsOpt([]string{"--v23.tests"}))
}

func runGoTest(t *testing.T, suffix string, exclusions []exclusion, expectedTestSuite xunit.TestSuites, expectedStatus test.Status, subPkg string, testOpts ...goTestOpt) {
	jirix := newJiriXWithRealRoot(t)
	testName, pkgName := "test-go-test", "v.io/x/devtools/jiri-test/internal/test/testdata/"+subPkg

	cleanupTest, err := initTestImpl(jirix, false, false, false, testName, nil, "")
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer cleanupTest()

	opts := []goTestOpt{
		pkgsOpt([]string{pkgName}),
		suffixOpt(suffix),
		exclusionsOpt(exclusions),
		suppressTestOutputOpt(true),
		skipProfiles,
	}
	opts = append(opts, testOpts...)

	result, err := goTestAndReport(jirix, testName, opts...)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got, want := result.Status, expectedStatus; got != want {
		t.Fatalf("unexpected result: got %s, want %s", got, want)
	}

	// Check the xUnit report.
	xUnitFile := xunit.ReportPath(testName)
	data, err := ioutil.ReadFile(xUnitFile)
	if err != nil {
		t.Fatalf("ReadFile(%v) failed: %v", xUnitFile, err)
	}
	defer os.RemoveAll(xUnitFile)
	var gotTest xunit.TestSuites
	if err := xml.Unmarshal(data, &gotTest); err != nil {
		t.Fatalf("Unmarshal() failed: %v\n%v", err, string(data))
	}
	if !suitesMatch(gotTest, expectedTestSuite) {
		t.Fatalf("unexpected result:\ngot\n%v\nwant\n%v", gotTest, expectedTestSuite)
	}
}

func newJiriXWithRealRoot(t *testing.T) *jiri.X {
	// Capture JIRI_ROOT using a relative path.  We need the real JIRI_ROOT for
	// test that build and use tools from third_party.
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "..", "..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return &jiri.X{Context: tool.NewDefaultContext(), Root: root}
}
