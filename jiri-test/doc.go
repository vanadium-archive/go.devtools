// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Manage vanadium tests.

Usage:
   test [flags] <command>

The test commands are:
   generate    Generate supporting code for v23 integration tests
   project     Run tests for a vanadium project
   run         Run vanadium tests
   list        List vanadium tests
   help        Display help for commands or topics

The test flags are:
 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

The global flags are:
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.

Test generate - Generate supporting code for v23 integration tests

The generate command supports the vanadium integration test framework and unit
tests by generating go files that contain supporting code.  jiri test generate
is intended to be invoked via the 'go generate' mechanism and the resulting
files are to be checked in.

Integration tests are functions of the following form:

    func V23Test<x>(i *v23tests.T)

These functions are typically defined in 'external' *_test packages, to ensure
better isolation.  But they may also be defined directly in the 'internal' *
package.  The following helper functions will be generated:

    func TestV23<x>(t *testing.T) {
      v23tests.RunTest(t, V23Test<x>)
    }

In addition a TestMain function is generated, if it doesn't already exist.  Note
that Go requires that at most one TestMain function is defined across both the
internal and external test packages.

The generated TestMain performs common initialization, and also performs child
process dispatching for tests that use "v.io/veyron/test/modules".

Usage:
   test generate [flags] [packages]

list of go packages

The test generate flags are:
 -prefix=v23
   Specifies the prefix to use for generated files. Up to two files may
   generated, the defaults are v23_test.go and v23_internal_test.go, or
   <prefix>_test.go and <prefix>_internal_test.go.
 -progress=false
   Print verbose progress information.

Test project - Run tests for a vanadium project

Runs tests for a vanadium project that is by the remote URL specified as the
command-line argument. Projects hosted on googlesource.com, can be specified
using the basename of the URL (e.g. "vanadium.go.core" implies
"https://vanadium.googlesource.com/vanadium.go.core").

Usage:
   test project <project>

<project> identifies the project for which to run tests.

Test run - Run vanadium tests

Run vanadium tests.

Usage:
   test run [flags] <name...>

<name...> is a list names identifying the tests to run.

The test run flags are:
 -blessings-root=dev.v.io
   The blessings root.
 -clean-go=true
   Specify whether to remove Go object files and binaries before running the
   tests. Setting this flag to 'false' may lead to faster Go builds, but it may
   also result in some source code changes not being reflected in the tests
   (e.g., if the change was made in a different Go workspace).
 -num-test-workers=<runtime.NumCPU()>
   Set the number of test workers to use; use 1 to serialize all tests.
 -output-dir=
   Directory to output test results into.
 -part=-1
   Specify which part of the test to run.
 -pkgs=
   Comma-separated list of Go package expressions that identify a subset of
   tests to run; only relevant for Go-based tests
 -v23.credentials.admin=
   Directory for vanadium credentials.
 -v23.credentials.publisher=
   Directory for vanadium credentials for publishing new binaries.
 -v23.namespace.root=/ns.dev.v.io:8101
   The namespace root.

Test list - List vanadium tests

List vanadium tests.

Usage:
   test list

Test help - Display help for commands or topics

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

Usage:
   test help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The test help flags are:
 -style=compact
   The formatting style for help output:
      compact - Good for compact cmdline output.
      full    - Good for cmdline output, shows all global flags.
      godoc   - Good for godoc processing.
   Override the default by setting the CMDLINE_STYLE environment variable.
 -width=<terminal width>
   Format output to this target width in runes, or unlimited if width < 0.
   Defaults to the terminal width if available.  Override the default by setting
   the CMDLINE_WIDTH environment variable.
*/
package main
