// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// THIS FILE IS DEPRECATED!!!
// Please edit the new "jiri" tool in release/go/src/v.io/jiri.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// main calls "jiri" tool with whatever arguments it was called with.
func main() {
	args := os.Args[1:]
	cmd := exec.Command("jiri", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	// If JIRI_ROOT is not set, set it to $V23_ROOT.
	jiriRoot := os.Getenv("JIRI_ROOT")
	if jiriRoot == "" {
		if err := os.Setenv("JIRI_ROOT", os.Getenv("V23_ROOT")); err != nil {
			panic(err)
		}
	}

	fmt.Fprintf(os.Stderr, "\nWARNING: The v23 tool will soon be deprecated.\nPlease run 'jiri %s' instead.\n\n", strings.Join(args, " "))

	// Sleep for annoyance.
	time.Sleep(3 * time.Second)

	if err := cmd.Run(); err != nil {
		// The jiri tool should have reported an error in its output.  Don't
		// print an error here because it can be confusing and makes it harder
		// to spot the real error.
		os.Exit(1)
	}
}
