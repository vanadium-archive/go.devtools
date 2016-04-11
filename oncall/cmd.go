// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go .

package main

import (
	"v.io/jiri/tool"
	"v.io/x/lib/cmdline"
)

const (
	bucketData = "gs://vanadium-oncall/data"
	bucketPics = "gs://vanadium-oncall-pics"
)

func init() {
	tool.InitializeRunFlags(&cmdRoot.Flags)
}

func main() {
	cmdline.Main(cmdRoot)
}

// cmdRoot represents the root of the oncall tool.
var cmdRoot = &cmdline.Command{
	Name:     "oncall",
	Short:    "Command oncall implements oncall specific utilities used by Vanadium team",
	Long:     "Command oncall implements oncall specific utilities used by Vanadium team.",
	Children: []*cmdline.Command{cmdServe},
}
