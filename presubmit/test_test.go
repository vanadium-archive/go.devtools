package main

import (
	"reflect"
	"strings"
	"testing"

	"v.io/tools/lib/util"
)

func TestParseRefsAndRepos(t *testing.T) {
	type testCase struct {
		refs          string
		repos         string
		expectErr     bool
		expectedCLs   []cl
		expectedRefs  []string
		expectedRepos []string
	}
	testCases := []testCase{
		// Single ref and repo.
		testCase{
			refs:      "refs/changes/10/1000/1",
			repos:     "release.go.core",
			expectErr: false,
			expectedCLs: []cl{
				cl{
					clNumber: 1000,
					patchset: 1,
					ref:      "refs/changes/10/1000/1",
					repo:     "https://vanadium.googlesource.com/release.go.core",
				},
			},
			expectedRefs:  []string{"refs/changes/10/1000/1"},
			expectedRepos: []string{"https://vanadium.googlesource.com/release.go.core"},
		},

		// Multiple refs and repos.
		testCase{
			refs:      "refs/changes/10/1000/1:refs/changes/20/1020/1",
			repos:     "release.go.core:release.js.core",
			expectErr: false,
			expectedCLs: []cl{
				cl{
					clNumber: 1000,
					patchset: 1,
					ref:      "refs/changes/10/1000/1",
					repo:     "https://vanadium.googlesource.com/release.go.core",
				},
				cl{
					clNumber: 1020,
					patchset: 1,
					ref:      "refs/changes/20/1020/1",
					repo:     "https://vanadium.googlesource.com/release.js.core",
				},
			},
			expectedRefs: []string{"refs/changes/10/1000/1", "refs/changes/20/1020/1"},
			expectedRepos: []string{
				"https://vanadium.googlesource.com/release.go.core",
				"https://vanadium.googlesource.com/release.js.core",
			},
		},

		// len(refs) != len(repos)
		testCase{
			refs:      "refs/changes/10/1000/1:refs/changes/20/1020/1",
			repos:     "release.go.core",
			expectErr: true,
		},
	}

	for _, test := range testCases {
		reviewTargetRefsFlag = test.refs
		reposFlag = test.repos
		gotCLs, gotRefs, gotRepos, err := parseRefsAndRepos()
		if test.expectErr && err == nil {
			t.Fatalf("want errors, got no errors")

		}
		if !test.expectErr && err != nil {
			t.Fatalf("want no errors, got: %v", err)
		}
		if err == nil {
			if !reflect.DeepEqual(test.expectedCLs, gotCLs) {
				t.Fatalf("want %#v, got %#v", test.expectedCLs, gotCLs)
			}
			if !reflect.DeepEqual(test.expectedRefs, gotRefs) {
				t.Fatalf("want %#v, got %#v", test.expectedRefs, gotRefs)
			}
			if !reflect.DeepEqual(test.expectedRepos, gotRepos) {
				t.Fatalf("want %#v, got %#v", test.expectedRepos, gotRepos)
			}
		}
	}
}

func TestParseLastCompletedBuildStatusJsonResponse(t *testing.T) {
	// "SUCCESS" status.
	input := `
	{
		"building": false,
		"fullDisplayName": "vanadium-android-build #182",
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
		"fullDisplayName": "vanadium-android-build #182",
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

func TestGenFailedTestLinks(t *testing.T) {
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
		failedTestGetterResult []testCase
		expectedLinksMap       failedTestLinksMap
		expectedSeenTests      map[string]int
	}

	tests := []test{
		test{
			failedTestGetterResult: []testCase{},
			expectedLinksMap: failedTestLinksMap{
				newFailure: []string{
					"- c1::n::n1\nhttp://goto.google.com/vpst/10/testReport/c1/n/n1",
					"- c2::n::n2\nhttp://goto.google.com/vpst/10/testReport/c2/n/n2",
					"- go::vanadium::abc::n5\nhttp://goto.google.com/vpst/10/testReport/go.vanadium/abc/n5",
				},
			},
			expectedSeenTests: map[string]int{
				"c1::n::n1":             1,
				"c2::n::n2":             1,
				"c3::n::n3":             2,
				`ts1::"n9"`:             1,
				"go::vanadium::abc::n5": 1,
			},
		},
		test{
			failedTestGetterResult: []testCase{
				testCase{
					ClassName: "c1.n",
					Name:      "n1",
				},
				testCase{
					ClassName: "c4.n",
					Name:      "n4",
				},
			},
			expectedLinksMap: failedTestLinksMap{
				newFailure: []string{
					"- c2::n::n2\nhttp://goto.google.com/vpst/10/testReport/c2/n/n2",
					"- go::vanadium::abc::n5\nhttp://goto.google.com/vpst/10/testReport/go.vanadium/abc/n5",
				},
				knownFailure: []string{
					"- c1::n::n1\nhttp://goto.google.com/vpst/10/testReport/c1/n/n1",
				},
				fixedFailure: []string{
					"- c4::n::n4",
				},
			},
			expectedSeenTests: map[string]int{
				"c1::n::n1":             1,
				"c2::n::n2":             1,
				"c3::n::n3":             2,
				`ts1::"n9"`:             1,
				"go::vanadium::abc::n5": 1,
			},
		},
	}

	for _, curTest := range tests {
		seenTests := map[string]int{}
		failedTestGetterFn := func(string) ([]testCase, error) {
			return curTest.failedTestGetterResult, nil
		}
		linksMap, err := genFailedTestLinks(ctx, strings.NewReader(reportFileContent), seenTests, "veyron-go-test", failedTestGetterFn)
		if err != nil {
			t.Fatalf("want no errors, got: %v", err)
		}
		if !reflect.DeepEqual(curTest.expectedLinksMap, linksMap) {
			t.Fatalf("want %v, got %v", curTest.expectedLinksMap, linksMap)
		}
		if !reflect.DeepEqual(curTest.expectedSeenTests, seenTests) {
			t.Fatalf("want %v, got %v", curTest.expectedSeenTests, seenTests)
		}
	}
}

func TestGenTestResultLink(t *testing.T) {
	type testCase struct {
		className    string
		testName     string
		testFullName string
		suffix       int
		expectedLink string
	}

	jenkinsBuildNumberFlag = 10
	testCases := []testCase{
		testCase{
			className:    "c",
			testName:     "t",
			testFullName: "T1",
			suffix:       0,
			expectedLink: "- T1\nhttp://goto.google.com/vpst/10/testReport/%28root%29/c/t",
		},
		testCase{
			className:    "c n",
			testName:     "t",
			testFullName: "T1",
			suffix:       0,
			expectedLink: "- T1\nhttp://goto.google.com/vpst/10/testReport/%28root%29/c%20n/t",
		},
		testCase{
			className:    "c.n",
			testName:     "t",
			testFullName: "T1",
			suffix:       0,
			expectedLink: "- T1\nhttp://goto.google.com/vpst/10/testReport/c/n/t",
		},
		testCase{
			className:    "c.n",
			testName:     "t.n",
			testFullName: "T1",
			suffix:       0,
			expectedLink: "- T1\nhttp://goto.google.com/vpst/10/testReport/c/n/t_n",
		},
		testCase{
			className:    "c.n",
			testName:     "t.n",
			testFullName: "T1",
			suffix:       1,
			expectedLink: "- T1\nhttp://goto.google.com/vpst/10/testReport/c/n/t_n",
		},
		testCase{
			className:    "c.n",
			testName:     "t.n",
			testFullName: "T1",
			suffix:       2,
			expectedLink: "- T1\nhttp://goto.google.com/vpst/10/testReport/c/n/t_n_2",
		},
	}

	for _, test := range testCases {
		if got, expected := genTestResultLink(test.className, test.testName, test.testFullName, test.suffix), test.expectedLink; got != expected {
			t.Fatalf("want %v, got %v", expected, got)
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
