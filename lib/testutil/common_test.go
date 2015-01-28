package testutil

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"v.io/tools/lib/runutil"
	"v.io/tools/lib/util"
)

func TestGenXUnitReportOnCmdError(t *testing.T) {
	ctx := util.DefaultContext()

	// Set WORKSPACE to a tmp dir.
	workspaceDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer ctx.Run().RemoveAll(workspaceDir)
	oldWorkspaceDir := os.Getenv("WORKSPACE")
	if err := os.Setenv("WORKSPACE", workspaceDir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("WORKSPACE", oldWorkspaceDir)

	// Create a script in workspace.
	scriptFile := filepath.Join(workspaceDir, "test.sh")
	fileContent := `#!/bin/bash
echo "ok: test 1"
echo "fail: test 2"
exit 1`
	if err := ioutil.WriteFile(scriptFile, []byte(fileContent), 0755); err != nil {
		t.Fatalf("WriteFile() failed: %v", err)
	}

	expectedGenSuite := testSuite{
		Name: "vanadium-go-test",
		Cases: []testCase{
			testCase{
				Name:      "build",
				Classname: "vanadium-go-test",
				Failures: []testFailure{
					testFailure{
						Message: "failure",
						Data:    "Error message:\nexit status 1\n\nConsole output:\n......\nok: test 1\nfail: test 2\n",
					},
				},
				Time: "0.00",
			},
		},
		Tests:    1,
		Failures: 1,
	}
	aFailedTestSuite := testSuite{
		Name: "name1",
		Cases: []testCase{
			testCase{
				Name:      "test1",
				Classname: "class1",
				Failures: []testFailure{
					testFailure{
						Message: "failure",
						Data:    "test failed",
					},
				},
				Time: "0.10",
			},
		},
		Tests:    1,
		Failures: 1,
	}

	// Tests.
	testCases := []struct {
		createXUnitFile bool
		existingSuites  *testSuites
		expectedSuites  *testSuites
	}{
		// No xUnit file exists.
		{
			createXUnitFile: false,
			expectedSuites: &testSuites{
				Suites: []testSuite{expectedGenSuite},
				XMLName: xml.Name{
					Local: "testsuites",
				},
			},
		},
		// xUnit file exists but empty (invalid).
		{
			createXUnitFile: true,
			existingSuites:  &testSuites{},
			expectedSuites: &testSuites{
				Suites: []testSuite{expectedGenSuite},
				XMLName: xml.Name{
					Local: "testsuites",
				},
			},
		},
		// xUnit file exists but doesn't contain failed test cases.
		{
			createXUnitFile: true,
			existingSuites: &testSuites{
				Suites: []testSuite{
					testSuite{
						Name: "name1",
						Cases: []testCase{
							testCase{
								Name:      "test1",
								Classname: "class1",
								Time:      "0.10",
							},
						},
						Tests:    1,
						Failures: 0,
					},
				},
			},
			expectedSuites: &testSuites{
				Suites: []testSuite{expectedGenSuite},
				XMLName: xml.Name{
					Local: "testsuites",
				},
			},
		},
		// xUnit file exists and contains failed test cases.
		{
			createXUnitFile: true,
			existingSuites: &testSuites{
				Suites: []testSuite{aFailedTestSuite},
			},
			expectedSuites: &testSuites{
				Suites: []testSuite{aFailedTestSuite},
				XMLName: xml.Name{
					Local: "testsuites",
				},
			},
		},
	}

	xUnitFileName := XUnitReportPath("vanadium-go-test")
	for _, test := range testCases {
		if err := os.RemoveAll(xUnitFileName); err != nil {
			t.Fatalf("RemoveAll(%s) failed: %v", xUnitFileName, err)
		}
		if test.createXUnitFile && test.existingSuites != nil {
			bytes, err := xml.MarshalIndent(test.existingSuites, "", "  ")
			if err != nil {
				t.Fatalf("MarshalIndent(%v) failed: %v", test.existingSuites, err)
			}
			if err := ioutil.WriteFile(xUnitFileName, bytes, os.FileMode(0644)); err != nil {
				t.Fatalf("WriteFile(%v) failed: %v", xUnitFileName, err)
			}
		}
		testResult, err := genXUnitReportOnCmdError(ctx, "vanadium-go-test", "build", "failure",
			func(opts runutil.Opts) error {
				return ctx.Run().CommandWithOpts(opts, scriptFile)
			})
		if err != nil {
			t.Fatalf("want no errors, got %v", err)
		}
		gotSuites, err := parseXUnitFile(xUnitFileName)
		if err != nil {
			t.Fatalf("%v", err)
		}
		if !reflect.DeepEqual(gotSuites, test.expectedSuites) {
			t.Fatalf("want\n%#v\n\ngot\n%#v", test.expectedSuites, gotSuites)
		}
		if got, expected := testResult.Status, TestFailed; got != expected {
			t.Fatalf("want %v, got %v", expected, got)
		}
	}
}

func parseXUnitFile(fileName string) (*testSuites, error) {
	bytes, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, fmt.Errorf("ReadFile(%s) failed: %v", fileName, err)
	}
	var s testSuites
	if err := xml.Unmarshal(bytes, &s); err != nil {
		return nil, fmt.Errorf("Unmarshal() failed: %v\n%v", err, string(bytes))
	}
	return &s, nil
}
