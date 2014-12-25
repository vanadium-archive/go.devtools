package main

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"v.io/tools/lib/gerrit"
	"v.io/tools/lib/util"
)

func TestMultiPartCLSet(t *testing.T) {
	set := NewMultiPartCLSet()
	checkMultiPartCLSet(t, -1, map[int]gerrit.QueryResult{}, set)

	// Add a non-multipart cl.
	cl := genCL(1000, 1, "veyron.go.core")
	if err := set.addCL(cl); err == nil {
		t.Fatalf("expected addCL(%v) to fail and it did not", cl)
	}
	checkMultiPartCLSet(t, -1, map[int]gerrit.QueryResult{}, set)

	// Add a multi part cl.
	cl.MultiPart = &gerrit.MultiPartCLInfo{
		Topic: "test",
		Index: 1,
		Total: 2,
	}
	if err := set.addCL(cl); err != nil {
		t.Fatalf("addCL(%v) failed: %v", cl, err)
	}
	checkMultiPartCLSet(t, 2, map[int]gerrit.QueryResult{
		1: cl,
	}, set)

	// Test incomplete.
	if expected, got := false, set.complete(); expected != got {
		t.Fatalf("want %s, got %s", expected, got)
	}

	// Add another multi part cl with the wrong "Total" number.
	cl2 := genMultiPartCL(1050, 2, "veyron.js", "test", 2, 3)
	if err := set.addCL(cl2); err == nil {
		t.Fatalf("expected addCL(%v) to fail and it did not", cl)
	}
	checkMultiPartCLSet(t, 2, map[int]gerrit.QueryResult{
		1: cl,
	}, set)

	// Add another multi part cl with duplicated "Index" number.
	cl3 := genMultiPartCL(1052, 2, "veyron.js", "Test", 1, 2)
	if err := set.addCL(cl3); err == nil {
		t.Fatalf("expected addCL(%v) to fail and it did not", cl)
	}
	checkMultiPartCLSet(t, 2, map[int]gerrit.QueryResult{
		1: cl,
	}, set)

	// Add another multi part cl with the wrong "Topic".
	cl4 := genMultiPartCL(1062, 2, "veyron.js", "test123", 1, 2)
	if err := set.addCL(cl4); err == nil {
		t.Fatalf("expected addCL(%v) to fail and it did not", cl)
	}
	checkMultiPartCLSet(t, 2, map[int]gerrit.QueryResult{
		1: cl,
	}, set)

	// Add a valid multi part cl.
	cl5 := genMultiPartCL(1072, 2, "veyron.js", "test", 2, 2)
	if err := set.addCL(cl5); err != nil {
		t.Fatalf("addCL(%v) failed: %v", cl, err)
	}
	checkMultiPartCLSet(t, 2, map[int]gerrit.QueryResult{
		1: cl,
		2: cl5,
	}, set)

	// Test complete.
	if expected, got := true, set.complete(); expected != got {
		t.Fatalf("want %v, got %v", expected, got)
	}

	// Test cls.
	if expected, got := (clList{cl, cl5}), set.cls(); !reflect.DeepEqual(expected, got) {
		t.Fatalf("want %v, got %v", expected, got)
	}
}

func checkMultiPartCLSet(t *testing.T, expectedTotal int, expectedCLsByPart map[int]gerrit.QueryResult, set *multiPartCLSet) {
	if expectedTotal != set.expectedTotal {
		t.Fatalf("total: want %v, got %v", expectedTotal, set.expectedTotal)
	}
	if !reflect.DeepEqual(expectedCLsByPart, set.parts) {
		t.Fatalf("clsByPart: want %+v, got %+v", expectedCLsByPart, set.parts)
	}
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
	ctx := util.DefaultContext()
	nonMultiPartCLs := clList{
		genCL(1010, 1, "veyron"),
		genCL(1020, 2, "tools"),
		genCL(1030, 3, "veyron.js"),

		genMultiPartCL(1000, 1, "veyron.js", "T1", 1, 2),
		genMultiPartCL(1001, 1, "veyron.go.core", "T1", 2, 2),
		genMultiPartCL(1002, 2, "veyron.go.core", "T2", 2, 2),
		genMultiPartCL(1001, 2, "veyron.go.core", "T1", 2, 2),
	}
	multiPartCLs := clList{
		// Multi part CLs.
		// The first two form a complete set for topic T1.
		// The third one looks like the second one, but has a different topic.
		// The last one has a larger patchset than the second one.
		genMultiPartCL(1000, 1, "veyron.js", "T1", 1, 2),
		genMultiPartCL(1001, 1, "veyron.go.core", "T1", 2, 2),
		genMultiPartCL(1002, 2, "veyron.go.core", "T2", 2, 2),
		genMultiPartCL(1001, 2, "veyron.go.core", "T1", 2, 2),
	}

	type testCase struct {
		prevCLsMap clRefMap
		curCLs     clList
		expected   []clList
	}
	testCases := []testCase{
		////////////////////////////////
		// Tests for non-multipart CLs.

		// Both prevCLsMap and curCLs are empty.
		testCase{
			prevCLsMap: clRefMap{},
			curCLs:     clList{},
			expected:   []clList{},
		},
		// prevCLsMap is empty, curCLs is not.
		testCase{
			prevCLsMap: clRefMap{},
			curCLs:     clList{nonMultiPartCLs[0], nonMultiPartCLs[1]},
			expected:   []clList{clList{nonMultiPartCLs[0]}, clList{nonMultiPartCLs[1]}},
		},
		// prevCLsMap is not empty, curCLs is.
		testCase{
			prevCLsMap: clRefMap{nonMultiPartCLs[0].Ref: nonMultiPartCLs[0]},
			curCLs:     clList{},
			expected:   []clList{},
		},
		// prevCLsMap and curCLs are not empty, and they have overlapping refs.
		testCase{
			prevCLsMap: clRefMap{
				nonMultiPartCLs[0].Ref: nonMultiPartCLs[0],
				nonMultiPartCLs[1].Ref: nonMultiPartCLs[1],
			},
			curCLs:   clList{nonMultiPartCLs[1], nonMultiPartCLs[2]},
			expected: []clList{clList{nonMultiPartCLs[2]}},
		},
		// prevCLsMap and curCLs are not empty, and they have NO overlapping refs.
		testCase{
			prevCLsMap: clRefMap{nonMultiPartCLs[0].Ref: nonMultiPartCLs[0]},
			curCLs:     clList{nonMultiPartCLs[1]},
			expected:   []clList{clList{nonMultiPartCLs[1]}},
		},

		////////////////////////////////
		// Tests for multi part CLs.

		// len(curCLs) > len(prevCLsMap).
		// And the CLs in curCLs have different topics.
		testCase{
			prevCLsMap: clRefMap{multiPartCLs[0].Ref: multiPartCLs[0]},
			curCLs:     clList{multiPartCLs[0], multiPartCLs[2]},
			expected:   []clList{},
		},
		// len(curCLs) > len(prevCLsMap).
		// And the CLs in curCLs form a complete multi part cls set.
		testCase{
			prevCLsMap: clRefMap{multiPartCLs[0].Ref: multiPartCLs[0]},
			curCLs:     clList{multiPartCLs[0], multiPartCLs[1]},
			expected:   []clList{clList{multiPartCLs[0], multiPartCLs[1]}},
		},
		// len(curCLs) == len(prevCLsMap).
		// And cl[6] has a larger patchset than multiPartCLs[4] with identical cl number.
		testCase{
			prevCLsMap: clRefMap{
				multiPartCLs[0].Ref: multiPartCLs[0],
				multiPartCLs[1].Ref: multiPartCLs[1],
			},
			curCLs:   clList{multiPartCLs[0], multiPartCLs[3]},
			expected: []clList{clList{multiPartCLs[0], multiPartCLs[3]}},
		},

		////////////////////////////////
		// Tests for mixed.
		testCase{
			prevCLsMap: clRefMap{
				multiPartCLs[0].Ref: multiPartCLs[0],
				multiPartCLs[1].Ref: multiPartCLs[1],
			},
			curCLs: clList{nonMultiPartCLs[0], multiPartCLs[0], multiPartCLs[3]},
			expected: []clList{
				clList{nonMultiPartCLs[0]},
				clList{multiPartCLs[0], multiPartCLs[3]},
			},
		},
	}

	for index, test := range testCases {
		got := newOpenCLs(ctx, test.prevCLsMap, test.curCLs)
		if !reflect.DeepEqual(test.expected, got) {
			t.Fatalf("case %d: want: %v, got: %v", index, test.expected, got)
		}
	}
}

func TestSendCLListsToPresubmitTest(t *testing.T) {
	clLists := []clList{
		clList{
			genCL(1000, 1, "veyron.js"),
		},
		clList{
			genMultiPartCL(1001, 1, "veyron.js", "t", 1, 2),
			genMultiPartCL(1002, 1, "veyron.go.core", "t", 2, 2),
		},
	}
	var buf bytes.Buffer
	ctx := util.NewContext(nil, os.Stdin, &buf, &buf, false, false, false)
	numSentCLs := sendCLListsToPresubmitTest(ctx, clLists, nil,
		// Mock out the removeOutdatedBuilds function.
		func(ctx *util.Context, cls clNumberToPatchsetMap) []error { return nil },

		// Mock out the addPresubmitTestBuild function.
		// It will return error for the first clList.
		func(cls clList) error {
			if reflect.DeepEqual(cls, clLists[0]) {
				return fmt.Errorf("err\n")
			} else {
				return nil
			}
		},
	)

	// Check output and return value.
	expectedOutput := `[VEYRON PRESUBMIT] FAIL: Add http://go/vcl/1000/1
[VEYRON PRESUBMIT] addPresubmitTestBuild([{Ref:refs/changes/xx/1000/1 Repo:veyron.js ChangeID: MultiPart:<nil>}]) failed: err
[VEYRON PRESUBMIT] PASS: Add http://go/vcl/1001/1, http://go/vcl/1002/1
`
	if got := buf.String(); expectedOutput != got {
		t.Fatalf("output: want:\n%v\n, got:\n%v", expectedOutput, got)
	}
	if expected := 2; expected != numSentCLs {
		t.Fatalf("numSentCLs: want %d, got %d", expected, numSentCLs)
	}
}

func TestQueuedOutdatedBuilds(t *testing.T) {
	response := `
{
	"items" : [
	  {
			"id": 10,
			"params": "\nREPOS=veyron.js veyron.go.core\nREFS=refs/changes/78/4778/1:refs/changes/50/4750/2",
			"task" : {
				"name": "veyron-presubmit-test"
			}
		},
	  {
			"id": 20,
			"params": "\nREPOS=veyron.js\nREFS=refs/changes/99/4799/2",
			"task" : {
				"name": "veyron-presubmit-test"
			}
		},
	  {
			"id": 30,
			"task" : {
				"name": "veyron-go-test"
			}
		}
	]
}
	`
	type testCase struct {
		cls      clNumberToPatchsetMap
		expected []queuedItem
	}
	testCases := []testCase{
		// A single matching CL with larger patchset.
		testCase{
			cls: clNumberToPatchsetMap{4799: 3},
			expected: []queuedItem{queuedItem{
				id:  20,
				ref: "refs/changes/99/4799/2",
			}},
		},
		// A single matching CL with equal patchset.
		testCase{
			cls: clNumberToPatchsetMap{4799: 2},
			expected: []queuedItem{queuedItem{
				id:  20,
				ref: "refs/changes/99/4799/2",
			}},
		},
		// A single matching CL with smaller patchset.
		testCase{
			cls:      clNumberToPatchsetMap{4799: 1},
			expected: []queuedItem{},
		},
		// Non-matching cl.
		testCase{
			cls:      clNumberToPatchsetMap{1234: 1},
			expected: []queuedItem{},
		},
		// Matching multi part CLs, with one equal patchset and one smaller patch set.
		testCase{
			cls:      clNumberToPatchsetMap{4778: 1, 4750: 1},
			expected: []queuedItem{},
		},
		// Matching multi part CLs, with equal patchset for both
		testCase{
			cls: clNumberToPatchsetMap{4778: 1, 4750: 2},
			expected: []queuedItem{queuedItem{
				id:  10,
				ref: "refs/changes/78/4778/1:refs/changes/50/4750/2",
			}},
		},
		// Matching multi part CLs, with larger patchset for both
		testCase{
			cls: clNumberToPatchsetMap{4778: 3, 4750: 4},
			expected: []queuedItem{queuedItem{
				id:  10,
				ref: "refs/changes/78/4778/1:refs/changes/50/4750/2",
			}},
		},
		// Try to match multi part CLs, but one cl number doesn't match.
		testCase{
			cls:      clNumberToPatchsetMap{4778: 3, 4751: 4},
			expected: []queuedItem{},
		},
	}
	for _, test := range testCases {
		got, errs := queuedOutdatedBuilds(strings.NewReader(response), test.cls)
		if len(errs) != 0 {
			t.Fatalf("want no errors, got: %v", errs)
		}
		if !reflect.DeepEqual(test.expected, got) {
			t.Fatalf("want %v, got %v", test.expected, got)
		}
	}
}

func TestOngoingOutdatedBuilds(t *testing.T) {
	nonMultiPartResponse := `
	{
		"actions": [
			{
				"parameters": [
				  {
						"name": "REPOS",
						"value": "veyron.go.core"
					},
					{
						"name": "REFS",
						"value": "refs/changes/96/5396/3"
					}
				]
			}
		],
		"building": true,
		"number": 1234
	}
	`
	multiPartResponse := `
	{
		"actions": [
			{
				"parameters": [
				  {
						"name": "REPOS",
						"value": "veyron.js veyron.go.core"
					},
					{
						"name": "REFS",
						"value": "refs/changes/00/5400/2:refs/changes/96/5396/3"
					}
				]
			}
		],
		"building": true,
		"number": 2014
	}
	`
	type testCase struct {
		cls      clNumberToPatchsetMap
		input    string
		expected ongoingBuild
	}
	nonMultiPartBuild := ongoingBuild{
		buildNumber: 1234,
		ref:         "refs/changes/96/5396/3",
	}
	multiPartBuild := ongoingBuild{
		buildNumber: 2014,
		ref:         "refs/changes/00/5400/2:refs/changes/96/5396/3",
	}
	invalidBuild := ongoingBuild{buildNumber: -1}
	testCases := []testCase{
		// A single matching CL with larger patchset.
		testCase{
			cls:      clNumberToPatchsetMap{5396: 4},
			input:    nonMultiPartResponse,
			expected: nonMultiPartBuild,
		},
		// A single matching CL with equal patchset.
		testCase{
			cls:      clNumberToPatchsetMap{5396: 3},
			input:    nonMultiPartResponse,
			expected: nonMultiPartBuild,
		},
		// A single matching CL with smaller patchset.
		testCase{
			cls:      clNumberToPatchsetMap{5396: 2},
			input:    nonMultiPartResponse,
			expected: invalidBuild,
		},
		// Non-matching CL.
		testCase{
			cls:      clNumberToPatchsetMap{1999: 2},
			input:    nonMultiPartResponse,
			expected: invalidBuild,
		},
		// Matching multi part CLs, with one equal patchset and one smaller patch set.
		testCase{
			cls:      clNumberToPatchsetMap{5400: 2, 5396: 2},
			input:    multiPartResponse,
			expected: invalidBuild,
		},
		// Matching multi part CLs, with equal patchset for both
		testCase{
			cls:      clNumberToPatchsetMap{5400: 2, 5396: 3},
			input:    multiPartResponse,
			expected: multiPartBuild,
		},
		// Matching multi part CLs, with larger patchset for both
		testCase{
			cls:      clNumberToPatchsetMap{5400: 3, 5396: 4},
			input:    multiPartResponse,
			expected: multiPartBuild,
		},
		// Try to match multi part CLs, but one cl number doesn't match.
		testCase{
			cls:      clNumberToPatchsetMap{5400: 3, 8399: 4},
			input:    multiPartResponse,
			expected: invalidBuild,
		},
	}
	for _, test := range testCases {
		got, err := ongoingOutdatedBuild(strings.NewReader(test.input), test.cls)
		if err != nil {
			t.Fatalf("want no errors, got: %v", err)
		}
		if !reflect.DeepEqual(test.expected, got) {
			t.Fatalf("want %v, got %v", test.expected, got)
		}
	}
}

func TestIsBuildOutdated(t *testing.T) {
	type testCase struct {
		refs     string
		cls      clNumberToPatchsetMap
		outdated bool
	}
	testCases := []testCase{
		// Builds with a single ref.
		testCase{
			refs:     "refs/changes/10/1000/2",
			cls:      clNumberToPatchsetMap{1000: 2},
			outdated: true,
		},
		testCase{
			refs:     "refs/changes/10/1000/2",
			cls:      clNumberToPatchsetMap{1000: 1},
			outdated: false,
		},

		// Builds with multiple refs.
		//
		// One of the cl numbers doesn't match.
		testCase{
			refs:     "refs/changes/10/1000/2:refs/changes/10/2000/2",
			cls:      clNumberToPatchsetMap{1001: 2, 2000: 2},
			outdated: false,
		},
		// Both patchsets in "cls" are smaller.
		testCase{
			refs:     "refs/changes/10/1000/2:refs/changes/10/2000/2",
			cls:      clNumberToPatchsetMap{1000: 1, 2000: 1},
			outdated: false,
		},
		// One of the patchsets in "cls" is larger than the one in "refs".
		testCase{
			refs:     "refs/changes/10/1000/2:refs/changes/10/2000/2",
			cls:      clNumberToPatchsetMap{1000: 3, 2000: 2},
			outdated: true,
		},
		// Both patchsets in "cls" are the same as the ones in "refs".
		testCase{
			refs:     "refs/changes/10/1000/2:refs/changes/10/2000/2",
			cls:      clNumberToPatchsetMap{1000: 2, 2000: 2},
			outdated: true,
		},
	}

	for _, test := range testCases {
		outdated, err := isBuildOutdated(test.refs, test.cls)
		if err != nil {
			t.Fatalf("want no errors, got: %v", err)
		}
		if expected, got := test.outdated, outdated; expected != got {
			t.Fatalf("want %v, got %v", expected, got)
		}
	}
}

func TestParseRefString(t *testing.T) {
	type testCase struct {
		ref              string
		expectErr        bool
		expectedCL       int
		expectedPatchSet int
	}
	testCases := []testCase{
		// Normal case
		testCase{
			ref:              "ref/changes/12/3412/2",
			expectedCL:       3412,
			expectedPatchSet: 2,
		},
		// Error cases
		testCase{
			ref:       "ref/123",
			expectErr: true,
		},
		testCase{
			ref:       "ref/changes/12/a/2",
			expectErr: true,
		},
		testCase{
			ref:       "ref/changes/12/3412/a",
			expectErr: true,
		},
	}
	for _, test := range testCases {
		cl, patchset, err := parseRefString(test.ref)
		if test.expectErr {
			if err == nil {
				t.Fatalf("want errors, got: %v", err)
			}
		} else {
			if err != nil {
				t.Fatalf("want no errors, got: %v", err)
			}
			if cl != test.expectedCL {
				t.Fatalf("want %v, got %v", test.expectedCL, cl)
			}
			if patchset != test.expectedPatchSet {
				t.Fatalf("want %v, got %v", test.expectedPatchSet, patchset)
			}
		}
	}
}

func genCL(clNumber, patchset int, repo string) gerrit.QueryResult {
	return gerrit.QueryResult{
		Ref:      fmt.Sprintf("refs/changes/xx/%d/%d", clNumber, patchset),
		Repo:     repo,
		ChangeID: "",
	}
}

func genMultiPartCL(clNumber, patchset int, repo, topic string, index, total int) gerrit.QueryResult {
	return gerrit.QueryResult{
		Ref:      fmt.Sprintf("refs/changes/xx/%d/%d", clNumber, patchset),
		Repo:     repo,
		ChangeID: "",
		MultiPart: &gerrit.MultiPartCLInfo{
			Topic: topic,
			Index: index,
			Total: total,
		},
	}
}
