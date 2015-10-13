// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Executes a Go command in a docker container. This is primarily aimed at the
builds of Linux binaries and libraries where there is a dependence on cgo. This
allows for compilation (and cross-compilation) without polluting the host
filesystem with compilers, C-headers, libraries etc. as dependencies are
encapsulated in the docker image.

The docker image is expected to have the appropriate C-compiler and any
pre-built headers/libraries to be linked in.  It is also expected to have the
appropriate environment variables (such as CGO_ENABLED, CGO_CFLAGS etc) set.

Sample usage on *all* platforms (Linux/OS X):

Build the "./foo" package for the host architecture and linux (command works
from OS X as well):

    jiri-dockergo build

Build for linux/arm from any host (including OS X):

    GOARCH=arm jiri-dockergo build

For more information on docker see https://www.docker.com.

For more information on the design of this particular tool including the
definitions of default images, see:
https://docs.google.com/document/d/1Ud-QUVOjsaya57kgq0j24wDwTzKKE7o_PShQQs0DR5w/edit?usp=sharing

While the targets are built using the toolchain in the docker image, a local Go
installation is still required for Vanadium-specific compilation prep work -
such as invoking the VDL compiler on packages to generate up-to-date .go files.

Usage:
   jiri dockergo [flags] <arg ...>

<arg ...> is a list of arguments for the go tool.

The jiri dockergo flags are:
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
 -image=
   Name of the docker image to use. If empty, the tool will automatically select
   an image based on the environment variables, possibly edited by the profile
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.
*/
package main
