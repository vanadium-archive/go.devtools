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

	// No xUnit file exists.
	testResult, err := genXUnitReportOnCmdError(ctx, "vanadium-go-test", "build", "failure",
		func(opts runutil.Opts) error {
			return ctx.Run().CommandWithOpts(opts, scriptFile)
		})
	if err != nil {
		t.Fatalf("want no errors, got %v", err)
	}
	xUnitFileName := XUnitReportPath("vanadium-go-test")
	gotSuites, err := parseXUnitFile(xUnitFileName)
	if err != nil {
		t.Fatalf("%v", err)
	}
	expectedSuites := &testSuites{
		Suites: []testSuite{
			testSuite{
				Name: "vanadium-go-test",
				Cases: []testCase{
					testCase{
						Name:      "build",
						Classname: "vanadium-go-test",
						Failures: []testFailure{
							testFailure{
								Message: "failure",
								Data:    "......\nok: test 1\nfail: test 2\n",
							},
						},
						Time: "0.00",
					},
				},
				Tests:    1,
				Failures: 1,
			},
		},
		XMLName: xml.Name{
			Local: "testsuites",
		},
	}
	if !reflect.DeepEqual(gotSuites, expectedSuites) {
		t.Fatalf("want\n%#v\n\ngot\n%#v", expectedSuites, gotSuites)
	}
	if got, expected := testResult.Status, TestFailed; got != expected {
		t.Fatalf("want %v, got %v", expected, got)
	}

	// There is already an xUnit file there.
	if err := os.RemoveAll(xUnitFileName); err != nil {
		t.Fatalf("RemoveAll(%s) failed: %v", xUnitFileName, err)
	}
	existingSuites := testSuites{
		Suites: []testSuite{
			testSuite{
				Name: "vanadium-go-test",
				Cases: []testCase{
					testCase{
						Name:      "test1",
						Classname: "vanadium-go-test",
						Time:      "1.00",
					},
				},
				Tests: 1,
			},
		},
	}
	bytes, err := xml.MarshalIndent(existingSuites, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() failed: %v", err)
	}
	if err := ioutil.WriteFile(xUnitFileName, bytes, 0644); err != nil {
		t.Fatalf("WriteFile(%s) failed: %v", xUnitFileName, err)
	}
	testResult, err = genXUnitReportOnCmdError(ctx, "vanadium-go-test", "build", "failure",
		func(opts runutil.Opts) error {
			return ctx.Run().CommandWithOpts(opts, scriptFile)
		})
	if err != nil {
		t.Fatalf("want no errors, got %v", err)
	}
	gotSuites, err = parseXUnitFile(xUnitFileName)
	if err != nil {
		t.Fatalf("%v", err)
	}
	expectedSuites = &testSuites{
		Suites: []testSuite{
			expectedSuites.Suites[0],
			existingSuites.Suites[0],
		},
		XMLName: xml.Name{
			Local: "testsuites",
		},
	}
	if !reflect.DeepEqual(gotSuites, expectedSuites) {
		t.Fatalf("want\n%#v\n\ngot\n%#v", expectedSuites, gotSuites)
	}
	if got, expected := testResult.Status, TestFailed; got != expected {
		t.Fatalf("want %v, got %v", expected, got)
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
