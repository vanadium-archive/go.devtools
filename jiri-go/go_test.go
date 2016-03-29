// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"v.io/jiri/jiritest"
	"v.io/jiri/profiles/profilesreader"
	"v.io/jiri/project"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/buildinfo"
	"v.io/x/devtools/tooldata"
	"v.io/x/lib/metadata"
)

// TestGoVanadiumEnvironment checks that the implementation of the "jiri go"
// command sets up the vanadium environment and then dispatches calls to the go
// tool.
func TestGoVanadiumEnvironment(t *testing.T) {
	// Unset GOPATH to start with a clean environment, and skip profiles to avoid
	// requiring a real third_party project.
	oldGoPath := os.Getenv("GOPATH")
	if err := os.Setenv("GOPATH", ""); err != nil {
		t.Fatal(err)
	}
	oldProfilesModeFlag := readerFlags.ProfilesMode
	readerFlags.ProfilesMode = profilesreader.SkipProfiles
	defer func() {
		if err := os.Setenv("GOPATH", oldGoPath); err != nil {
			t.Error(err)
		}
		readerFlags.ProfilesMode = oldProfilesModeFlag
	}()

	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()

	// Create a test project and identify it as a Go workspace.
	if err := fake.CreateRemoteProject("test"); err != nil {
		t.Fatal(err)
	}
	if err := fake.AddProject(project.Project{
		Name:   "test",
		Path:   "test",
		Remote: fake.Projects["test"],
	}); err != nil {
		t.Fatal(err)
	}
	if err := fake.UpdateUniverse(false); err != nil {
		t.Fatal(err)
	}
	config := tooldata.NewConfig(tooldata.GoWorkspacesOpt([]string{"test"}))
	if err := tooldata.SaveConfig(fake.X, config); err != nil {
		t.Fatal(err)
	}

	// The go tool should report the GOPATH contains the test workspace.
	var stdout bytes.Buffer
	fake.X.Context = tool.NewContext(tool.ContextOpts{Stdout: &stdout})
	if err := runGo(fake.X, []string{"env", "GOPATH"}); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(stdout.String()), filepath.Join(fake.X.Root, "test"); got != want {
		t.Errorf("GOPATH got %v, want %v", got, want)
	}
}

func TestGoBuildWithMetaData(t *testing.T) {
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()
	jirix := fake.X
	start := time.Now().UTC()
	if err := tooldata.SaveConfig(jirix, tooldata.NewConfig()); err != nil {
		t.Fatal(err)
	}
	s := jirix.NewSeq()
	// Set up a temp directory.
	dir, err := s.TempDir("", "v23_metadata_test")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer jirix.NewSeq().RemoveAll(dir)
	// Build the jiri-go binary itself.
	var buf bytes.Buffer
	testbin := filepath.Join(dir, "testbin")
	if err := s.Verbose(true).Capture(&buf, &buf).Last("jiri", "go", "--skip-profiles", "build", "-o", testbin); err != nil {
		t.Fatalf("build of jiri-go failed: %v\n%s", err, buf.String())
	}
	// Run the jiri-go binary.
	buf.Reset()
	if err := s.Verbose(true).Capture(&buf, &buf).Last(testbin, "-metadata"); err != nil {
		t.Errorf("run of jiri-go -metadata failed: %v\n%s", err, buf.String())
	}
	// Decode the output metadata and spot-check some values.
	outData := buf.Bytes()
	t.Log(string(outData))
	md, err := metadata.FromXML(outData)
	if err != nil {
		t.Errorf("FromXML failed: %v\n%s", err, outData)
	}
	bi, err := buildinfo.FromMetaData(md)
	if err != nil {
		t.Errorf("DecodeMetaData(%#v) failed: %v", md, err)
	}
	const fudge = -5 * time.Second
	if bi.Time.Before(start.Add(fudge)) {
		t.Errorf("build time %v < start %v", bi.Time, start)
	}
}
