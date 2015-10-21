// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Print vanadium environment variables.

If no arguments are given, prints all variables in NAME="VALUE" format, each on
a separate line ordered by name.  This format makes it easy to set all vars by
running the following bash command (or similar for other shells):
   eval $(jiri env)

If arguments are given, prints only the value of each named variable, each on a
separate line in the same order as the arguments.

Usage:
   jiri env [flags] [name ...]

[name ...] is an optional list of variable names.

The jiri env flags are:
 -color=true
   Use color to format output.
 -manifest=$JIRI_ROOT/.jiri_v23_profiles
   specify the profiles XML manifest filename.
 -n=false
   Show what commands will run but do not execute them.
 -profiles=base
   a comma separated list of profiles to use
 -skip-profiles=false
   if set, no profiles will be used
 -target=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form:
   <arch>-<os>[@<version>]|<tag>[@version]|<tag>=<arch>-<val>[@<version>]
 -v=false
   Print verbose output.

The global flags are:
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.
 -time=false
   Dump timing information to stderr before exiting the program.
*/
package main
