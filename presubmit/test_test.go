package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"tools/lib/cmdline"
	"tools/lib/util"
)

var cmdTmp = &cmdline.Command{
	Name:  "tmp",
	Short: "For testing",
	Long:  "For testing",
	Run:   func(*cmdline.Command, []string) error { return nil },
}

func init() {
	cmdTmp.Init(nil, os.Stdout, os.Stderr)
}

func TestTestsForRepo(t *testing.T) {
	repos := map[string][]string{
		"veyron":  []string{"veyron-go-build", "veyron-go-test"},
		"default": []string{"tools-go-build", "tools-go-test"},
	}

	// Get tests for a repo that is in the config file.
	got, err := testsForRepo(repos, "veyron", cmdTmp)
	expected := []string{
		"veyron-go-build",
		"veyron-go-test",
	}
	if err != nil {
		t.Fatalf("want no errors, got: %v", err)
	}
	if !reflect.DeepEqual(expected, got) {
		t.Errorf("want: %v, got: %v", expected, got)
	}

	// Get tests for a repo that is NOT in the config file.
	// This should return empty tests.
	got, err = testsForRepo(repos, "non-exist-repo", cmdTmp)
	expected = []string{}
	if err != nil {
		t.Fatalf("want no errors, got: %v", err)
	}
	if !reflect.DeepEqual(expected, got) {
		t.Errorf("want: %v, got: %v", expected, got)
	}
}

func TestCreateTests(t *testing.T) {
	type testCase struct {
		dep           map[string][]string
		tests         []string
		expectedTests testInfoMap
		expectDepLoop bool
	}
	testCases := []testCase{
		// A single test without any dependencies.
		testCase{
			dep: map[string][]string{
				"A": []string{},
			},
			tests: []string{"A"},
			expectedTests: testInfoMap{
				"A": &testInfo{
					deps:    []string{},
					visited: true,
				},
			},
		},
		// A -> B
		testCase{
			dep: map[string][]string{
				"A": []string{"B"},
			},
			tests: []string{"A", "B"},
			expectedTests: testInfoMap{
				"A": &testInfo{
					deps:    []string{"B"},
					visited: true,
				},
				"B": &testInfo{
					deps:    []string{},
					visited: true,
				},
			},
		},
		// A -> {B, C, D}
		testCase{
			dep: map[string][]string{
				"A": []string{"B", "C", "D"},
			},
			tests: []string{"A", "B", "C", "D"},
			expectedTests: testInfoMap{
				"A": &testInfo{
					deps:    []string{"B", "C", "D"},
					visited: true,
				},
				"B": &testInfo{
					deps:    []string{},
					visited: true,
				},
				"C": &testInfo{
					deps:    []string{},
					visited: true,
				},
				"D": &testInfo{
					deps:    []string{},
					visited: true,
				},
			},
		},
		// Same as above, but "dep" has no data.
		testCase{
			tests: []string{"A", "B", "C", "D"},
			expectedTests: testInfoMap{
				"A": &testInfo{
					deps:    []string{},
					visited: true,
				},
				"B": &testInfo{
					deps:    []string{},
					visited: true,
				},
				"C": &testInfo{
					deps:    []string{},
					visited: true,
				},
				"D": &testInfo{
					deps:    []string{},
					visited: true,
				},
			},
		},
		// A -> {B, C, D}, but A is the only given test to resolve dependency for.
		testCase{
			dep: map[string][]string{
				"A": []string{"B", "C", "D"},
			},
			tests: []string{"A"},
			expectedTests: testInfoMap{
				"A": &testInfo{
					deps:    []string{},
					visited: true,
				},
			},
		},
		// A -> {B, C, D} -> E
		testCase{
			dep: map[string][]string{
				"A": []string{"B", "C", "D"},
				"B": []string{"E"},
				"C": []string{"E"},
				"D": []string{"E"},
			},
			tests: []string{"A", "B", "C", "D", "E"},
			expectedTests: testInfoMap{
				"A": &testInfo{
					deps:    []string{"B", "C", "D"},
					visited: true,
				},
				"B": &testInfo{
					deps:    []string{"E"},
					visited: true,
				},
				"C": &testInfo{
					deps:    []string{"E"},
					visited: true,
				},
				"D": &testInfo{
					deps:    []string{"E"},
					visited: true,
				},
				"E": &testInfo{
					deps:    []string{},
					visited: true,
				},
			},
		},
		// Dependency loop:
		// A -> B
		// B -> C, C -> B
		testCase{
			dep: map[string][]string{
				"A": []string{"B"},
				"B": []string{"C"},
				"C": []string{"B"},
			},
			tests:         []string{"A", "B", "C"},
			expectDepLoop: true,
		},
	}
	for index, test := range testCases {
		got, err := createTests(test.dep, test.tests)
		if test.expectDepLoop {
			if err == nil {
				t.Fatalf("test case %d: want errors, got: %v", index, err)
			}
		} else {
			if err != nil {
				t.Fatalf("test case %d: want no errors, got: %v", index, err)
			}
			if !reflect.DeepEqual(test.expectedTests, got) {
				t.Fatalf("test case %d: want %v, got %v", index, test.expectedTests, got)
			}
		}
	}
}

func TestParseLastCompletedBuildStatusJsonResponse(t *testing.T) {
	// "SUCCESS" status.
	input := `
	{
		"building": false,
		"fullDisplayName": "veyron-android-build #182",
		"result": "SUCCESS"
	}
	`
	expected := true
	got, err := parseLastCompletedBuildStatusJsonResponse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("want no errors, got: %v", err)
	}
	if expected != got {
		t.Fatalf("want %v, got %v", expected, got)
	}

	// "FAILURE" status.
	input = `
	{
		"building": false,
		"fullDisplayName": "veyron-android-build #182",
		"result": "FAILURE"
	}
	`
	expected = false
	got, err = parseLastCompletedBuildStatusJsonResponse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("want no errors, got: %v", err)
	}
	if expected != got {
		t.Fatalf("want %v, got %v", expected, got)
	}
}

func TestTestsConfigFile(t *testing.T) {
	veyronRoot, err := util.VeyronRoot()
	if err != nil {
		t.Fatalf("%v", err)
	}

	presubmitTestsConfigFile := filepath.Join(veyronRoot, "tools", "conf", "presubmit")
	configFileContent, err := ioutil.ReadFile(presubmitTestsConfigFile)
	if err != nil {
		t.Fatalf("ReadFile(%q) failed: %v", presubmitTestsConfigFile, err)
	}
	var testConfig struct {
		// Tests maps repository URLs to a list of test to execute for the given test.
		Tests map[string][]string `json:"tests"`
		// Dependencies maps tests to a list of tests that the test depends on.
		Dependencies map[string][]string `json:"dependencies"`
		// Timeouts maps tests to their timeout value.
		Timeouts map[string]string `json:"timeouts"`
	}
	if err := json.Unmarshal(configFileContent, &testConfig); err != nil {
		t.Fatalf("Unmarshal(%q) failed: %v", configFileContent, err)
	}
	_, err = testsForRepo(testConfig.Tests, repoFlag, cmdTmp)
	if err != nil {
		t.Fatalf("%v", err)
	}
}

func TestParseJUnitReportFileWithoutFailedTests(t *testing.T) {
	// Report with no test failures.
	reportFileContent := `
<?xml version="1.0" encoding="utf-8"?>
<testsuites>
  <testsuite name="ts1" tests="1" errors="0" failures="0" skip="0">
    <testcase classname="c1" name="n1" time="0">
    </testcase>
  </testsuite>
</testsuites>
	`
	seenTests := map[string]int{}
	expectedSeenTests := map[string]int{
		"c1::n1": 1,
	}
	expected := []string{}
	got, err := parseJUnitReportFileForFailedTestLinks(strings.NewReader(reportFileContent), seenTests)
	if err != nil {
		t.Fatalf("want no errors, got: %v", err)
	}
	if !reflect.DeepEqual(expected, got) {
		t.Fatalf("want %v, got %v", expected, got)
	}
	if !reflect.DeepEqual(expectedSeenTests, seenTests) {
		t.Fatalf("want %v, got %v", expectedSeenTests, seenTests)
	}
}

func TestParseJUnitReportFileWithFailedTests(t *testing.T) {
	// Report with some test failures.
	// We have two test cases with the same classname+testname, and the second one is failed.
	reportFileContent := `
<?xml version="1.0" encoding="utf-8"?>
<testsuites>
  <testsuite name="ts1" tests="1" errors="0" failures="1" skip="0">
    <testcase classname="package c1" name="n1" time="0">
		  <failure message="error">
# tools/presubmit
tools/go/src/tools/presubmit/main.go:106: undefined: test
		  </failure>
    </testcase>
  </testsuite>
  <testsuite name="ts2" tests="1" errors="0" failures="0" skip="0">
    <testcase classname="v.c2" name="n2" time="0">
    </testcase>
  </testsuite>
  <testsuite name="ts2" tests="1" errors="0" failures="1" skip="0">
    <testcase classname="v.c2" name="n2" time="0">
		  <failure message="error">
# some other errors.
		  </failure>
    </testcase>
  </testsuite>
</testsuites>
	`
	jenkinsBuildNumberFlag = 10
	seenTests := map[string]int{}
	expectedSeenTests := map[string]int{
		"package c1::n1": 1,
		"v::c2::n2":      2,
	}
	expected := []string{
		"- package c1::n1\nhttp://goto.google.com/vpst/10/testReport/%28root%29/package%20c1/n1",
		"- v::c2::n2\nhttp://goto.google.com/vpst/10/testReport/v/c2/n2_2",
	}
	got, err := parseJUnitReportFileForFailedTestLinks(strings.NewReader(reportFileContent), seenTests)
	if err != nil {
		t.Fatalf("want no errors, got: %v", err)
	}
	if !reflect.DeepEqual(expected, got) {
		t.Fatalf("want %v, got %v", expected, got)
	}
	if !reflect.DeepEqual(expectedSeenTests, seenTests) {
		t.Fatalf("want %v, got %v", expectedSeenTests, seenTests)
	}
}

func TestSafePackageOrClassName(t *testing.T) {
	name := "name"
	expected := "name"
	if got := safePackageOrClassName(name); expected != got {
		t.Fatalf("want %q, got %q", expected, got)
	}

	name = "name\\0/a:b?c#d%e-f_g e"
	expected = "name_0_a_b_c_d_e-f_g e"
	if got := safePackageOrClassName(name); expected != got {
		t.Fatalf("want %q, got %q", expected, got)
	}
}

func TestSafeTestName(t *testing.T) {
	name := "name"
	expected := "name"
	if got := safeTestName(name); expected != got {
		t.Fatalf("want %q, got %q", expected, got)
	}

	name = "name-a b$c_d"
	expected = "name_a_b$c_d"
	if got := safeTestName(name); expected != got {
		t.Fatalf("want %q, got %q", expected, got)
	}
}
