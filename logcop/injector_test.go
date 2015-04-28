// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"go/build"
	"go/token"
	"path"
	"strconv"
	"testing"

	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
)

const (
	failingPrefix       = "failschecks"
	failingPackageCount = 7
	testPackagePrefix   = "v.io/x/devtools/logcop/testdata"
)

func TestValidPackages(t *testing.T) {
	pkg := path.Join(testPackagePrefix, "passeschecks")
	_, methods := doTest(t, []string{pkg})
	if len(methods) > 0 {
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

func configureDefaultBuildConfig(ctx *tool.Context, tags []string) (cleanup func(), err error) {
	env, err := util.VanadiumEnvironment(ctx, util.HostPlatform())
	if err != nil {
		return nil, fmt.Errorf("failed to obtain the Vanadium environment: %v", err)
	}
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
	ctx := tool.NewDefaultContext()
	if _, err := configureDefaultBuildConfig(ctx, []string{"testpackage"}); err != nil {
		t.Fatal(err)
	}
	interfaceList := []string{path.Join(testPackagePrefix, "iface")}

	ifcs, impls, err := importPkgs(ctx, interfaceList, packages)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := len(impls), 1; got != want {
		t.Fatalf("got %d, want %d", got, want)
	}

	fset := token.NewFileSet() // positions are relative to fset

	impl := impls[0]
	asts, tpkg, err := parseAndTypeCheckPackage(ctx, fset, impl)
	if err != nil {
		t.Fatal(err)
	}

	interfaces := findPublicInterfaces(ctx, ifcs, tpkg)
	if len(interfaces) == 0 {
		t.Fatalf("Log injector did not find any interfaces in %s for %s", interfaceList, tpkg.Path())
	}
	methods := findMethodsImplementing(ctx, fset, tpkg, interfaces)
	if len(methods) == 0 {
		t.Fatalf("Log injector could not find any methods implementing the test interfaces in %v", impls)
	}
	methodPositions := functionDeclarationsAtPositions(asts, methods)
	return fset, checkMethods(methodPositions)
}