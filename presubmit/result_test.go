// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"reflect"
	"testing"

	"v.io/x/devtools/internal/jenkins"
	"v.io/x/devtools/internal/testutil"
	"v.io/x/devtools/internal/tool"
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
	ctx := tool.NewDefaultContext()
	type test struct {
		testResult                testResultInfo
		postsubmitFailedTestCases []jenkins.TestCase
		expectedGroups            *failedTestCasesGroups
		expectedSeenTests         map[string]int
	}

	tests := []test{
		test{
			testResult: testResultInfo{
				Result:   testutil.TestResult{Status: testutil.TestFailed},
				TestName: "vanadium-go-test",
				AxisValues: axisValuesInfo{
					Arch:      "amd64",
					OS:        "linux",
					PartIndex: 0,
				},
			},
			postsubmitFailedTestCases: []jenkins.TestCase{},
			expectedGroups: &failedTestCasesGroups{
				newFailure: []failedTestCaseInfo{
					failedTestCaseInfo{
						suiteName:    "ts1",
						className:    "c1.n",
						testCaseName: "n1",
						testName:     "vanadium-go-test",
						axisValues: axisValuesInfo{
							Arch:      "amd64",
							OS:        "linux",
							PartIndex: 0,
						},
					},
					failedTestCaseInfo{
						suiteName:    "ts1",
						className:    "c2.n",
						testCaseName: "n2",
						testName:     "vanadium-go-test",
						axisValues: axisValuesInfo{
							Arch:      "amd64",
							OS:        "linux",
							PartIndex: 0,
						},
					},
					failedTestCaseInfo{
						suiteName:    "ts1",
						className:    "go.vanadium.abc",
						testCaseName: "n5",
						testName:     "vanadium-go-test",
						axisValues: axisValuesInfo{
							Arch:      "amd64",
							OS:        "linux",
							PartIndex: 0,
						},
					},
				},
			},
		},
	}

	reporter := testReporter{}
	for _, curTest := range tests {
		gotGroups, err := reporter.genFailedTestCasesGroupsForOneTest(ctx, curTest.testResult, []byte(reportFileContent), curTest.postsubmitFailedTestCases)
		if err != nil {
			t.Fatalf("want no errors, got: %v", err)
		}
		if !reflect.DeepEqual(curTest.expectedGroups, gotGroups) {
			t.Fatalf("want:\n%v, got\n%v", curTest.expectedGroups, gotGroups)
		}
	}
}

func TestGenTestResultLink(t *testing.T) {
	type testCase struct {
		suiteName    string
		className    string
		testCaseName string
		testName     string
		axisValues   axisValuesInfo
		expectedLink string
	}

	jenkinsBuildNumberFlag = 10
	testCases := []testCase{
		testCase{
			suiteName:    "s",
			className:    "c",
			testCaseName: "t",
			testName:     "vanadium-go-test",
			axisValues: axisValuesInfo{
				Arch:      "amd64",
				OS:        "linux",
				PartIndex: 0,
			},
			expectedLink: "- c::t\nhttps://dashboard.staging.v.io/?arch=amd64&class=c&job=vanadium-go-test&n=10&os=linux&part=0&suite=s&test=t&type=presubmit",
		},
		testCase{
			suiteName:    "s/1&2",
			className:    "c",
			testCaseName: "t",
			testName:     "vanadium-go-test",
			axisValues: axisValuesInfo{
				Arch:      "amd64",
				OS:        "linux",
				PartIndex: 0,
			},
			expectedLink: "- c::t\nhttps://dashboard.staging.v.io/?arch=amd64&class=c&job=vanadium-go-test&n=10&os=linux&part=0&suite=s%2F1%262&test=t&type=presubmit",
		},
	}

	for _, test := range testCases {
		if got, expected := genTestResultLink(test.suiteName, test.className, test.testCaseName, test.testName, test.axisValues), test.expectedLink; got != expected {
			t.Fatalf("want:\n%v,\ngot:\n%v", expected, got)
		}
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
