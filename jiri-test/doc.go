// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Manage vanadium tests.

Usage:
   jiri test [flags] <command>

The jiri test commands are:
   poll        Poll existing jiri projects
   project     Run tests for a vanadium project
   run         Run vanadium tests
   list        List vanadium tests
   help        Display help for commands or topics

The jiri test flags are:
 -color=true
   Use color to format output.
 -env=
   specify an environment variable in the form: <var>=[<val>],...
 -merge-policies=+CCFLAGS,+CGO_CFLAGS,+CGO_CXXFLAGS,+CGO_LDFLAGS,+CXXFLAGS,GOARCH,GOOS,GOPATH:,^GOROOT*,+LDFLAGS,:PATH,VDLPATH:
   specify policies for merging environment variables
 -profiles=v23:base,jiri
   a comma separated list of profiles to use
 -profiles-db=$JIRI_ROOT/.jiri_root/profile_db
   the path, relative to JIRI_ROOT, that contains the profiles database.
 -skip-profiles=false
   if set, no profiles will be used
 -target=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form: <arch>-<os>[@<version>]
 -v=false
   Print verbose output.

The global flags are:
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.
 -time=false
   Dump timing information to stderr before exiting the program.

Jiri test poll - Poll existing jiri projects

Poll jiri projects that can affect the outcome of the given tests and report
whether any new changes in these projects exist. If no tests are specified, all
projects are polled by default.

Usage:
   jiri test poll [flags] <test ...>

<test ...> is a list of tests that determine what projects to poll.

The jiri test poll flags are:
 -manifest=
   Name of the project manifest.

 -color=true
   Use color to format output.
 -env=
   specify an environment variable in the form: <var>=[<val>],...
 -merge-policies=+CCFLAGS,+CGO_CFLAGS,+CGO_CXXFLAGS,+CGO_LDFLAGS,+CXXFLAGS,GOARCH,GOOS,GOPATH:,^GOROOT*,+LDFLAGS,:PATH,VDLPATH:
   specify policies for merging environment variables
 -profiles=v23:base,jiri
   a comma separated list of profiles to use
 -profiles-db=$JIRI_ROOT/.jiri_root/profile_db
   the path, relative to JIRI_ROOT, that contains the profiles database.
 -skip-profiles=false
   if set, no profiles will be used
 -target=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form: <arch>-<os>[@<version>]
 -v=false
   Print verbose output.

Jiri test project - Run tests for a vanadium project

Runs tests for a vanadium project that is by the remote URL specified as the
command-line argument. Projects hosted on googlesource.com, can be specified
using the basename of the URL (e.g. "vanadium.go.core" implies
"https://vanadium.googlesource.com/vanadium.go.core").

Usage:
   jiri test project [flags] <project>

<project> identifies the project for which to run tests.

The jiri test project flags are:
 -color=true
   Use color to format output.
 -env=
   specify an environment variable in the form: <var>=[<val>],...
 -merge-policies=+CCFLAGS,+CGO_CFLAGS,+CGO_CXXFLAGS,+CGO_LDFLAGS,+CXXFLAGS,GOARCH,GOOS,GOPATH:,^GOROOT*,+LDFLAGS,:PATH,VDLPATH:
   specify policies for merging environment variables
 -profiles=v23:base,jiri
   a comma separated list of profiles to use
 -profiles-db=$JIRI_ROOT/.jiri_root/profile_db
   the path, relative to JIRI_ROOT, that contains the profiles database.
 -skip-profiles=false
   if set, no profiles will be used
 -target=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form: <arch>-<os>[@<version>]
 -v=false
   Print verbose output.

Jiri test run - Run vanadium tests

Run vanadium tests.

Usage:
   jiri test run [flags] <name...>

<name...> is a list names identifying the tests to run.

The jiri test run flags are:
 -blessings-root=dev.v.io
   The blessings root.
 -clean-go=true
   Specify whether to remove Go object files and binaries before running the
   tests. Setting this flag to 'false' may lead to faster Go builds, but it may
   also result in some source code changes not being reflected in the tests
   (e.g., if the change was made in a different Go workspace).
 -mock-file-contents=
   Colon-separated file contents to check when testing presubmit test. This flag
   is only used when running presubmit end-to-end test.
 -mock-file-paths=
   Colon-separated file paths to read when testing presubmit test. This flag is
   only used when running presubmit end-to-end test.
 -num-test-workers=<runtime.NumCPU()>
   Set the number of test workers to use; use 1 to serialize all tests.
 -output-dir=
   Directory to output test results into.
 -part=-1
   Specify which part of the test to run.
 -pkgs=
   Comma-separated list of Go package expressions that identify a subset of
   tests to run; only relevant for Go-based tests. Example usage: jiri test run
   -pkgs v.io/x/ref vanadium-go-test
 -v23.namespace.root=/ns.dev.v.io:8101
   The namespace root.

 -color=true
   Use color to format output.
 -env=
   specify an environment variable in the form: <var>=[<val>],...
 -merge-policies=+CCFLAGS,+CGO_CFLAGS,+CGO_CXXFLAGS,+CGO_LDFLAGS,+CXXFLAGS,GOARCH,GOOS,GOPATH:,^GOROOT*,+LDFLAGS,:PATH,VDLPATH:
   specify policies for merging environment variables
 -profiles=v23:base,jiri
   a comma separated list of profiles to use
 -profiles-db=$JIRI_ROOT/.jiri_root/profile_db
   the path, relative to JIRI_ROOT, that contains the profiles database.
 -skip-profiles=false
   if set, no profiles will be used
 -target=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form: <arch>-<os>[@<version>]
 -v=false
   Print verbose output.

Jiri test list - List vanadium tests

List vanadium tests.

Usage:
   jiri test list [flags]

The jiri test list flags are:
 -color=true
   Use color to format output.
 -env=
   specify an environment variable in the form: <var>=[<val>],...
 -merge-policies=+CCFLAGS,+CGO_CFLAGS,+CGO_CXXFLAGS,+CGO_LDFLAGS,+CXXFLAGS,GOARCH,GOOS,GOPATH:,^GOROOT*,+LDFLAGS,:PATH,VDLPATH:
   specify policies for merging environment variables
 -profiles=v23:base,jiri
   a comma separated list of profiles to use
 -profiles-db=$JIRI_ROOT/.jiri_root/profile_db
   the path, relative to JIRI_ROOT, that contains the profiles database.
 -skip-profiles=false
   if set, no profiles will be used
 -target=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form: <arch>-<os>[@<version>]
 -v=false
   Print verbose output.

Jiri test help - Display help for commands or topics

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

Usage:
   jiri test help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The jiri test help flags are:
 -style=compact
   The formatting style for help output:
      compact   - Good for compact cmdline output.
      full      - Good for cmdline output, shows all global flags.
      godoc     - Good for godoc processing.
      shortonly - Only output short description.
   Override the default by setting the CMDLINE_STYLE environment variable.
 -width=<terminal width>
   Format output to this target width in runes, or unlimited if width < 0.
   Defaults to the terminal width if available.  Override the default by setting
   the CMDLINE_WIDTH environment variable.
*/
package main
