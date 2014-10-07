package impl

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"tools/lib/cmdline"
	"tools/lib/gerrit"
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

func TestParseValidNetRcFile(t *testing.T) {
	// Valid content.
	netrcFileContent := `
machine veyron.googlesource.com login git-jingjin.google.com password 12345
machine veyron-review.googlesource.com login git-jingjin.google.com password 54321
	`
	got, err := parseNetRcFile(strings.NewReader(netrcFileContent))
	expected := map[string]credential{
		"veyron.googlesource.com": credential{
			username: "git-jingjin.google.com",
			password: "12345",
		},
		"veyron-review.googlesource.com": credential{
			username: "git-jingjin.google.com",
			password: "54321",
		},
	}
	if err != nil {
		t.Fatalf("want no errors, got: %v", err)
	}
	if !reflect.DeepEqual(expected, got) {
		t.Fatalf("want: %#v, got: %#v", expected, got)
	}
}

func TestParseInvalidNetRcFile(t *testing.T) {
	// Content with invalid entries which should be skipped.
	netRcFileContentWithInvalidEntries := `
machine veyron.googlesource.com login git-jingjin.google.com password
machine_blah veyron3.googlesource.com login git-jingjin.google.com password 12345
machine veyron2.googlesource.com login_blah git-jingjin.google.com password 12345
machine veyron4.googlesource.com login git-jingjin.google.com password_blah 12345
machine veyron-review.googlesource.com login git-jingjin.google.com password 54321
	`
	got, err := parseNetRcFile(strings.NewReader(netRcFileContentWithInvalidEntries))
	expected := map[string]credential{
		"veyron-review.googlesource.com": credential{
			username: "git-jingjin.google.com",
			password: "54321",
		},
	}
	if err != nil {
		t.Fatalf("want no errors, got: %v", err)
	}
	if !reflect.DeepEqual(expected, got) {
		t.Fatalf("want: %#v, got: %#v", expected, got)
	}
}

func TestNewOpenCLs(t *testing.T) {
	queryResults := []gerrit.QueryResult{
		gerrit.QueryResult{
			Ref:      "refs/10/1010/1",
			Repo:     "veyron",
			ChangeID: "abcd",
		},
		gerrit.QueryResult{
			Ref:      "refs/20/1020/2",
			Repo:     "tools",
			ChangeID: "efgh",
		},
		gerrit.QueryResult{
			Ref:      "refs/30/1030/3",
			Repo:     "veyron.js",
			ChangeID: "mn",
		},
	}

	// Both prevRefs and curQueryResults are empty.
	prevRefs := map[string]bool{}
	curQueryResults := []gerrit.QueryResult{}
	got := newOpenCLs(prevRefs, curQueryResults)
	expected := []gerrit.QueryResult{}
	if !reflect.DeepEqual(expected, got) {
		t.Errorf("want: %v, got: %v", expected, got)
	}

	// prevRefs is empty, curQueryResults is not.
	curQueryResults = []gerrit.QueryResult{
		queryResults[0],
		queryResults[1],
	}
	got = newOpenCLs(prevRefs, curQueryResults)
	expected = []gerrit.QueryResult{
		queryResults[0],
		queryResults[1],
	}
	if !reflect.DeepEqual(expected, got) {
		t.Errorf("want: %v, got: %v", expected, got)
	}

	// prevRefs is not empty, curQueryResults is.
	prevRefs = map[string]bool{
		queryResults[0].Ref: true,
	}
	curQueryResults = []gerrit.QueryResult{}
	got = newOpenCLs(prevRefs, curQueryResults)
	expected = []gerrit.QueryResult{}
	if !reflect.DeepEqual(expected, got) {
		t.Errorf("want: %v, got: %v", expected, got)
	}

	// prevRefs and curQueryResults are not empty, and they have overlapping refs.
	prevRefs = map[string]bool{
		queryResults[0].Ref: true,
		queryResults[1].Ref: true,
	}
	curQueryResults = []gerrit.QueryResult{
		queryResults[1],
		queryResults[2],
	}
	got = newOpenCLs(prevRefs, curQueryResults)
	expected = []gerrit.QueryResult{
		queryResults[2],
	}
	if !reflect.DeepEqual(expected, got) {
		t.Errorf("want: %v, got: %v", expected, got)
	}

	// prevRefs and curQueryResults are not empty, and they have NO overlapping refs.
	prevRefs = map[string]bool{
		queryResults[0].Ref: true,
	}
	curQueryResults = []gerrit.QueryResult{
		queryResults[1],
	}
	got = newOpenCLs(prevRefs, curQueryResults)
	expected = []gerrit.QueryResult{
		queryResults[1],
	}
	if !reflect.DeepEqual(expected, got) {
		t.Errorf("want: %v, got: %v", expected, got)
	}
}

func TestTestsForRepo(t *testing.T) {
	configFileContent := `
{
  "veyron": [
    "veyron-go-build",
    "veyron-go-test"
  ],
  "default": [
    "tools-go-build",
    "tools-go-test"
  ]
}
  `

	// Get tests for a repo that is in the config file.
	got, err := testsForRepo([]byte(configFileContent), "veyron", cmdTmp)
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
	got, err = testsForRepo([]byte(configFileContent), "non-exist-repo", cmdTmp)
	expected = []string{}
	if err != nil {
		t.Fatalf("want no errors, got: %v", err)
	}
	if !reflect.DeepEqual(expected, got) {
		t.Errorf("want: %v, got: %v", expected, got)
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

	presubmitTestsConfigFile := filepath.Join(veyronRoot, "tools", "go", "src", "tools", "presubmitter", "presubmit_tests.conf")
	configFileContent, err := ioutil.ReadFile(presubmitTestsConfigFile)
	if err != nil {
		t.Fatalf("ReadFile(%q) failed: %v", presubmitTestsConfigFile, err)
	}
	_, err = testsForRepo(configFileContent, repoFlag, cmdTmp)
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
	expectedSeenTests := map[string]int{}
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
	reportFileContent := `
<?xml version="1.0" encoding="utf-8"?>
<testsuites>
  <testsuite name="ts1" tests="1" errors="0" failures="1" skip="0">
    <testcase classname="c1" name="n1" time="0">
		  <failure message="error">
# tools/presubmitter
tools/go/src/tools/presubmitter/main.go:106: undefined: test
		  </failure>
    </testcase>
  </testsuite>
  <testsuite name="ts2" tests="1" errors="0" failures="1" skip="0">
    <testcase classname="v.c2" name="n2" time="0">
		  <failure message="error">
# some errors.
		  </failure>
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
		"c1::n1":    1,
		"v::c2::n2": 2,
	}
	expected := []string{
		"- c1::n1\n  http://go/vpst/10/testReport/(root)/c1/n1",
		"- v::c2::n2\n  http://go/vpst/10/testReport/v/c2/n2",
		"- v::c2::n2\n  http://go/vpst/10/testReport/v/c2/n2_2",
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

func TestNormalizeNameForTestReport(t *testing.T) {
	expected := "t_1"
	if got := normalizeNameForTestReport("t/1", false); got != expected {
		t.Errorf("want: %v, got: %v", expected, got)
	}
	if got := normalizeNameForTestReport("t.1", false); got != expected {
		t.Errorf("want: %v, got: %v", expected, got)
	}
	if got := normalizeNameForTestReport("t-1", true); got != expected {
		t.Errorf("want: %v, got: %v", expected, got)
	}
}
