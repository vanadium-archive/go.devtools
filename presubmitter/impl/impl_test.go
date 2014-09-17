package impl

import (
	"reflect"
	"strings"
	"testing"

	"tools/lib/gerrit"
)

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
	got, err := testsForRepo([]byte(configFileContent), "veyron")
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
	// This should fall back to getting tests for "_all".
	got, err = testsForRepo([]byte(configFileContent), "non-exist-repo")
	expected = []string{
		"tools-go-build",
		"tools-go-test",
	}
	if err != nil {
		t.Fatalf("want no errors, got: %v", err)
	}
	if !reflect.DeepEqual(expected, got) {
		t.Errorf("want: %v, got: %v", expected, got)
	}
}
