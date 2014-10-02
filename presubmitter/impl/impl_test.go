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

func TestParseFailedTestsJsonResponse(t *testing.T) {
	jenkinsBuildNumberFlag = 10
	testReportJson := `
{
	"suites": [
	  {
			"cases": [
			  {
					"className": "c1",
					"name": "t1",
					"status": "PASSED"
				},
			  {
					"className": "c2",
					"name": "t2",
					"status": "FAILED"
				}
			]
		},
	  {
			"cases": [
			  {
					"className": "c3",
					"name": "t3",
					"status": "REGRESSION"
				}
			]
		}
	]
}
  `
	got, err := parseFailedTestsJsonResponse(strings.NewReader(testReportJson))
	expected := []string{
		"- c2․t2\n  http://go/vpst/10/testReport/(root)/c2/t2/",
		"- c3․t3\n  http://go/vpst/10/testReport/(root)/c3/t3/",
	}
	if err != nil {
		t.Fatalf("want no errors, got: %v", err)
	}
	if !reflect.DeepEqual(expected, got) {
		t.Errorf("want: %v, got: %v", expected, got)
	}

	// Make sure the names are normalized.
	testReportJson = `
{
	"suites": [
	  {
			"cases": [
			  {
					"className": "c.1",
					"name": "t-1",
					"status": "FAILED"
				},
			  {
					"className": "c/2",
					"name": "t2",
					"status": "FAILED"
				}
			]
		}
	]
}
  `
	got, err = parseFailedTestsJsonResponse(strings.NewReader(testReportJson))
	expected = []string{
		"- c․1․t-1\n  http://go/vpst/10/testReport/c/1/t_1/",
		"- c/2․t2\n  http://go/vpst/10/testReport/(root)/c_2/t2/",
	}
	if err != nil {
		t.Fatalf("want no errors, got: %v", err)
	}
	if !reflect.DeepEqual(expected, got) {
		t.Errorf("want: %v, got: %v", expected, got)
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
