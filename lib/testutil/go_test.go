package testutil

import (
	"encoding/xml"
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"v.io/tools/lib/util"
)

func init() {
	// Prevent the initTest() function from cleaning up Go object
	// files and binaries to avoid interference with concurrently
	// running tests.
	cleanGo = false
}

// caseMatch checks whether the given test cases match modulo their
// execution time.
func caseMatch(c1, c2 testCase) bool {
	if c1.Name != c2.Name {
		return false
	}
	if c1.Classname != c2.Classname {
		return false
	}
	if !reflect.DeepEqual(c1.Errors, c2.Errors) {
		return false
	}
	if !reflect.DeepEqual(c1.Failures, c2.Failures) {
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
func suiteMatch(s1, s2 testSuite) bool {
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
		if !caseMatch(s1.Cases[i], s2.Cases[i]) {
			return false
		}
	}
	return true
}

// suitesMatch checks whether the given test suites match modulo their
// execution time.
func suitesMatch(ss1, ss2 testSuites) bool {
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

// setupTempHome sets up a temporary HOME directory to which the tests
// will generate their temporary files.
func setupTempHome(t *testing.T, ctx *util.Context) func() {
	workDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", workDir); err != nil {
		t.Fatalf("Setenv() failed: %v", err)
	}
	oldTmpDir := os.Getenv("TMPDIR")
	return func() {
		os.RemoveAll(workDir)
		os.Setenv("HOME", oldHome)
		os.Setenv("TMPDIR", oldTmpDir)
	}
}

var (
	wantBuild = testSuites{
		Suites: []testSuite{
			testSuite{
				Name: "v.io/tools/lib/testutil/testdata/foo",
				Cases: []testCase{
					testCase{
						Classname: "v.io/tools/lib/testutil/testdata/foo",
						Name:      "Build",
					},
				},
				Tests: 1,
			},
		},
	}
	wantTest = testSuites{
		Suites: []testSuite{
			testSuite{
				Name: "v.io/tools/lib/testutil/testdata/foo",
				Cases: []testCase{
					testCase{
						Classname: "v.io/tools/lib/testutil/testdata/foo",
						Name:      "TestFoo",
					},
				},
				Tests: 1,
			},
		},
	}
	wantTestWithTestCaseSuffix = testSuites{
		Suites: []testSuite{
			testSuite{
				Name: "v.io/tools/lib/testutil/testdata/foo",
				Cases: []testCase{
					testCase{
						Classname: "v.io/tools/lib/testutil/testdata/foo",
						Name:      "TestFoo [GoRace]",
					},
				},
				Tests: 1,
			},
		},
	}
	wantCoverage = testCoverage{
		LineRate:   0,
		BranchRate: 0,
		Packages: []testCoveragePkg{
			testCoveragePkg{
				Name:       "v.io/tools/lib/testutil/testdata/foo",
				LineRate:   0,
				BranchRate: 0,
				Complexity: 0,
				Classes: []testCoverageClass{
					testCoverageClass{
						Name:       "-",
						Filename:   "v.io/tools/lib/testutil/testdata/foo/foo.go",
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
									testCoverageLine{Number: 3, Hits: 1},
									testCoverageLine{Number: 4, Hits: 1},
									testCoverageLine{Number: 5, Hits: 1},
								},
							},
						},
					},
				},
			},
		},
	}
)

// TestGoBuild checks the Go build based test logic.
func TestGoBuild(t *testing.T) {
	ctx := util.DefaultContext()

	defer setupTempHome(t, ctx)()
	testName, pkgName := "test-go-build", "v.io/tools/lib/testutil/testdata/foo"
	result, err := goBuild(ctx, testName, []string{pkgName})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got, want := result.Status, TestPassed; got != want {
		t.Fatalf("unexpected result: got %s, want %s", got, want)
	}

	// Check the xUnit report.
	xUnitFile := XUnitReportPath(testName)
	data, err := ioutil.ReadFile(xUnitFile)
	if err != nil {
		t.Fatalf("ReadFile(%v) failed: %v", xUnitFile, err)
	}
	defer os.RemoveAll(xUnitFile)
	var gotBuild testSuites
	if err := xml.Unmarshal(data, &gotBuild); err != nil {
		t.Fatalf("Unmarshal() failed: %v\n%v", err, string(data))
	}
	if !suitesMatch(gotBuild, wantBuild) {
		t.Fatalf("unexpected result:\ngot\n%v\nwant\n%v", gotBuild, wantBuild)
	}
}

// TestGoCoverage checks the Go test coverage based test logic.
func TestGoCoverage(t *testing.T) {
	ctx := util.DefaultContext()

	defer setupTempHome(t, ctx)()
	testName, pkgName := "test-go-coverage", "v.io/tools/lib/testutil/testdata/foo"
	result, err := goCoverage(ctx, testName, []string{pkgName})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got, want := result.Status, TestPassed; got != want {
		t.Fatalf("unexpected result: got %s, want %s", got, want)
	}

	// Check the xUnit report.
	xUnitFile := XUnitReportPath(testName)
	data, err := ioutil.ReadFile(xUnitFile)
	if err != nil {
		t.Fatalf("ReadFile(%v) failed: %v", xUnitFile, err)
	}
	defer os.RemoveAll(xUnitFile)
	var gotTest testSuites
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
		t.Fatalf("ReadFile(%v) failed: %v", xUnitFile, err)
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
	runGoTest(t, "", wantTest)
}

func TestGoTestWithTestCaseSuffix(t *testing.T) {
	runGoTest(t, " [GoRace]", wantTestWithTestCaseSuffix)
}

func runGoTest(t *testing.T, testCaseNameSuffix string, expectedTestSuite testSuites) {
	ctx := util.DefaultContext()

	defer setupTempHome(t, ctx)()
	testName, pkgName := "test-go-test", "v.io/tools/lib/testutil/testdata/foo"
	result, err := goTest(ctx, testName, []string{pkgName}, suffixOpt(testCaseNameSuffix))
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got, want := result.Status, TestPassed; got != want {
		t.Fatalf("unexpected result: got %s, want %s", got, want)
	}

	// Check the xUnit report.
	xUnitFile := XUnitReportPath(testName)
	data, err := ioutil.ReadFile(xUnitFile)
	if err != nil {
		t.Fatalf("ReadFile(%v) failed: %v", xUnitFile, err)
	}
	defer os.RemoveAll(xUnitFile)
	var gotTest testSuites
	if err := xml.Unmarshal(data, &gotTest); err != nil {
		t.Fatalf("Unmarshal() failed: %v\n%v", err, string(data))
	}
	if !suitesMatch(gotTest, expectedTestSuite) {
		t.Fatalf("unexpected result:\ngot\n%v\nwant\n%v", gotTest, expectedTestSuite)
	}
}
