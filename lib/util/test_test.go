package util

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestProjectTests(t *testing.T) {
	projects := map[string][]string{
		"veyron":  []string{"veyron-go-build", "veyron-go-test"},
		"default": []string{"tools-go-build", "tools-go-test"},
	}
	ctx := DefaultContext()

	// Get tests for a repo that is in the config file.
	got, err := projectTests(ctx, projects, "veyron")
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
	got, err = projectTests(ctx, projects, "non-exist-repo")
	expected = nil
	if err != nil {
		t.Fatalf("want no errors, got: %v", err)
	}
	if !reflect.DeepEqual(expected, got) {
		t.Errorf("want: %#v, got: %#v", expected, got)
	}
}

func TestCreateDepGraph(t *testing.T) {
	type testCase struct {
		dep           map[string][]string
		tests         []string
		expectedTests testDepGraph
		expectDepLoop bool
	}
	testCases := []testCase{
		// A single test without any dependencies.
		testCase{
			dep: map[string][]string{
				"A": []string{},
			},
			tests: []string{"A"},
			expectedTests: testDepGraph{
				"A": &testNode{
					deps:    []string{},
					visited: true,
				},
			},
		},
		// A -> B
		testCase{
			dep: map[string][]string{
				"A": []string{"B"},
			},
			tests: []string{"A", "B"},
			expectedTests: testDepGraph{
				"A": &testNode{
					deps:    []string{"B"},
					visited: true,
				},
				"B": &testNode{
					deps:    []string{},
					visited: true,
				},
			},
		},
		// A -> {B, C, D}
		testCase{
			dep: map[string][]string{
				"A": []string{"B", "C", "D"},
			},
			tests: []string{"A", "B", "C", "D"},
			expectedTests: testDepGraph{
				"A": &testNode{
					deps:    []string{"B", "C", "D"},
					visited: true,
				},
				"B": &testNode{
					deps:    []string{},
					visited: true,
				},
				"C": &testNode{
					deps:    []string{},
					visited: true,
				},
				"D": &testNode{
					deps:    []string{},
					visited: true,
				},
			},
		},
		// Same as above, but "dep" has no data.
		testCase{
			tests: []string{"A", "B", "C", "D"},
			expectedTests: testDepGraph{
				"A": &testNode{
					deps:    []string{},
					visited: true,
				},
				"B": &testNode{
					deps:    []string{},
					visited: true,
				},
				"C": &testNode{
					deps:    []string{},
					visited: true,
				},
				"D": &testNode{
					deps:    []string{},
					visited: true,
				},
			},
		},
		// A -> {B, C, D}, but A is the only given test to resolve dependency for.
		testCase{
			dep: map[string][]string{
				"A": []string{"B", "C", "D"},
			},
			tests: []string{"A"},
			expectedTests: testDepGraph{
				"A": &testNode{
					deps:    []string{},
					visited: true,
				},
			},
		},
		// A -> {B, C, D} -> E
		testCase{
			dep: map[string][]string{
				"A": []string{"B", "C", "D"},
				"B": []string{"E"},
				"C": []string{"E"},
				"D": []string{"E"},
			},
			tests: []string{"A", "B", "C", "D", "E"},
			expectedTests: testDepGraph{
				"A": &testNode{
					deps:    []string{"B", "C", "D"},
					visited: true,
				},
				"B": &testNode{
					deps:    []string{"E"},
					visited: true,
				},
				"C": &testNode{
					deps:    []string{"E"},
					visited: true,
				},
				"D": &testNode{
					deps:    []string{"E"},
					visited: true,
				},
				"E": &testNode{
					deps:    []string{},
					visited: true,
				},
			},
		},
		// Dependency loop:
		// A -> B
		// B -> C, C -> B
		testCase{
			dep: map[string][]string{
				"A": []string{"B"},
				"B": []string{"C"},
				"C": []string{"B"},
			},
			tests:         []string{"A", "B", "C"},
			expectDepLoop: true,
		},
	}
	for index, test := range testCases {
		got, err := createTestDepGraph(test.dep, test.tests)
		if test.expectDepLoop {
			if err == nil {
				t.Fatalf("test case %d: want errors, got: %v", index, err)
			}
		} else {
			if err != nil {
				t.Fatalf("test case %d: want no errors, got: %v", index, err)
			}
			if !reflect.DeepEqual(test.expectedTests, got) {
				t.Fatalf("test case %d: want %v, got %v", index, test.expectedTests, got)
			}
		}
	}
}

func createBuildCopFile(veyronRoot string, t *testing.T) {
	content := `<?xml version="1.0" ?>
<rotation>
  <shift>
    <primary>spetrovic</primary>
    <secondary>suharshs</secondary>
    <startDate>Nov 5, 2014 12:00:00 PM</startDate>
  </shift>
  <shift>
    <primary>suharshs</primary>
    <secondary>tilaks</secondary>
    <startDate>Nov 12, 2014 12:00:00 PM</startDate>
  </shift>
  <shift>
    <primary>jsimsa</primary>
    <secondary>toddw</secondary>
    <startDate>Nov 19, 2014 12:00:00 PM</startDate>
  </shift>
</rotation>`
	buildCopRotationsFile, err := BuildCopRotationPath()
	if err != nil {
		t.Fatalf("%v", err)
	}
	dir := filepath.Dir(buildCopRotationsFile)
	dirMode := os.FileMode(0700)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		t.Fatalf("MkdirAll(%q, %v) failed: %v", dir, dirMode, err)
	}
	fileMode := os.FileMode(0644)
	if err := ioutil.WriteFile(buildCopRotationsFile, []byte(content), fileMode); err != nil {
		t.Fatalf("WriteFile(%q, %q, %v) failed: %v", buildCopRotationsFile, content, fileMode, err)
	}
}

func TestBuildCop(t *testing.T) {
	// Setup a fake VEYRON_ROOT.
	dir, prefix := "", ""
	tmpDir, err := ioutil.TempDir(dir, prefix)
	if err != nil {
		t.Fatalf("TempDir(%v, %v) failed: %v", dir, prefix, err)
	}
	defer os.RemoveAll(tmpDir)
	oldRoot, err := VeyronRoot()
	if err := os.Setenv("VEYRON_ROOT", tmpDir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("VEYRON_ROOT", oldRoot)

	// Create a buildcop.xml file.
	createBuildCopFile(tmpDir, t)

	ctx := DefaultContext()
	type testCase struct {
		targetTime       time.Time
		expectedBuildCop string
	}
	testCases := []testCase{
		testCase{
			targetTime:       time.Date(2013, time.November, 5, 12, 0, 0, 0, time.Local),
			expectedBuildCop: "",
		},
		testCase{
			targetTime:       time.Date(2014, time.November, 5, 12, 0, 0, 0, time.Local),
			expectedBuildCop: "spetrovic",
		},
		testCase{
			targetTime:       time.Date(2014, time.November, 5, 14, 0, 0, 0, time.Local),
			expectedBuildCop: "spetrovic",
		},
		testCase{
			targetTime:       time.Date(2014, time.November, 20, 14, 0, 0, 0, time.Local),
			expectedBuildCop: "jsimsa",
		},
	}
	for _, test := range testCases {
		got, err := BuildCop(ctx, test.targetTime)
		if err != nil {
			t.Fatalf("want no errors, got: %v", err)
		}
		if test.expectedBuildCop != got {
			t.Fatalf("want %v, got %v", test.expectedBuildCop, got)
		}
	}
}
