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

Usage:
   jiri go [flags] <arg ...>

<arg ...> is a list of arguments for the go tool.

The jiri go flags are:
 -color=true
   Use color to format output.
 -env=
   specifcy an environment variable in the form: <var>=[<val>],...
 -merge-policies=+CCFLAGS,+CGO_CFLAGS,+CGO_CXXFLAGS,+CGO_LDFLAGS,+CXXFLAGS,GOARCH,GOOS,GOPATH:,^GOROOT*,+LDFLAGS,:PATH,VDLPATH:
   specify policies for merging environment variables
 -n=false
   Show what commands will run but do not execute them.
 -profiles=base,jiri
   a comma separated list of profiles to use
 -profiles-db=$JIRI_ROOT/.jiri_v23_profiles
   specify the profiles XML manifest filename.
 -skip-profiles=false
   if set, no profiles will be used
 -target=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form:
   <arch>-<os>[@<version>]|<arch>-<val>[@<version>]
 -v=false
   Print verbose output.

The global flags are:
 -extra-ldflags=
   This tool sets some ldflags automatically, e.g. to set binary metadata.  The
   extra-ldflags are appended to the end of those automatically generated
   ldflags.  Note that if your go command line specifies -ldflags explicitly, it
   will override both the automatically generated ldflags as well as the
   extra-ldflags.
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.
 -print-run-env=false
   print detailed info on environment variables and the command line used
 -system-go=false
   use the version of go found in $PATH rather than that built by the go profile
 -time=false
   Dump timing information to stderr before exiting the program.
*/
package main
