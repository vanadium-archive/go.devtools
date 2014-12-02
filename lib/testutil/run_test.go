package testutil

import (
	"reflect"
	"testing"

	"veyron.io/tools/lib/util"
)

func TestProjectTests(t *testing.T) {
	projects := map[string][]string{
		"veyron":  []string{"veyron-go-build", "veyron-go-test"},
		"default": []string{"tools-go-build", "tools-go-test"},
	}
	ctx := util.DefaultContext()

	// Get tests for a repo that is in the config file.
	got, err := projectTests(ctx, projects, []string{"veyron"})
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
	got, err = projectTests(ctx, projects, []string{"non-exist-repo"})
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
