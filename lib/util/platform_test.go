package util

import (
	"runtime"
	"testing"
)

var empty = Platform{}

type parseTest struct {
	platform string
	expected Platform
}

// TestParsePlatform checks the program logic used for parsing
// platform strings.
func TestParsePlatform(t *testing.T) {
	tests := []parseTest{
		{"x86", empty},
		{"x86-linux-gnu-oops", empty},
		{"", Platform{runtime.GOARCH, "unknown", runtime.GOOS, "unknown"}},
		{"x86-linux", Platform{"x86", "", "linux", "unknown"}},
		{"x86-linux-gnu", Platform{"x86", "", "linux", "gnu"}},
		{"x86v1.0-linux-gnu", Platform{"x86", "v1.0", "linux", "gnu"}},
		{"armv1.0-linux-gnu", Platform{"arm", "v1.0", "linux", "gnu"}},
		{"amd64v1.0-linux-gnu", Platform{"amd64", "v1.0", "linux", "gnu"}},
		{"oops-linux", Platform{"unknown", "unknown", "linux", "unknown"}},
	}
	for _, test := range tests {
		got, err := ParsePlatform(test.platform)
		if test.expected == empty {
			if err == nil {
				t.Errorf("ParsePlatform(%v) didn't fail and it should", test.platform)
			}
			continue
		}
		if err != nil {
			t.Errorf("unexpected failure: %v", err)
			continue
		}
		if got != test.expected {
			t.Errorf("unexpected result: expected %+v, got %+v", test.expected, got)
		}
	}
}
