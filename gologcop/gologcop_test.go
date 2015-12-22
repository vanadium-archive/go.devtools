// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"go/build"
	"go/token"
	"go/types"
	"io/ioutil"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"v.io/jiri/jiri"
	"v.io/jiri/jiritest"
	"v.io/jiri/profiles"
	"v.io/jiri/profiles/reader"
	"v.io/jiri/tool"
)

const (
	failingPrefix               = "failschecks"
	withArgsPrefix              = "withargs"
	withCommandLinePrefix       = "commandline"
	failingPackageCount         = 8
	withArgsPackageCount        = 2
	withCommandLinePackageCount = 2
	testPackagePrefix           = "v.io/x/devtools/gologcop/testdata"
)

func TestValidPackages(t *testing.T) {
	pkg := path.Join(testPackagePrefix, "passeschecks")
	_, methods := doTest(t, []string{pkg})
	if len(methods) > 0 {
		for _, m := range methods {
			t.Logf(">>> %v", m)
		}
		t.Fatalf("Test package %q failed to pass the log checks", pkg)
	}
}

func TestInvalidPackages(t *testing.T) {
	for i := 1; i <= failingPackageCount; i++ {
		pkg := path.Join(testPackagePrefix, failingPrefix, "test"+strconv.Itoa(i))
		_, methods := doTest(t, []string{pkg})
		if len(methods) == 0 {
			t.Fatalf("Test package %q passes log checks but it should not", pkg)
		}
	}
}

var skipProfiles = []string{"-skip-profiles"}

func TestRemove(t *testing.T) {
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()
	var stdout bytes.Buffer
	fake.X.Context = tool.NewContext(tool.ContextOpts{Stdout: &stdout})
	if _, err := configureDefaultBuildConfig(fake.X, []string{"testpackage"}); err != nil {
		t.Fatal(err)
	}
	pkg := path.Join(testPackagePrefix, "passeschecks")

	diffOnlyFlag = true
	if err := runRemover(fake.X, skipProfiles, []string{pkg}); err != nil {
		t.Fatal(err)
	}
	diffs := []string{}
	scanner := bufio.NewScanner(bytes.NewBufferString(stdout.String()))
	for scanner.Scan() {
		text := scanner.Text()
		if strings.Contains(text, "] >>") {
			continue
		}
		diffs = append(diffs, text)
	}

	diffFilename := filepath.Join("testdata", "passeschecks.diff")
	want := ""
	if buf, err := ioutil.ReadFile(diffFilename); err != nil {
		t.Fatal(err)
	} else {
		want = strings.TrimRight(string(buf), "\n")
	}

	if got := strings.Join(diffs, "\n"); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestInject(t *testing.T) {
	savedContextFlag := useContextFlag
	defer func() {
		useContextFlag = savedContextFlag
	}()
	useContextFlag = false
	testInject(t, "iface", failingPrefix, failingPackageCount)
}

func TestInjectWithArgs(t *testing.T) {
	savedContextFlag := useContextFlag
	defer func() {
		useContextFlag = savedContextFlag
	}()
	useContextFlag = false
	testInject(t, "iface2", withArgsPrefix, withArgsPackageCount)
}

func TestCommandLineArgs(t *testing.T) {
	savedCallFlag := injectCallFlag
	savedCallImportFlag := injectCallImportFlag
	defer func() {
		injectCallFlag = savedCallFlag
		injectCallImportFlag = savedCallImportFlag
	}()
	injectCallFlag = "Bar"
	injectCallImportFlag = "bar\tfoo.com/x/baz"
	if err := initInjectorFlags(); err != nil {
		t.Fatal(err)
	}
	if got, want := injectImportTag, "bar"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := injectImportPath, "foo.com/x/baz"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := injectPackage, "bar"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := injectCall, "Bar"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	injectCallFlag = "Bar"
	injectCallImportFlag = "foo.com/x/baz"
	if err := initInjectorFlags(); err != nil {
		t.Fatal(err)
	}
	if got, want := injectImportTag, ""; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := injectImportPath, "foo.com/x/baz"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := injectPackage, "baz"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := injectCall, "Bar"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInjectCommandLine(t *testing.T) {
	savedContextFlag := useContextFlag
	savedCallFlag := injectCallFlag
	savedCallImportFlag := injectCallImportFlag
	defer func() {
		injectCallFlag = savedCallFlag
		injectCallImportFlag = savedCallImportFlag
		useContextFlag = savedContextFlag
	}()
	injectCallFlag = "Bar"
	injectCallImportFlag = "bar 	\"foo.com/x/baz\""
	useContextFlag = true
	testInject(t, "iface3", withCommandLinePrefix, withCommandLinePackageCount)
}

func testInject(t *testing.T, iface, prefix string, testPackageCount int) {
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()
	if _, err := configureDefaultBuildConfig(fake.X, []string{"testpackage"}); err != nil {
		t.Fatal(err)
	}
	ifc := path.Join(testPackagePrefix, iface)

	diffOnlyFlag = true
	for i := 1; i <= testPackageCount; i++ {
		stdout := bytes.NewBuffer(nil)
		jirix := fake.X.Clone(tool.ContextOpts{Stdout: stdout})
		testPkg := "test" + strconv.Itoa(i)
		pkg := path.Join(testPackagePrefix, prefix, testPkg)
		if err := runInjector(jirix, skipProfiles, []string{ifc}, []string{pkg}, false); err != nil {
			t.Fatal(err)
		}
		diffs := []string{}
		scanner := bufio.NewScanner(bytes.NewBufferString(stdout.String()))
		re := regexp.MustCompile(".*Warning: [[:^space:]]+: (.*)")
		for scanner.Scan() {
			text := scanner.Text()
			if strings.Contains(text, "] >>") {
				continue
			}
			if parts := re.FindStringSubmatch(text); len(parts) == 2 {
				text = parts[1]
			}
			diffs = append(diffs, text)
		}
		diffFilename := filepath.Join("testdata", prefix, testPkg+".diff")
		want := ""
		if buf, err := ioutil.ReadFile(diffFilename); err != nil {
			t.Fatal(err)
		} else {
			want = strings.TrimRight(string(buf), "\n")
		}
		if got := strings.Join(diffs, "\n"); got != want {
			t.Errorf("%s: got %v, want %v", testPkg, got, want)
		}
	}
}

func configureDefaultBuildConfig(jirix *jiri.X, tags []string) (cleanup func(), err error) {
	rd, err := reader.NewReader(jirix, reader.SkipProfiles, "")
	if err != nil {
		return nil, fmt.Errorf("failed to obtain the Vanadium environment: %v", err)
	}
	rd.MergeEnvFromProfiles(reader.JiriMergePolicies(), profiles.NativeTarget(), "jiri")
	env := rd.Vars
	prevGOPATH := build.Default.GOPATH
	prevBuildTags := build.Default.BuildTags
	cleanup = func() {
		build.Default.GOPATH = prevGOPATH
		build.Default.BuildTags = prevBuildTags
	}
	build.Default.GOPATH = env.Get("GOPATH")
	build.Default.BuildTags = tags
	return cleanup, nil
}

func doTest(t *testing.T, packages []string) (*token.FileSet, map[funcDeclRef]error) {
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()
	if _, err := configureDefaultBuildConfig(fake.X, []string{"testpackage"}); err != nil {
		t.Fatal(err)
	}

	initInjectorFlags()
	interfaceList := []string{path.Join(testPackagePrefix, "iface")}

	ifcs, err := importPkgs(fake.X, skipProfiles, interfaceList)
	if err != nil {
		t.Fatal(err)
	}

	impls, err := importPkgs(fake.X, skipProfiles, packages)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := len(impls), 1; got != want {
		t.Fatalf("got %d, want %d", got, want)
	}

	ps := newState(fake.X)

	ifc := ifcs[0]
	_, ifcpkg, err := ps.parseAndTypeCheckPackage(ifc)
	if err != nil {
		t.Fatal(err)
	}

	interfaces := findPublicInterfaces(fake.X, []*types.Package{ifcpkg})
	if len(interfaces) == 0 {
		t.Fatalf("Log injector did not find any interfaces in %s for %s", interfaceList, ifcpkg.Path())
	}

	impl := impls[0]
	asts, tpkg, err := ps.parseAndTypeCheckPackage(impl)
	if err != nil {
		t.Fatal(err)
	}

	methods := findMethodsImplementing(fake.X, ps.fset, tpkg, interfaces)
	if len(methods) == 0 {
		t.Fatalf("Log injector could not find any methods implementing the test interfaces in %v", impls)
	}
	methodPositions, err := functionDeclarationsAtPositions(ps.fset, asts, ps.info, methods)
	if err != nil {
		t.Fatal(err)
	}
	return ps.fset, checkMethods(methodPositions)
}
