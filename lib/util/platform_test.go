package util

import (
	"reflect"
	"testing"
)

type parseTest struct {
	platform string
	expected *Platform
}

// TestParsePlatform checks the program logic used for parsing
// platform strings.
func TestParsePlatform(t *testing.T) {
	tests := []parseTest{
		{"x86", nil},
		{"x86-linux-oops", nil},
		{"x86-linux", &Platform{"x86", "", "linux"}},
		{"x86v1.0-linux", &Platform{"x86", "v1.0", "linux"}},
		{"armv1.0-linux", &Platform{"arm", "v1.0", "linux"}},
		{"amd64v1.0-linux", &Platform{"amd64", "v1.0", "linux"}},
		{"oops-linux", &Platform{"unknown", "unknown", "linux"}},
	}
	for _, test := range tests {
		got, err := ParsePlatform(test.platform)
		if test.expected == nil {
			if err == nil {
				t.Errorf("ParsePlatform(%v) didn't fail and it should", test.platform)
			}
			continue
		}
		if err != nil {
			t.Errorf("unexpected failure: %v", err)
			continue
		}
		if !reflect.DeepEqual(got, test.expected) {
			t.Errorf("unexpected result: expected %+v, got %+v", test.expected, got)
		}
	}
}
