// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Manages the build pipeline for the Swift framework, from CGO bindings to
fattening the binaries.

Usage:
   jiri swift [flags] <command>

The jiri swift commands are:
   build       Builds and installs the cgo wrapper, as well as the Swift
               framework
   clean       Removes generated cgo binaries and headers
   help        Display help for commands or topics

The jiri swift flags are:
 -color=true
   Use color to format output.
 -v=false
   Print verbose output.

The global flags are:
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.
 -time=false
   Dump timing information to stderr before exiting the program.
*/
package main
