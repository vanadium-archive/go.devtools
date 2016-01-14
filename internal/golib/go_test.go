// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package golib

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"v.io/jiri/jiritest"
	"v.io/jiri/runutil"
	"v.io/x/devtools/internal/buildinfo"
	_ "v.io/x/devtools/internal/golib/testdata/basedep"
	"v.io/x/lib/metadata"
	"v.io/x/lib/set"
)

// TestGoVDLGeneration checks that PrepareGo generates up-to-date VDL files for
// select go tool commands.
func TestGoVDLGeneration(t *testing.T) {
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()
	reset := unsetJiriEnvVars(t)
	defer reset()
	s := fake.X.NewSeq()
	// Create a temporary directory for all our work.
	const tmpDirPrefix = "test_vgo"
	tmpDir, err := s.TempDir("", tmpDirPrefix)
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer fake.X.NewSeq().RemoveAll(tmpDir)

	// Create test files <tmpDir>/src/testpkg/test.vdl and
	// <tmpDir>/src/testpkg/doc.go
	pkgdir := filepath.Join(tmpDir, "src", "testpkg")
	const perm = os.ModePerm
	goFile := filepath.Join(pkgdir, "doc.go")
	inFile := filepath.Join(pkgdir, "test.vdl")
	outFile := inFile + ".go"
	if err := s.MkdirAll(pkgdir, perm).
		WriteFile(goFile, []byte("package testpkg\n"), perm).
		WriteFile(inFile, []byte("package testpkg\n"), perm).Done(); err != nil {
		t.Fatalf(`WriteFiles failed: %v`, err)
	}

	// Add <tmpDir> as first component of GOPATH and VDLPATH, so
	// we'll be able to find testpkg.  We need GOPATH for the "go
	// list" call when computing dependencies, and VDLPATH for the
	// "vdl generate" call.
	env := map[string]string{
		"PATH":    os.Getenv("PATH"),
		"GOPATH":  tmpDir,
		"VDLPATH": filepath.Join(tmpDir, "src"),
	}
	// Check that the 'env' go command does not generate the test VDL file.
	if _, err := PrepareGo(fake.X, env, []string{"env", "GOPATH"}, "", ""); err != nil {
		t.Fatalf("%v", err)
	}
	if _, err := s.Stat(outFile); err != nil {
		if !runutil.IsNotExist(err) {
			t.Fatalf("%v", err)
		}
	} else {
		t.Fatalf("file %v exists and it should not.", outFile)
	}
	// Check that the 'build' go command generates the test VDL file.
	if _, err := PrepareGo(fake.X, env, []string{"build", "testpkg"}, "", ""); err != nil {
		t.Fatalf("%v", err)
	}
	if _, err := s.Stat(outFile); err != nil {
		t.Fatalf("%v", err)
	}
}

// TestProcessGoCmdAndArgs tests the internal function that parses and filters
// out flags from the go tool command line.
func TestProcessGoCmdAndArgs(t *testing.T) {
	const (
		buildcmds = "build install test"
		allcmds   = "build generate install run test"
	)
	tests := []struct {
		Cmds        string
		Args        []string
		Pkgs, Files []string
		Tags        string
	}{
		{allcmds, nil, nil, nil, ""},
		{allcmds, []string{}, nil, nil, ""},

		// PACKAGES
		{buildcmds, []string{"pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"pkg1", "pkg2"}, []string{"pkg1", "pkg2"}, nil, ""},
		// Single dash
		{buildcmds, []string{"-a"}, nil, nil, ""},
		{buildcmds, []string{"-a", "pkg"}, []string{"pkg"}, nil, ""},
		// Single dash with equals
		{buildcmds, []string{"-p=99"}, nil, nil, ""},
		{buildcmds, []string{"-p=99", "pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"-a", "-p=99", "pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"-p=99", "-a", "pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"-tags=foo"}, nil, nil, "foo"},
		{buildcmds, []string{"-tags=foo", "pkg"}, []string{"pkg"}, nil, "foo"},
		{buildcmds, []string{"-a", "-tags=foo", "pkg"}, []string{"pkg"}, nil, "foo"},
		{buildcmds, []string{"-tags=foo", "-a", "pkg"}, []string{"pkg"}, nil, "foo"},
		// Single dash with space
		{buildcmds, []string{"-p", "99"}, nil, nil, ""},
		{buildcmds, []string{"-p", "99", "pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"-a", "-p", "99", "pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"-p", "99", "-a", "pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"-tags", "foo"}, nil, nil, "foo"},
		{buildcmds, []string{"-tags", "foo", "pkg"}, []string{"pkg"}, nil, "foo"},
		{buildcmds, []string{"-a", "-tags", "foo", "pkg"}, []string{"pkg"}, nil, "foo"},
		{buildcmds, []string{"-tags", "foo", "-a", "pkg"}, []string{"pkg"}, nil, "foo"},
		// Double dash
		{buildcmds, []string{"--a"}, nil, nil, ""},
		{buildcmds, []string{"--a", "pkg"}, []string{"pkg"}, nil, ""},
		// Double dash with equals
		{buildcmds, []string{"--p=99"}, nil, nil, ""},
		{buildcmds, []string{"--p=99", "pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"--a", "--p=99", "pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"--p=99", "--a", "pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"--tags=foo"}, nil, nil, "foo"},
		{buildcmds, []string{"--tags=foo", "pkg"}, []string{"pkg"}, nil, "foo"},
		{buildcmds, []string{"--a", "--tags=foo", "pkg"}, []string{"pkg"}, nil, "foo"},
		{buildcmds, []string{"--tags=foo", "--a", "pkg"}, []string{"pkg"}, nil, "foo"},
		// Double dash with space
		{buildcmds, []string{"--p", "99"}, nil, nil, ""},
		{buildcmds, []string{"--p", "99", "pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"--a", "--p", "99", "pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"--p", "99", "--a", "pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"--tags", "foo"}, nil, nil, "foo"},
		{buildcmds, []string{"--tags", "foo", "pkg"}, []string{"pkg"}, nil, "foo"},
		{buildcmds, []string{"--a", "--tags", "foo", "pkg"}, []string{"pkg"}, nil, "foo"},
		{buildcmds, []string{"--tags", "foo", "--a", "pkg"}, []string{"pkg"}, nil, "foo"},
		// Mixed
		{buildcmds, []string{"--p", "99", "-a", "-ccflags", "-I inc -X", "pkg1", "pkg2"}, []string{"pkg1", "pkg2"}, nil, ""},
		{buildcmds, []string{"--p", "99", "-tags=foo", "-a", "-ccflags", "-I inc -X", "pkg1", "pkg2"}, []string{"pkg1", "pkg2"}, nil, "foo"},

		// FILES
		{buildcmds, []string{"1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"1.go", "2.go"}, nil, []string{"1.go", "2.go"}, ""},
		// Single dash
		{buildcmds, []string{"-a"}, nil, nil, ""},
		{buildcmds, []string{"-a", "1.go"}, nil, []string{"1.go"}, ""},
		// Single dash with equals
		{buildcmds, []string{"-p=99"}, nil, nil, ""},
		{buildcmds, []string{"-p=99", "1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"-a", "-p=99", "1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"-p=99", "-a", "1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"-tags=foo"}, nil, nil, "foo"},
		{buildcmds, []string{"-tags=foo", "1.go"}, nil, []string{"1.go"}, "foo"},
		{buildcmds, []string{"-a", "-tags=foo", "1.go"}, nil, []string{"1.go"}, "foo"},
		{buildcmds, []string{"-tags=foo", "-a", "1.go"}, nil, []string{"1.go"}, "foo"},
		// Single dash with space
		{buildcmds, []string{"-p", "99"}, nil, nil, ""},
		{buildcmds, []string{"-p", "99", "1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"-a", "-p", "99", "1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"-p", "99", "-a", "1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"-tags", "foo"}, nil, nil, "foo"},
		{buildcmds, []string{"-tags", "foo", "1.go"}, nil, []string{"1.go"}, "foo"},
		{buildcmds, []string{"-a", "-tags", "foo", "1.go"}, nil, []string{"1.go"}, "foo"},
		{buildcmds, []string{"-tags", "foo", "-a", "1.go"}, nil, []string{"1.go"}, "foo"},
		// Double dash
		{buildcmds, []string{"--a"}, nil, nil, ""},
		{buildcmds, []string{"--a", "1.go"}, nil, []string{"1.go"}, ""},
		// Double dash with equals
		{buildcmds, []string{"--p=99"}, nil, nil, ""},
		{buildcmds, []string{"--p=99", "1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"--a", "--p=99", "1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"--p=99", "--a", "1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"--tags=foo"}, nil, nil, "foo"},
		{buildcmds, []string{"--tags=foo", "1.go"}, nil, []string{"1.go"}, "foo"},
		{buildcmds, []string{"--a", "--tags=foo", "1.go"}, nil, []string{"1.go"}, "foo"},
		{buildcmds, []string{"--tags=foo", "--a", "1.go"}, nil, []string{"1.go"}, "foo"},
		// Double dash with space
		{buildcmds, []string{"--p", "99"}, nil, nil, ""},
		{buildcmds, []string{"--p", "99", "1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"--a", "--p", "99", "1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"--p", "99", "--a", "1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"--tags", "foo"}, nil, nil, "foo"},
		{buildcmds, []string{"--tags", "foo", "1.go"}, nil, []string{"1.go"}, "foo"},
		{buildcmds, []string{"--a", "--tags", "foo", "1.go"}, nil, []string{"1.go"}, "foo"},
		{buildcmds, []string{"--tags", "foo", "--a", "1.go"}, nil, []string{"1.go"}, "foo"},
		// Mixed
		{buildcmds, []string{"--p", "99", "-a", "-ccflags", "-I inc -X", "1.go", "2.go"}, nil, []string{"1.go", "2.go"}, ""},
		{buildcmds, []string{"--p", "99", "-tags=foo", "-a", "-ccflags", "-I inc -X", "1.go", "2.go"}, nil, []string{"1.go", "2.go"}, "foo"},

		// Go run requires gofiles, and treats every non-gofile as an arg.
		{"run", []string{"-a"}, nil, nil, ""},
		{"run", []string{"--a"}, nil, nil, ""},
		{"run", []string{"-p=99"}, nil, nil, ""},
		{"run", []string{"--p=99"}, nil, nil, ""},
		{"run", []string{"-p", "99"}, nil, nil, ""},
		{"run", []string{"--p", "99"}, nil, nil, ""},
		{"run", []string{"-tags=foo"}, nil, nil, "foo"},
		{"run", []string{"--tags=foo"}, nil, nil, "foo"},
		{"run", []string{"-tags", "foo"}, nil, nil, "foo"},
		{"run", []string{"--tags", "foo"}, nil, nil, "foo"},
		{"run", []string{"arg"}, nil, nil, ""},
		{"run", []string{"1.go"}, nil, []string{"1.go"}, ""},
		{"run", []string{"1.go", "2.go"}, nil, []string{"1.go", "2.go"}, ""},
		{"run", []string{"1.go", "2.go", "arg"}, nil, []string{"1.go", "2.go"}, ""},
		{"run", []string{"-a", "--p", "99", "1.go", "2.go", "arg"}, nil, []string{"1.go", "2.go"}, ""},
		{"run", []string{"-a", "--tags", "foo", "1.go", "2.go", "arg"}, nil, []string{"1.go", "2.go"}, "foo"},

		// Go test treats the first dash-prefix as the start of the testbin flags.
		{"test", []string{"pkg", "-t"}, []string{"pkg"}, nil, ""},
		{"test", []string{"pkg", "-t1", "-t2"}, []string{"pkg"}, nil, ""},
		{"test", []string{"pkg1", "pkg2", "-t1", "-t2"}, []string{"pkg1", "pkg2"}, nil, ""},
		{"test", []string{"--a", "-p", "99", "pkg1", "pkg2", "-t1", "-t2"}, []string{"pkg1", "pkg2"}, nil, ""},
		{"test", []string{"--a", "-tags", "foo", "pkg1", "pkg2", "-t1", "-t2"}, []string{"pkg1", "pkg2"}, nil, "foo"},
		{"test", []string{"1.go", "-t"}, nil, []string{"1.go"}, ""},
		{"test", []string{"1.go", "-t1", "-t2"}, nil, []string{"1.go"}, ""},
		{"test", []string{"1.go", "2.go", "-t1", "-t2"}, nil, []string{"1.go", "2.go"}, ""},
		{"test", []string{"--a", "-p", "99", "1.go", "2.go", "-t1", "-t2"}, nil, []string{"1.go", "2.go"}, ""},
		{"test", []string{"--a", "-tags", "foo", "1.go", "2.go", "-t1", "-t2"}, nil, []string{"1.go", "2.go"}, "foo"},

		// Go generate only supports the -run non-bool flag.
		{"generate", []string{"-a"}, nil, nil, ""},
		{"generate", []string{"--a"}, nil, nil, ""},
		{"generate", []string{"-run=XX"}, nil, nil, ""},
		{"generate", []string{"--run=XX"}, nil, nil, ""},
		{"generate", []string{"-run", "XX"}, nil, nil, ""},
		{"generate", []string{"--run", "XX"}, nil, nil, ""},
		{"generate", []string{"pkg"}, []string{"pkg"}, nil, ""},
		{"generate", []string{"pkg1", "pkg2"}, []string{"pkg1", "pkg2"}, nil, ""},
		{"generate", []string{"-a", "--run", "XX", "pkg1", "pkg2"}, []string{"pkg1", "pkg2"}, nil, ""},
		{"generate", []string{"--run", "XX", "-a", "pkg1", "pkg2"}, []string{"pkg1", "pkg2"}, nil, ""},

		{"generate", []string{"1.go"}, nil, []string{"1.go"}, ""},
		{"generate", []string{"1.go", "2.go"}, nil, []string{"1.go", "2.go"}, ""},
		{"generate", []string{"-a", "--run", "XX", "1.go", "2.go"}, nil, []string{"1.go", "2.go"}, ""},
		{"generate", []string{"--run", "XX", "-a", "1.go", "2.go"}, nil, []string{"1.go", "2.go"}, ""},
	}
	for _, test := range tests {
		for _, cmd := range strings.Split(test.Cmds, " ") {
			pkgs, files, tags := processGoCmdAndArgs(cmd, test.Args)
			if got, want := pkgs, test.Pkgs; !reflect.DeepEqual(got, want) {
				t.Errorf("%s %v got pkgs %#v, want %#v", cmd, test.Args, got, want)
			}
			if got, want := files, test.Files; !reflect.DeepEqual(got, want) {
				t.Errorf("%s %v got files %#v, want %#v", cmd, test.Args, got, want)
			}
			if got, want := tags, test.Tags; got != want {
				t.Errorf("%s %v got tags %#v, want %#v", cmd, test.Args, got, want)
			}
		}
	}
}

// TestComputeGoDeps tests the internal function that calls "go list" to get
// transitive dependencies.
func TestComputeGoDeps(t *testing.T) {
	jirix, cleanup := jiritest.NewX(t)
	defer cleanup()
	// The golib package depends on itself and "fmt", but doesn't depend on
	// basedep.  The test does depend on basedep, which transitively depends on
	// transdep.
	deps := []string{"v.io/x/devtools/internal/golib", "fmt"}
	baseDeps := []string{"v.io/x/devtools/internal/golib/testdata/basedep", "v.io/x/devtools/internal/golib/testdata/transdep"}
	testDeps := append(deps, baseDeps...)
	tests := []struct {
		Pkgs, Deps []string
		Test       bool
	}{
		// This is checking the actual dependencies of the specified packages, so it
		// may break if we change the implementation; we try to pick dependencies
		// that are likely to remain in these packages.
		{nil, deps, false},
		{[]string{"."}, deps, false},
		{[]string{"../golib"}, deps, false},
		{[]string{"v.io/x/devtools/internal/golib"}, deps, false},
		{[]string{"v.io/x/devtools/internal/golib/..."}, deps, false},
		{nil, testDeps, true},
		{[]string{"."}, testDeps, true},
		{[]string{"../golib"}, testDeps, true},
		{[]string{"v.io/x/devtools/internal/golib"}, testDeps, true},
		{[]string{"v.io/x/devtools/internal/golib/..."}, testDeps, true},
	}
	// The PATH needs to include the "go" tool somewhere.
	env := map[string]string{
		"PATH": os.Getenv("PATH"),
	}
	for _, test := range tests {
		got, err := computeGoDeps(jirix, env, test.Pkgs, "", test.Test)
		if err != nil {
			t.Errorf("%v test=%v failed: %v", test.Pkgs, test.Test, err)
		}
		if want := test.Deps; !containsStrings(got, want) {
			t.Errorf("%v test=%v got %v, want to contain %v", test.Pkgs, test.Test, got, want)
		}
	}
}

func containsStrings(super, sub []string) bool {
	subSet := set.String.FromSlice(sub)
	set.String.Difference(subSet, set.String.FromSlice(super))
	return len(subSet) == 0
}

func extractFlag(args []string, name string) (string, error) {
	found, prefix, value := false, "-"+name+"=", ""
	for _, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			if found {
				return "", fmt.Errorf("flag %s set more than once in %v", name, args)
			}
			found = true
			value = strings.TrimPrefix(arg, prefix)
		}
	}
	if !found {
		return "", fmt.Errorf("flag %s not found in %v", name, args)
	}
	return value, nil
}

func TestSetBuildInfo(t *testing.T) {
	start := time.Now().UTC()
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()
	reset := unsetJiriEnvVars(t)
	defer reset()
	s := fake.X.NewSeq()
	// Set up a temp directory.
	dir, err := s.TempDir("", "v23_metadata_test")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer fake.X.NewSeq().RemoveAll(dir)

	env := map[string]string{
		"PATH":    os.Getenv("PATH"),
		"GOPATH":  os.Getenv("GOPATH"),
		"VDLPATH": os.Getenv("VDLPATH"),
	}
	args, err := PrepareGo(fake.X, env, []string{"build"}, "-when=now -why", "mypath")
	if err != nil {
		t.Fatal(err)
	}

	var md *metadata.T
	// Find a flag with 'prefix' and extract metadata out of it.
	ldFlagsValue, err := extractFlag(args, "ldflags")
	if err != nil {
		t.Fatal(err)
	}
	prefix := strings.Split(metadata.LDFlag(&metadata.T{}), "=")[0] + "="
	if !strings.HasPrefix(ldFlagsValue, prefix) {
		t.Fatalf("metadata flag not set")
	}
	ldFlagsValue = strings.TrimPrefix(ldFlagsValue, prefix)
	ldFlagsSuffix := " -when=now -why"
	if !strings.HasSuffix(ldFlagsValue, ldFlagsSuffix) {
		t.Fatalf("missing extra ld flags in %s", ldFlagsValue)
	}
	ldFlagsValue = strings.TrimSuffix(ldFlagsValue, ldFlagsSuffix)
	if md, err = metadata.FromBase64([]byte(ldFlagsValue)); err != nil {
		t.Fatalf("Unparseable: %v: %v", ldFlagsValue, err)
	}
	if md == nil {
		t.Fatalf("metadata flag not set")
	}
	bi, err := buildinfo.FromMetaData(md)
	if err != nil {
		t.Errorf("DecodeMetaData(%#v) failed: %v", md, err)
	}

	installSuffixValue, err := extractFlag(args, "installsuffix")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := installSuffixValue, "mypath"; got != want {
		t.Fatalf("expected suffix %q, but got %q", "mypath", want, got)
	}

	const fudge = -5 * time.Second
	if bi.Time.Before(start.Add(fudge)) {
		t.Errorf("build time %v < start %v", bi.Time, start)
	}
}

// unsetJiriEnvVars unsets the JIRI_ROOT and VDLROOT environment variables, so
// that running the vdl tool with -builtin_vdlroot takes effect.  Otherwise the
// vdl tool will look for standard vdl package (e.g. vdltool) under the source
// directories, and that doesn't exist in these tests.
//
// TODO(toddw): This is a temporary hack.  We should clean up the runutil logic
// so that it doesn't automatically append envvars from the OS.
func unsetJiriEnvVars(t *testing.T) func() {
	keys := []string{"JIRI_ROOT", "VDLROOT"}
	old := make(map[string]string)
	for _, key := range keys {
		old[key] = os.Getenv(key)
		if err := os.Setenv(key, ""); err != nil {
			t.Fatalf(`os.Setenv(%q, "") failed: %v`, key, err)
		}
	}
	return func() {
		for key, val := range old {
			if err := os.Setenv(key, val); err != nil {
				t.Fatalf(`os.Setenv(%q, %q) failed: %v`, key, val, err)
			}
		}
	}
}
