package util

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
