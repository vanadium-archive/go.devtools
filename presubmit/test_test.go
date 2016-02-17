// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"v.io/jiri/gitutil"
	"v.io/jiri/jiritest"
	"v.io/jiri/project"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/xunit"
)

func TestParseCLs(t *testing.T) {
	type testCase struct {
		refs        string
		projects    string
		expectErr   bool
		expectedCLs []cl
	}
	testCases := []testCase{
		// Single ref and project.
		testCase{
			refs:      "refs/changes/10/1000/1",
			projects:  "release.go.core",
			expectErr: false,
			expectedCLs: []cl{
				cl{
					clNumber: 1000,
					patchset: 1,
					ref:      "refs/changes/10/1000/1",
					project:  "release.go.core",
				},
			},
		},

		// Multiple refs and projects.
		testCase{
			refs:      "refs/changes/10/1000/1:refs/changes/20/1020/1",
			projects:  "release.go.core:release.js.core",
			expectErr: false,
			expectedCLs: []cl{
				cl{
					clNumber: 1000,
					patchset: 1,
					ref:      "refs/changes/10/1000/1",
					project:  "release.go.core",
				},
				cl{
					clNumber: 1020,
					patchset: 1,
					ref:      "refs/changes/20/1020/1",
					project:  "release.js.core",
				},
			},
		},

		// len(refs) != len(project)
		testCase{
			refs:      "refs/changes/10/1000/1:refs/changes/20/1020/1",
			projects:  "release.go.core",
			expectErr: true,
		},
	}

	for _, test := range testCases {
		reviewTargetRefsFlag = test.refs
		projectsFlag = test.projects
		gotCLs, err := parseCLs()
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
		}
	}
}

// xTestPresubmitTest is an end-to-end test for the "test" phase of presubmit.
// It follows the steps below:
//
// 1. Create a fake JIRI_ROOT.
// 2. Create two CLs in two different projects. Each CL contains a new file with
//    some specific content. Push those CLs to the remote of the fake JIRI_ROOT.
// 3. Run the "test-presubmit-test" presubmit test againest those two CLs. The
//    "test-presubmit-test" is set up in jiri-test/internal/test/presubmit.go.
//    It will read the files in the CLs and check their contents. The file names
//    and the expected contents are passed to presubmit through a set of global
//    vars.
// 4. After the test is done, we verify the test status file and the test report
//    file.
// 5. We run through step 2-4 twice, one for a passed presubmit test, and one
//    for a failed presubmit test.
//
// TODO(jingjin): enable this test when related changes in presubmit are
// submitted.
func xTestPresubmitTest(t *testing.T) {
	fake, _ := jiritest.NewFakeJiriRoot(t)

	// Set WORKSPACE to JIRI_ROOT.
	// The test status file and the report file will be written to WORKSPACE.
	if err := os.Setenv("WORKSPACE", fake.X.Root); err != nil {
		t.Fatalf("os.Setenv() failed: %v", err)
	}

	// Prepare two CLs in two different projects, and push them to remote.
	project1, branch1, file1, content1 := "p1", "refs/changes/45/12345/1", "f1", "content1"
	project2, branch2, file2, content2 := "p2", "refs/changes/90/67890/1", "f2", "content2"
	if err := createCLInProject(fake, project1, file1, content1, branch1); err != nil {
		t.Fatalf("%v", err)
	}
	if err := createCLInProject(fake, project2, file2, content2, branch2); err != nil {
		t.Fatalf("%v", err)
	}

	// Run presubmit for those two CLs with the correct contents.
	// We expect the presubmit test will pass.
	projectsFlag = fmt.Sprintf("%s:%s", project1, project2)
	reviewTargetRefsFlag = fmt.Sprintf("%s:%s", branch1, branch2)
	testFlag = "test-presubmit-test"
	testMode = true
	testFilePaths = fmt.Sprintf("%s:%s",
		filepath.Join(fake.X.Root, project1, file1),
		filepath.Join(fake.X.Root, project2, file2))
	testFileExpectedContents = fmt.Sprintf("%s:%s", content1, content2)
	if err := runTest(fake.X, []string{}); err != nil {
		t.Fatalf("%v", err)
	}

	// Make sure we got a test status file with "passed" as the result.
	statusFile := filepath.Join(fake.X.Root, "status_test_presubmit_test.json")
	if err := checkStatusFile(statusFile, test.Passed); err != nil {
		t.Fatalf("%v", err)
	}

	// Make sure we got a test report file without failed test cases.
	reportFile := filepath.Join(fake.X.Root, "tests_dummy.xml")
	if err := checkReportfile(reportFile, 0); err != nil {
		t.Fatalf("%v", err)
	}

	// Run presubmit again with the incorrect contents.
	// We expect the presubmit test will fail.
	os.RemoveAll(statusFile)
	os.RemoveAll(reportFile)
	testFileExpectedContents = "c1:c2"
	if err := runTest(fake.X, []string{}); err != nil {
		t.Fatalf("%v", err)
	}

	// Make sure we got a test status file with "failed" as the result.
	if err := checkStatusFile(statusFile, test.Failed); err != nil {
		t.Fatalf("%v", err)
	}
	// Make sure we got a test report file with 1 fail test cases.
	reportFile = filepath.Join(fake.X.Root, "tests_test_presubmit_test.xml")
	if err := checkReportfile(reportFile, 1); err != nil {
		t.Fatalf("%v", err)
	}
}

func createCLInProject(fake *jiritest.FakeJiriRoot, projectName, fileName, fileContent, branch string) error {
	// Create the remote project and add it to the manifest.
	if err := fake.CreateRemoteProject(projectName); err != nil {
		return err
	}
	if err := fake.AddProject(project.Project{
		Name:   projectName,
		Path:   projectName,
		Remote: fake.Projects[projectName],
	}); err != nil {
		return err
	}
	if err := fake.UpdateUniverse(false); err != nil {
		return err
	}

	// Create a CL locally with a new file, and push it to the given branch
	// in remote.
	projectPath := filepath.Join(fake.X.Root, projectName)
	rootDirOpt := gitutil.RootDirOpt(projectPath)
	s := fake.X.NewSeq()
	g := gitutil.New(s, rootDirOpt)
	if err := g.CreateAndCheckoutBranch(branch); err != nil {
		return err
	}
	testFile := filepath.Join(projectPath, fileName)
	if err := ioutil.WriteFile(testFile, []byte(fileContent), 0644); err != nil {
		return err
	}
	if err := g.Add(testFile); err != nil {
		return err
	}
	if err := g.CommitWithMessage("check in a new file"); err != nil {
		return err
	}
	if err := g.Push("origin", branch, gitutil.VerifyOpt(false)); err != nil {
		return err
	}
	if err := g.CheckoutBranch("master"); err != nil {
		return err
	}
	if err := g.DeleteBranch(branch, gitutil.ForceOpt(true)); err != nil {
		return err
	}

	return nil
}

func checkStatusFile(statusFile string, expectedStatus test.Status) error {
	bytes, err := ioutil.ReadFile(statusFile)
	if err != nil {
		return fmt.Errorf("ReadFile(%v) failed: %v", statusFile, err)
	}
	var result testResultInfo
	if err := json.Unmarshal(bytes, &result); err != nil {
		return fmt.Errorf("Unmarshal() failed: %v", err)
	}
	if result.Result.Status != expectedStatus {
		return fmt.Errorf("test status: want %s, got %s", expectedStatus, result.Result.Status)
	}
	return nil
}

func checkReportfile(reportFile string, expectedFailures int) error {
	bytes, err := ioutil.ReadFile(reportFile)
	if err != nil {
		return fmt.Errorf("ReadFile(%v) failed: %v", reportFile, err)
	}
	suites := xunit.TestSuites{}
	if err := xml.Unmarshal(bytes, &suites); err != nil {
		return fmt.Errorf("Unmarshal(%v) failed: %v", string(bytes), err)
	}
	failures := 0
	for _, suite := range suites.Suites {
		failures += suite.Failures
	}
	if failures != expectedFailures {
		return fmt.Errorf("failure count: want %v, got %v", expectedFailures, failures)
	}
	return nil
}
