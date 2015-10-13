// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Run an executable using the specified profile and target's environment.

Usage:
   jiri run [flags] <executable> [arg ...]

<executable> [arg ...] is the executable to run and any arguments to pass
verbatim to the executable.

The jiri run flags are:
 -color=true
   Use color to format output.
 -manifest=.jiri_v23_profiles
   specify the profiles XML manifest filename.
 -n=false
   Show what commands will run but do not execute them.
 -profiles=base
   a comma separated list of profiles to use
 -skip-profiles=false
   if set, no profiles will be used
 -target=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form: [<tag>=]<arch>-<os>
 -v=false
   Print verbose output.
 -version=
   target version

The global flags are:
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.
*/
package main
