package main

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"v.io/x/devtools/internal/gerrit"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
)

func TestMultiPartCLSet(t *testing.T) {
	set := NewMultiPartCLSet()
	checkMultiPartCLSet(t, -1, map[int]gerrit.Change{}, set)

	// Add a non-multipart cl.
	cl := genCL(1000, 1, "relase.go.core")
	if err := set.addCL(cl); err == nil {
		t.Fatalf("expected addCL(%v) to fail and it did not", cl)
	}
	checkMultiPartCLSet(t, -1, map[int]gerrit.Change{}, set)

	// Add a multi part cl.
	cl.MultiPart = &gerrit.MultiPartCLInfo{
		Topic: "test",
		Index: 1,
		Total: 2,
	}
	if err := set.addCL(cl); err != nil {
		t.Fatalf("addCL(%v) failed: %v", cl, err)
	}
	checkMultiPartCLSet(t, 2, map[int]gerrit.Change{
		1: cl,
	}, set)

	// Test incomplete.
	if expected, got := false, set.complete(); expected != got {
		t.Fatalf("want %s, got %s", expected, got)
	}

	// Add another multi part cl with the wrong "Total" number.
	cl2 := genMultiPartCL(1050, 2, "release.js.core", "test", 2, 3)
	if err := set.addCL(cl2); err == nil {
		t.Fatalf("expected addCL(%v) to fail and it did not", cl)
	}
	checkMultiPartCLSet(t, 2, map[int]gerrit.Change{
		1: cl,
	}, set)

	// Add another multi part cl with duplicated "Index" number.
	cl3 := genMultiPartCL(1052, 2, "release.js.core", "Test", 1, 2)
	if err := set.addCL(cl3); err == nil {
		t.Fatalf("expected addCL(%v) to fail and it did not", cl)
	}
	checkMultiPartCLSet(t, 2, map[int]gerrit.Change{
		1: cl,
	}, set)

	// Add another multi part cl with the wrong "Topic".
	cl4 := genMultiPartCL(1062, 2, "release.js.core", "test123", 1, 2)
	if err := set.addCL(cl4); err == nil {
		t.Fatalf("expected addCL(%v) to fail and it did not", cl)
	}
	checkMultiPartCLSet(t, 2, map[int]gerrit.Change{
		1: cl,
	}, set)

	// Add a valid multi part cl.
	cl5 := genMultiPartCL(1072, 2, "release.js.core", "test", 2, 2)
	if err := set.addCL(cl5); err != nil {
		t.Fatalf("addCL(%v) failed: %v", cl, err)
	}
	checkMultiPartCLSet(t, 2, map[int]gerrit.Change{
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

func checkMultiPartCLSet(t *testing.T, expectedTotal int, expectedCLsByPart map[int]gerrit.Change, set *multiPartCLSet) {
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
machine vanadium.googlesource.com login git-jingjin.google.com password 12345
machine vanadium-review.googlesource.com login git-jingjin.google.com password 54321
	`
	got, err := parseNetRcFile(strings.NewReader(netrcFileContent))
	expected := map[string]credential{
		"vanadium.googlesource.com": credential{
			username: "git-jingjin.google.com",
			password: "12345",
		},
		"vanadium-review.googlesource.com": credential{
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
machine vanadium.googlesource.com login git-jingjin.google.com password
machine_blah vanadium3.googlesource.com login git-jingjin.google.com password 12345
machine vanadium2.googlesource.com login_blah git-jingjin.google.com password 12345
machine vanadium4.googlesource.com login git-jingjin.google.com password_blah 12345
machine vanadium-review.googlesource.com login git-jingjin.google.com password 54321
	`
	got, err := parseNetRcFile(strings.NewReader(netRcFileContentWithInvalidEntries))
	expected := map[string]credential{
		"vanadium-review.googlesource.com": credential{
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
	ctx := tool.NewDefaultContext()
	nonMultiPartCLs := clList{
		genCL(1010, 1, "release.go.core"),
		genCL(1020, 2, "release.go.tools"),
		genCL(1030, 3, "release.js.core"),

		genMultiPartCL(1000, 1, "release.js.core", "T1", 1, 2),
		genMultiPartCL(1001, 1, "release.go.core", "T1", 2, 2),
		genMultiPartCL(1002, 2, "release.go.core", "T2", 2, 2),
		genMultiPartCL(1001, 2, "release.go.core", "T1", 2, 2),
	}
	multiPartCLs := clList{
		// Multi part CLs.
		// The first two form a complete set for topic T1.
		// The third one looks like the second one, but has a different topic.
		// The last one has a larger patchset than the second one.
		genMultiPartCL(1000, 1, "release.js.core", "T1", 1, 2),
		genMultiPartCL(1001, 1, "release.go.core", "T1", 2, 2),
		genMultiPartCL(1002, 2, "release.go.core", "T2", 2, 2),
		genMultiPartCL(1001, 2, "release.go.core", "T1", 2, 2),
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
			prevCLsMap: clRefMap{nonMultiPartCLs[0].Reference(): nonMultiPartCLs[0]},
			curCLs:     clList{},
			expected:   []clList{},
		},
		// prevCLsMap and curCLs are not empty, and they have overlapping refs.
		testCase{
			prevCLsMap: clRefMap{
				nonMultiPartCLs[0].Reference(): nonMultiPartCLs[0],
				nonMultiPartCLs[1].Reference(): nonMultiPartCLs[1],
			},
			curCLs:   clList{nonMultiPartCLs[1], nonMultiPartCLs[2]},
			expected: []clList{clList{nonMultiPartCLs[2]}},
		},
		// prevCLsMap and curCLs are not empty, and they have NO overlapping refs.
		testCase{
			prevCLsMap: clRefMap{nonMultiPartCLs[0].Reference(): nonMultiPartCLs[0]},
			curCLs:     clList{nonMultiPartCLs[1]},
			expected:   []clList{clList{nonMultiPartCLs[1]}},
		},

		////////////////////////////////
		// Tests for multi part CLs.

		// len(curCLs) > len(prevCLsMap).
		// And the CLs in curCLs have different topics.
		testCase{
			prevCLsMap: clRefMap{multiPartCLs[0].Reference(): multiPartCLs[0]},
			curCLs:     clList{multiPartCLs[0], multiPartCLs[2]},
			expected:   []clList{},
		},
		// len(curCLs) > len(prevCLsMap).
		// And the CLs in curCLs form a complete multi part cls set.
		testCase{
			prevCLsMap: clRefMap{multiPartCLs[0].Reference(): multiPartCLs[0]},
			curCLs:     clList{multiPartCLs[0], multiPartCLs[1]},
			expected:   []clList{clList{multiPartCLs[0], multiPartCLs[1]}},
		},
		// len(curCLs) == len(prevCLsMap).
		// And cl[6] has a larger patchset than multiPartCLs[4] with identical cl number.
		testCase{
			prevCLsMap: clRefMap{
				multiPartCLs[0].Reference(): multiPartCLs[0],
				multiPartCLs[1].Reference(): multiPartCLs[1],
			},
			curCLs:   clList{multiPartCLs[0], multiPartCLs[3]},
			expected: []clList{clList{multiPartCLs[0], multiPartCLs[3]}},
		},

		////////////////////////////////
		// Tests for mixed.
		testCase{
			prevCLsMap: clRefMap{
				multiPartCLs[0].Reference(): multiPartCLs[0],
				multiPartCLs[1].Reference(): multiPartCLs[1],
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
			genCL(1000, 1, "release.js.core"),
		},
		clList{
			genCLWithMoreData(2000, 1, "release.js.core", gerrit.PresubmitTestTypeNone, "vj@google.com"),
		},
		clList{
			genCLWithMoreData(2010, 1, "release.js.core", gerrit.PresubmitTestTypeAll, "foo@bar.com"),
		},
		clList{
			genMultiPartCL(1001, 1, "release.js.core", "t", 1, 2),
			genMultiPartCL(1002, 1, "release.go.core", "t", 2, 2),
		},
		clList{
			genMultiPartCL(1003, 1, "release.js.core", "t", 1, 3),
			genMultiPartCL(1004, 1, "release.go.core", "t", 2, 3),
			genMultiPartCLWithMoreData(1005, 1, "release.go.core", "t", 3, 3, "foo@bar.com"),
		},
		clList{
			genCL(3000, 1, "non-existent-project"),
		},
		clList{
			genMultiPartCL(1005, 1, "release.js.core", "t", 1, 2),
			genMultiPartCL(1006, 1, "non-existent-project", "t", 2, 2),
		},
	}
	var buf bytes.Buffer
	ctx := tool.NewContext(tool.ContextOpts{
		Stdout: &buf,
		Stderr: &buf,
	})
	sender := clsSender{
		clLists: clLists,
		projects: map[string]util.Project{
			"release.go.core": util.Project{},
			"release.js.core": util.Project{},
		},

		// Mock out the removeOutdatedBuilds function.
		removeOutdatedFn: func(ctx *tool.Context, cls clNumberToPatchsetMap) []error { return nil },

		// Mock out the addPresubmitTestBuild function.
		// It will return error for the first clList.
		addPresubmitFn: func(ctx *tool.Context, cls clList, tests []string) error {
			if reflect.DeepEqual(cls, clLists[0]) {
				return fmt.Errorf("err")
			} else {
				return nil
			}
		},

		// Mock out postMessage function.
		postMessageFn: func(ctx *tool.Context, message string, refs []string, success bool) error { return nil },
	}
	if err := sender.sendCLListsToPresubmitTest(ctx); err != nil {
		t.Fatalf("want no error, got: %v", err)
	}

	// Check output and return value.
	expectedOutput := `[VANADIUM PRESUBMIT] FAIL: Add http://go/vcl/1000/1
[VANADIUM PRESUBMIT] addPresubmitTestBuild failed: err
[VANADIUM PRESUBMIT] SKIP: Add http://go/vcl/2000/1 (presubmit=none)
[VANADIUM PRESUBMIT] SKIP: Add http://go/vcl/2010/1 (non-google owner)
[VANADIUM PRESUBMIT] PASS: Add http://go/vcl/1001/1, http://go/vcl/1002/1
[VANADIUM PRESUBMIT] SKIP: Add http://go/vcl/1003/1, http://go/vcl/1004/1, http://go/vcl/1005/1 (non-google owner)
[VANADIUM PRESUBMIT] project="non-existent-project" (refs/changes/xx/3000/1) not found. Skipped.
[VANADIUM PRESUBMIT] SKIP: Empty CL set
[VANADIUM PRESUBMIT] project="non-existent-project" (refs/changes/xx/1006/1) not found. Skipped.
[VANADIUM PRESUBMIT] PASS: Add http://go/vcl/1005/1
`
	if got := buf.String(); expectedOutput != got {
		t.Fatalf("output: want:\n%v\n, got:\n%v", expectedOutput, got)
	}
	if expected := 3; expected != sender.clsSent {
		t.Fatalf("numSentCLs: want %d, got %d", expected, sender.clsSent)
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
		// Overlapping cls.
		testCase{
			refs:     "refs/changes/10/1001/2",
			cls:      clNumberToPatchsetMap{1001: 3, 2000: 2},
			outdated: true,
		},
		// The other case with overlapping cl.
		testCase{
			refs:     "refs/changes/10/1000/2:refs/changes/10/2000/2",
			cls:      clNumberToPatchsetMap{1001: 2, 2000: 2},
			outdated: true,
		},
		// Both refs don't match.
		testCase{
			refs:     "refs/changes/10/1000/2:refs/changes/10/2000/2",
			cls:      clNumberToPatchsetMap{1001: 2, 2000: 2},
			outdated: true,
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

	for i, test := range testCases {
		outdated, err := isBuildOutdated(test.refs, test.cls)
		if err != nil {
			t.Fatalf("want no errors, got: %v", err)
		}
		if expected, got := test.outdated, outdated; expected != got {
			t.Fatalf("%d: want %v, got %v", i, expected, got)
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

func genCL(clNumber, patchset int, project string) gerrit.Change {
	return genCLWithMoreData(clNumber, patchset, project, gerrit.PresubmitTestTypeAll, "vj@google.com")
}

func genCLWithMoreData(clNumber, patchset int, project string, presubmit gerrit.PresubmitTestType, ownerEmail string) gerrit.Change {
	change := gerrit.Change{
		Current_revision: "r",
		Revisions: gerrit.Revisions{
			"r": gerrit.Revision{
				Fetch: gerrit.Fetch{
					Http: gerrit.Http{
						Ref: fmt.Sprintf("refs/changes/xx/%d/%d", clNumber, patchset),
					},
				},
			},
		},
		Project:       project,
		Change_id:     "",
		PresubmitTest: presubmit,
		Owner: gerrit.Owner{
			Email: ownerEmail,
		},
	}
	return change
}

func genMultiPartCL(clNumber, patchset int, project, topic string, index, total int) gerrit.Change {
	return genMultiPartCLWithMoreData(clNumber, patchset, project, topic, index, total, "vj@google.com")
}

func genMultiPartCLWithMoreData(clNumber, patchset int, project, topic string, index, total int, ownerEmail string) gerrit.Change {
	return gerrit.Change{
		Current_revision: "r",
		Revisions: gerrit.Revisions{
			"r": gerrit.Revision{
				Fetch: gerrit.Fetch{
					Http: gerrit.Http{
						Ref: fmt.Sprintf("refs/changes/xx/%d/%d", clNumber, patchset),
					},
				},
			},
		},
		Project:   project,
		Change_id: "",
		Owner: gerrit.Owner{
			Email: ownerEmail,
		},
		MultiPart: &gerrit.MultiPartCLInfo{
			Topic: topic,
			Index: index,
			Total: total,
		},
	}
}
