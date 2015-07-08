// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"go/build"
	"testing"
)

func TestPrintDot(t *testing.T) {
	// NOTE: we don't test direct=false goroot=true, since the test results would
	// be dependent on the standard go package dependencies, which aren't under
	// our control.
	const v = "v.io/x/devtools/godepcop/testdata/"
	tests := []struct {
		path   string
		direct bool
		goroot bool
		dot    string
	}{
		{v + "test-a", false, false, `digraph {
  node[shape=record,style=solid]
  edge[arrowhead=vee]
  graph[rankdir=TB,splines=true]
  0[label="v.io/x/devtools/godepcop/testdata/test-a"]
}
`},
		{v + "test-a", true, false, `digraph {
  node[shape=record,style=solid]
  edge[arrowhead=vee]
  graph[rankdir=TB,splines=true]
  0[label="v.io/x/devtools/godepcop/testdata/test-a"]
}
`},
		{v + "test-a", true, true, `digraph {
  node[shape=record,style=solid]
  edge[arrowhead=vee]
  graph[rankdir=TB,splines=true]
  0->{1}
  0[label="v.io/x/devtools/godepcop/testdata/test-a"]
  1[label="fmt",goroot=true]
}
`},
		{v + "test-b", false, false, `digraph {
  node[shape=record,style=solid]
  edge[arrowhead=vee]
  graph[rankdir=TB,splines=true]
  0->{1}
  1->{2}
  0[label="v.io/x/devtools/godepcop/testdata/test-b"]
  1[label="v.io/x/devtools/godepcop/testdata/test-c"]
  2[label="v.io/x/devtools/godepcop/testdata/test-a"]
}
`},
		{v + "test-b", true, false, `digraph {
  node[shape=record,style=solid]
  edge[arrowhead=vee]
  graph[rankdir=TB,splines=true]
  0->{1}
  0[label="v.io/x/devtools/godepcop/testdata/test-b"]
  1[label="v.io/x/devtools/godepcop/testdata/test-c"]
}
`},
		{v + "test-b", true, true, `digraph {
  node[shape=record,style=solid]
  edge[arrowhead=vee]
  graph[rankdir=TB,splines=true]
  0->{1 2}
  0[label="v.io/x/devtools/godepcop/testdata/test-b"]
  1[label="fmt",goroot=true]
  2[label="v.io/x/devtools/godepcop/testdata/test-c"]
}
`},
		{v + "test-c", false, false, `digraph {
  node[shape=record,style=solid]
  edge[arrowhead=vee]
  graph[rankdir=TB,splines=true]
  0->{1}
  0[label="v.io/x/devtools/godepcop/testdata/test-c"]
  1[label="v.io/x/devtools/godepcop/testdata/test-a"]
}
`},
		{v + "test-c", true, false, `digraph {
  node[shape=record,style=solid]
  edge[arrowhead=vee]
  graph[rankdir=TB,splines=true]
  0->{1}
  0[label="v.io/x/devtools/godepcop/testdata/test-c"]
  1[label="v.io/x/devtools/godepcop/testdata/test-a"]
}
`},
		{v + "test-c", true, true, `digraph {
  node[shape=record,style=solid]
  edge[arrowhead=vee]
  graph[rankdir=TB,splines=true]
  0->{1}
  0[label="v.io/x/devtools/godepcop/testdata/test-c"]
  1[label="v.io/x/devtools/godepcop/testdata/test-a"]
}
`},
	}
	for _, test := range tests {
		pkg, err := importPackage(test.path)
		if err != nil {
			t.Errorf("importPackage(%q) failed: %v", test.path, err)
		}
		opts := depOpts{DirectOnly: test.direct, IncludeGoroot: test.goroot}
		var buf bytes.Buffer
		if err := printDot(&buf, []*build.Package{pkg}, opts); err != nil {
			t.Errorf("printDot(%q, %v) failed: %v", test.path, opts, err)
		}
		if got, want := buf.String(), test.dot; got != want {
			t.Errorf("printDot(%q, %v) got %v, want %v", test.path, opts, got, want)
		}
	}
}
