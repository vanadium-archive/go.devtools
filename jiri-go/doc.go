// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Wrapper around the 'go' tool that can be used for compilation of vanadium Go
sources. It takes care of vanadium-specific setup, such as setting up the Go
specific environment variables or making sure that VDL generated files are
regenerated before compilation.

In particular, the tool invokes the following command before invoking any go
tool commands that compile vanadium Go code:

vdl generate -lang=go all

Usage:
   go [flags] <arg ...>

<arg ...> is a list of arguments for the go tool.

The go flags are:
 -color=true
   Use color to format output.
 -manifest=.jiri_profiles
   specify the profiles XML manifest filename.
 -n=false
   Show what commands will run but do not execute them.
 -profiles=base
   a comma separated list of profiles to use
 -target=native=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form: [<tag>=]<arch>-<os>
 -v=false
   Print verbose output.

The global flags are:
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.
 -system-go=false
   use the version of go found in $PATH rather than that built by the go profile
*/
package main
