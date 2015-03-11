package main

import (
	"reflect"
	"testing"

	"v.io/x/devtools/lib/jenkins"
	"v.io/x/devtools/lib/testutil"
	"v.io/x/devtools/lib/util"
)

func TestGenFailedTestCasesGroupsForOneTest(t *testing.T) {
	reportFileContent := `
<?xml version="1.0" encoding="utf-8"?>
<testsuites>
  <testsuite name="ts1" tests="4" errors="2" failures="2" skip="0">
    <testcase classname="c1.n" name="n1" time="0">
		  <failure message="error">
# v.io/x/devtools/presubmit
release/go/src/v.io/x/devtools/presubmit/main.go:106: undefined: test
		  </failure>
    </testcase>
    <testcase classname="c2.n" name="n2" time="0">
		  <failure message="error">
# v.io/x/devtools/v23
release/go/src/v.io/x/devtools/v23/main.go:1: you should feel bad
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
# v.io/x/devtools/v23
release/go/src/v.io/x/devtools/v23/main.go:1: you should feel bad
		  </failure>
    </testcase>
  </testsuite>
</testsuites>
	`
	jenkinsBuildNumberFlag = 10
	ctx := util.DefaultContext()
	type test struct {
		testResult                testResultInfo
		postsubmitFailedTestCases []jenkins.TestCase
		expectedGroups            *failedTestCasesGroups
		expectedSeenTests         map[string]int
	}

	tests := []test{
		test{
			testResult: testResultInfo{
				Result:     testutil.TestResult{Status: testutil.TestFailed},
				TestName:   "vanadium-go-test",
				SlaveLabel: "linux-slave",
			},
			postsubmitFailedTestCases: []jenkins.TestCase{},
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
	}

	reporter := testReporter{}
	for _, curTest := range tests {
		seenTests := map[string]int{}
		gotGroups, err := reporter.genFailedTestCasesGroupsForOneTest(ctx, curTest.testResult, []byte(reportFileContent), seenTests, curTest.postsubmitFailedTestCases)
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
