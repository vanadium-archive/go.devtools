package main

import (
	"fmt"
	"testing"
)

type testCase struct {
	args          []string
	expectedError error
	expectedHostA string
	expectedHostB string
	expectedUserA string
	expectedUserB string
}

func TestParseUserAndHost(t *testing.T) {
	testCases := []testCase{
		// Valid arguments: testing two different users.
		testCase{
			args:          []string{"jiri@home", "veyron@work"},
			expectedError: nil,
			expectedHostA: "home",
			expectedHostB: "work",
			expectedUserA: "jiri",
			expectedUserB: "veyron",
		},
		// Valid arguments: testing the default for the second user.
		testCase{
			args:          []string{"jiri@home", "work"},
			expectedError: nil,
			expectedHostA: "home",
			expectedHostB: "work",
			expectedUserA: "jiri",
			expectedUserB: "jiri",
		},
		// Invalid arguments: missing the first user.
		testCase{
			args:          []string{"home", ""},
			expectedError: fmt.Errorf("failed to parse user: home"),
			expectedHostA: "",
			expectedHostB: "",
			expectedUserA: "",
			expectedUserB: "",
		},
		// Invalid arguments: incorrent number of arguments.
		testCase{
			args:          []string{"jiri@home"},
			expectedError: fmt.Errorf("unexpected number of arguments: got 1, want 2"),
			expectedHostA: "",
			expectedHostB: "",
			expectedUserA: "",
			expectedUserB: "",
		},
		// Invalid arguments: more than one '@' character.
		testCase{
			args:          []string{"jiri@home@office", "veyron@work"},
			expectedError: fmt.Errorf("unexpected length of [jiri home office]: expected at most 2"),
			expectedHostA: "",
			expectedHostB: "",
			expectedUserA: "",
			expectedUserB: "",
		},
	}

	for _, test := range testCases {
		userA, hostA, userB, hostB, err := parseUserAndHost(test.args)
		if test.expectedError == nil && err != nil {
			t.Fatalf("parseUserAndHost(%v) failed: %v", test.args, err)
		}
		if test.expectedError != nil && err == nil {
			t.Fatalf("parseUserAndHost(%v) did not fail when it should", test.args)
		}
		if test.expectedError != nil && err != nil {
			if got, want := err.Error(), test.expectedError.Error(); got != want {
				t.Fatalf("unexpected error: got %q, want %q", got, want)
			}
		}
		if got, want := userA, test.expectedUserA; got != want {
			t.Fatalf("unexpected user: got %q, want %q", got, want)
		}
		if got, want := userB, test.expectedUserB; got != want {
			t.Fatalf("unexpected user: got %q, want %q", got, want)
		}
		if got, want := hostA, test.expectedHostA; got != want {
			t.Fatalf("unexpected host: got %q, want %q", got, want)
		}
		if got, want := hostB, test.expectedHostB; got != want {
			t.Fatalf("unexpected host: got %q, want %q", got, want)
		}
	}
}
