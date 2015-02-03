package main

import (
	"reflect"
	"testing"

	"v.io/tools/lib/testutil"
	"v.io/tools/lib/util"
)

func TestParseLastCompletedBuildStatus(t *testing.T) {
	// "SUCCESS" status.
	input := `
	{
		"building": false,
		"fullDisplayName": "vanadium-android-build #182",
		"result": "SUCCESS"
	}
	`
	expected := "SUCCESS"
	got, err := parseLastCompletedBuildStatus([]byte(input))
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
		"fullDisplayName": "vanadium-android-build #182",
		"result": "FAILURE"
	}
	`
	expected = "FAILURE"
	got, err = parseLastCompletedBuildStatus([]byte(input))
	if err != nil {
		t.Fatalf("want no errors, got: %v", err)
	}
	if expected != got {
		t.Fatalf("want %v, got %v", expected, got)
	}
}

func TestParseFailedTestCases(t *testing.T) {
	testCases := []struct {
		input             string
		slave             string
		expectedTestCases []testCase
	}{
		// Test results from regular project.
		{
			input: `{
	"suites": [
		{
			"cases": [
				{
					"className": "c1",
					"name": "n1",
					"status": "PASSED"
				},
				{
					"className": "c2",
					"name": "n2",
					"status": "FAILED"
				}
			]
		},
		{
			"cases": [
				{
					"className": "c3",
					"name": "n3",
					"status": "REGRESSION"
				}
			]
		}
	]
}`,
			expectedTestCases: []testCase{
				testCase{
					ClassName: "c2",
					Name:      "n2",
					Status:    "FAILED",
				},
				testCase{
					ClassName: "c3",
					Name:      "n3",
					Status:    "REGRESSION",
				},
			},
		},
	}

	for _, test := range testCases {
		gotTestCases, err := parseFailedTestCases([]byte(test.input))
		if err != nil {
			t.Fatalf("want no errors, got: %v", err)
		}
		if !reflect.DeepEqual(gotTestCases, test.expectedTestCases) {
			t.Fatalf("want %v, got %v", test.expectedTestCases, gotTestCases)
		}
	}
}

func TestGenFailedTestCasesGroupsForOneTest(t *testing.T) {
	reportFileContent := `
<?xml version="1.0" encoding="utf-8"?>
<testsuites>
  <testsuite name="ts1" tests="4" errors="2" failures="2" skip="0">
    <testcase classname="c1.n" name="n1" time="0">
		  <failure message="error">
# v.io/tools/presubmit
release/go/src/v.io/tools/presubmit/main.go:106: undefined: test
		  </failure>
    </testcase>
    <testcase classname="c2.n" name="n2" time="0">
		  <failure message="error">
# v.io/tools/v23
release/go/src/v.io/tools/v23/main.go:1: you should feel bad
		  </failure>
    </testcase>
    <testcase classname="c3.n" name="n3" time="0">
    </testcase>
    <testcase classname="c3.n" name="n3" time="0">
    </testcase>
    <testcase name="&quot;n9&quot;" time="0">
    </testcase>
    <testcase classname="go.vanadium.abc" name="n5" time="0">
		  <failure message="error">
# v.io/tools/v23
release/go/src/v.io/tools/v23/main.go:1: you should feel bad
		  </failure>
    </testcase>
  </testsuite>
</testsuites>
	`
	jenkinsBuildNumberFlag = 10
	ctx := util.DefaultContext()
	type test struct {
		testResult                testResultInfo
		postsubmitFailedTestCases []testCase
		expectedGroups            *failedTestCasesGroups
		expectedSeenTests         map[string]int
	}

	tests := []test{
		test{
			testResult: testResultInfo{
				result:     testutil.TestResult{Status: testutil.TestFailed},
				testName:   "vanadium-go-test",
				slaveLabel: "linux-slave",
			},
			postsubmitFailedTestCases: []testCase{},
			expectedGroups: &failedTestCasesGroups{
				newFailure: []failedTestCaseInfo{
					failedTestCaseInfo{
						className:      "c1.n",
						testCaseName:   "n1",
						seenTestsCount: 1,
						testName:       "vanadium-go-test",
						slaveLabel:     "linux-slave",
					},
					failedTestCaseInfo{
						className:      "c2.n",
						testCaseName:   "n2",
						seenTestsCount: 1,
						testName:       "vanadium-go-test",
						slaveLabel:     "linux-slave",
					},
					failedTestCaseInfo{
						className:      "go.vanadium.abc",
						testCaseName:   "n5",
						seenTestsCount: 1,
						testName:       "vanadium-go-test",
						slaveLabel:     "linux-slave",
					},
				},
			},
			expectedSeenTests: map[string]int{
				"c1::n::n1-linux-slave":             1,
				"c2::n::n2-linux-slave":             1,
				"c3::n::n3-linux-slave":             2,
				`ts1::"n9"-linux-slave`:             1,
				"go::vanadium::abc::n5-linux-slave": 1,
			},
		},
		/*
			test{
				postsubmitFailedTestCases: []testCase{
					testCase{
						ClassName: "c1.n",
						Name:      "n1",
					},
					testCase{
						ClassName: "c4.n",
						Name:      "n4",
					},
				},
				testName:   "vanadium-go-test",
				slaveLabel: "linux-slave",
				expectedGroups: failedTestLinksMap{
					newFailure: []string{
						"- c2::n::n2\nhttp://goto.google.com/vpst/10/L=linux-slave,TEST=vanadium-go-test/testReport/c2/n/n2",
						"- go::vanadium::abc::n5\nhttp://goto.google.com/vpst/10/L=linux-slave,TEST=vanadium-go-test/testReport/go.vanadium/abc/n5",
					},
					knownFailure: []string{
						"- c1::n::n1\nhttp://goto.google.com/vpst/10/L=linux-slave,TEST=vanadium-go-test/testReport/c1/n/n1",
					},
					fixedFailure: []string{
						"- c4::n::n4",
					},
				},
				expectedSeenTests: map[string]int{
					"c1::n::n1-linux-slave":             1,
					"c2::n::n2-linux-slave":             1,
					"c3::n::n3-linux-slave":             2,
					`ts1::"n9"-linux-slave`:             1,
					"go::vanadium::abc::n5-linux-slave": 1,
				},
			},
		*/
	}

	for _, curTest := range tests {
		seenTests := map[string]int{}
		gotGroups, err := genFailedTestCasesGroupsForOneTest(ctx, curTest.testResult, []byte(reportFileContent), seenTests, curTest.postsubmitFailedTestCases)
		if err != nil {
			t.Fatalf("want no errors, got: %v", err)
		}
		if !reflect.DeepEqual(curTest.expectedGroups, gotGroups) {
			t.Fatalf("want:\n%v, got\n%v", curTest.expectedGroups, gotGroups)
		}
		if !reflect.DeepEqual(curTest.expectedSeenTests, seenTests) {
			t.Fatalf("want %v, got %v", curTest.expectedSeenTests, seenTests)
		}
	}
}

func TestGenTestResultLink(t *testing.T) {
	type testCase struct {
		className    string
		testCaseName string
		suffix       int
		testName     string
		slaveLabel   string
		expectedLink string
	}

	jenkinsBuildNumberFlag = 10
	testCases := []testCase{
		testCase{
			className:    "c",
			testCaseName: "t",
			suffix:       0,
			testName:     "vanadium-go-test",
			slaveLabel:   "linux-slave",
			expectedLink: "- c::t\nhttp://goto.google.com/vpst/10/L=linux-slave,TEST=vanadium-go-test/testReport/%28root%29/c/t",
		},
		testCase{
			className:    "c n",
			testCaseName: "t",
			suffix:       0,
			testName:     "vanadium-go-test",
			slaveLabel:   "linux-slave",
			expectedLink: "- c n::t\nhttp://goto.google.com/vpst/10/L=linux-slave,TEST=vanadium-go-test/testReport/%28root%29/c%20n/t",
		},
		testCase{
			className:    "c.n",
			testCaseName: "t",
			suffix:       0,
			testName:     "vanadium-go-test",
			slaveLabel:   "linux-slave",
			expectedLink: "- c::n::t\nhttp://goto.google.com/vpst/10/L=linux-slave,TEST=vanadium-go-test/testReport/c/n/t",
		},
		testCase{
			className:    "c.n",
			testCaseName: "t.n",
			suffix:       0,
			testName:     "vanadium-go-test",
			slaveLabel:   "linux-slave",
			expectedLink: "- c::n::t::n\nhttp://goto.google.com/vpst/10/L=linux-slave,TEST=vanadium-go-test/testReport/c/n/t_n",
		},
		testCase{
			className:    "c.n",
			testCaseName: "t.n",
			suffix:       1,
			testName:     "vanadium-go-test",
			slaveLabel:   "linux-slave",
			expectedLink: "- c::n::t::n\nhttp://goto.google.com/vpst/10/L=linux-slave,TEST=vanadium-go-test/testReport/c/n/t_n",
		},
		testCase{
			className:    "c.n",
			testCaseName: "t.n",
			suffix:       2,
			testName:     "vanadium-go-test",
			slaveLabel:   "linux-slave",
			expectedLink: "- c::n::t::n\nhttp://goto.google.com/vpst/10/L=linux-slave,TEST=vanadium-go-test/testReport/c/n/t_n_2",
		},
	}

	for _, test := range testCases {
		if got, expected := genTestResultLink(test.className, test.testCaseName, test.suffix, test.testName, test.slaveLabel), test.expectedLink; got != expected {
			t.Fatalf("want:\n%v, got:\n%v", expected, got)
		}
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

func TestGenTestFullName(t *testing.T) {
	type testCase struct {
		className        string
		testName         string
		expectedFullName string
	}

	testCases := []testCase{
		testCase{
			className:        "c",
			testName:         "t",
			expectedFullName: "c::t",
		},
		testCase{
			className:        "c.n",
			testName:         "t.n",
			expectedFullName: "c::n::t::n",
		},
	}

	for _, test := range testCases {
		if got, expected := genTestFullName(test.className, test.testName), test.expectedFullName; got != expected {
			t.Fatalf("want %v, got %v", expected, got)
		}
	}
}
