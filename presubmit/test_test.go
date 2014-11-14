package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseLastCompletedBuildStatusJsonResponse(t *testing.T) {
	// "SUCCESS" status.
	input := `
	{
		"building": false,
		"fullDisplayName": "veyron-android-build #182",
		"result": "SUCCESS"
	}
	`
	expected := "SUCCESS"
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
	expected = "FAILURE"
	got, err = parseLastCompletedBuildStatusJsonResponse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("want no errors, got: %v", err)
	}
	if expected != got {
		t.Fatalf("want %v, got %v", expected, got)
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
