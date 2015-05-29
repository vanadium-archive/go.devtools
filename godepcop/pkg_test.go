// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"go/build"
	"reflect"
	"sort"
	"testing"
)

func TestPackageDeps(t *testing.T) {
	const v = "v.io/x/devtools/godepcop/testdata/"
	tests := []struct {
		path   string
		direct bool
		goroot bool
		deps   []string
	}{
		{v + "test-a", false, false, nil},
		{v + "test-a", false, true, []string{"fmt"}},
		{v + "test-a", true, false, nil},
		{v + "test-a", true, true, []string{"fmt"}},

		{v + "test-b", false, false, []string{v + "test-a", v + "test-c"}},
		{v + "test-b", false, true, []string{"fmt", v + "test-a", v + "test-c"}},
		{v + "test-b", true, false, []string{v + "test-c"}},
		{v + "test-b", true, true, []string{"fmt", v + "test-c"}},

		{v + "test-c", false, false, []string{v + "test-a"}},
		{v + "test-c", false, true, []string{"fmt", v + "test-a"}},
		{v + "test-c", true, false, []string{v + "test-a"}},
		{v + "test-c", true, true, []string{v + "test-a"}},
	}
	for _, test := range tests {
		pkg, err := importPackage(test.path)
		if err != nil {
			t.Errorf("importPackage(%q) failed: %v", test.path, err)
		}
		depPkgs := make(map[string]*build.Package)
		opts := depOpts{DirectOnly: test.direct, IncludeGoroot: test.goroot}
		if err := opts.Deps(pkg, depPkgs); err != nil {
			t.Errorf("%v failed: %v", test, err)
		}
		if test.direct || !test.goroot {
			var deps []string
			for path, _ := range depPkgs {
				deps = append(deps, path)
			}
			sort.Strings(deps)
			sort.Strings(test.deps)
			if got, want := deps, test.deps; !reflect.DeepEqual(got, want) {
				t.Errorf("%v got %q, want %q", test, got, want)
			}
		} else {
			// Showing transitive GOROOT packages leaves us at the mercy of the Go
			// standard package dependencies, which is annoying.  We only require that
			// deps contains all packages listed in test.deps.
			got := make(map[string]bool)
			for path, _ := range depPkgs {
				got[path] = true
			}
			for _, path := range test.deps {
				if !got[path] {
					t.Errorf("%v missing path %q, got %v", test, path, got)
				}
			}
		}
	}
}
