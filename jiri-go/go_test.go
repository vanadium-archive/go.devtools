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
	"v.io/jiri/profiles"
	"v.io/jiri/project"
	"v.io/jiri/runutil"
	"v.io/jiri/tool"
	"v.io/jiri/util"
	"v.io/x/devtools/internal/buildinfo"
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
	oldProfilesModeFlag := profilesModeFlag
	profilesModeFlag = profiles.SkipProfiles
	defer func() {
		if err := os.Setenv("GOPATH", oldGoPath); err != nil {
			t.Error(err)
		}
		profilesModeFlag = oldProfilesModeFlag
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
	config := util.NewConfig(util.GoWorkspacesOpt([]string{"test"}))
	if err := util.SaveConfig(fake.X, config); err != nil {
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
	ctx, start := tool.NewDefaultContext(), time.Now().UTC()
	// Set up a temp directory.
	dir, err := ctx.Run().TempDir("", "v23_metadata_test")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer ctx.Run().RemoveAll(dir)
	// Build the jiri-go binary itself.
	var buf bytes.Buffer
	opts := runutil.Opts{Stdout: &buf, Stderr: &buf, Verbose: true}
	testbin := filepath.Join(dir, "testbin")
	if err := ctx.Run().CommandWithOpts(opts, "jiri", "go", "build", "-o", testbin); err != nil {
		t.Fatalf("build of jiri-go failed: %v\n%s", err, buf.String())
	}
	// Run the jiri-go binary.
	buf.Reset()
	if err := ctx.Run().CommandWithOpts(opts, testbin, "-metadata"); err != nil {
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
