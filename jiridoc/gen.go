// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go -install v.io/x/devtools/jiri-... -env="" v.io/jiri

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("jiridoc is a dummy build target to generate full godoc for the jiri tool.")
	os.Exit(1)
}
