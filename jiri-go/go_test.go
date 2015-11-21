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

	"v.io/jiri/jiri"
	"v.io/jiri/profiles"
	"v.io/jiri/runutil"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/buildinfo"
	"v.io/x/devtools/jiri-v23-profile/v23_profile"
	"v.io/x/lib/metadata"
)

// TestGoVanadiumEnvironment checks that the implementation of the
// "jiri go" command sets up the vanadium environment and then
// dispatches calls to the go tool.
func TestGoVanadiumEnvironment(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ctx := tool.NewContext(tool.ContextOpts{Stdout: &stdout, Stderr: &stderr})
	jirix := &jiri.X{Context: ctx}
	oldGoPath := os.Getenv("GOPATH")
	if err := os.Setenv("GOPATH", ""); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("GOPATH", oldGoPath)
	if err := runGo(jirix, []string{"env", "GOPATH"}); err != nil {
		t.Fatalf("%v", err)
	}
	ch, err := profiles.NewConfigHelper(jirix, profiles.UseProfiles, v23_profile.DefaultManifestFilename)
	if err != nil {
		t.Fatalf("%v", err)
	}
	ch.MergeEnvFromProfiles(profiles.JiriMergePolicies(), profiles.NativeTarget(), "jiri")
	if got, want := strings.TrimSpace(stdout.String()), ch.Get("GOPATH"); got != want {
		t.Fatalf("GOPATH got %v, want %v", got, want)
	}
}

func TestGoBuildWithMetaData(t *testing.T) {
	jirix, start := tool.NewDefaultContext(), time.Now().UTC()
	// Set up a temp directory.
	dir, err := jirix.Run().TempDir("", "v23_metadata_test")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer jirix.Run().RemoveAll(dir)
	// Build the jiri binary itself.
	var buf bytes.Buffer
	opts := runutil.Opts{Stdout: &buf, Stderr: &buf, Verbose: true}
	testbin := filepath.Join(dir, "testbin")
	if err := jirix.Run().CommandWithOpts(opts, "jiri", "go", "build", "-o", testbin); err != nil {
		t.Fatalf("build of jiri failed: %v\n%s", err, buf.String())
	}
	// Run the jiri binary.
	buf.Reset()
	if err := jirix.Run().CommandWithOpts(opts, testbin, "-metadata"); err != nil {
		t.Errorf("run of jiri -metadata failed: %v\n%s", err, buf.String())
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
