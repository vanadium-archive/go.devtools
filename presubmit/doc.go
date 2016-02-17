// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Command presubmit performs Vanadium presubmit related functions.

Usage:
   presubmit [flags] <command>

The presubmit commands are:
   query       Query open CLs from Gerrit
   result      Process and post test results
   test        Run tests for a CL
   help        Display help for commands or topics

The presubmit flags are:
 -color=true
   Use color to format output.
 -host=
   The Jenkins host. Presubmit will not send any CLs to an empty host.
 -job=vanadium-presubmit-test
   The name of the Jenkins job to add presubmit-test builds to.
 -url=https://vanadium-review.googlesource.com
   The base url of the gerrit instance.
 -v=false
   Print verbose output.

The global flags are:
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.
 -time=false
   Dump timing information to stderr before exiting the program.

Presubmit query - Query open CLs from Gerrit

This subcommand queries open CLs from Gerrit, calculates diffs from the previous
query results, and sends each one with related metadata (ref, project, changeId)
to a Jenkins job which will run tests against the corresponding CL and post
review with test results.

Usage:
   presubmit query [flags]

The presubmit query flags are:
 -log-file=${HOME}/tmp/presubmit_log
   The file that stores the refs from the previous Gerrit query.
 -manifest=
   Name of the project manifest.
 -query=(status:open -project:experimental)
   The string used to query Gerrit for open CLs.

 -color=true
   Use color to format output.
 -host=
   The Jenkins host. Presubmit will not send any CLs to an empty host.
 -job=vanadium-presubmit-test
   The name of the Jenkins job to add presubmit-test builds to.
 -url=https://vanadium-review.googlesource.com
   The base url of the gerrit instance.
 -v=false
   Print verbose output.

Presubmit result - Process and post test results

Result processes all the test statuses and results files collected from all the
presubmit test configuration builds, creates a result summary, and posts the
summary back to the corresponding Gerrit review thread.

Usage:
   presubmit result [flags]

The presubmit result flags are:
 -build-number=-1
   The number of the Jenkins build.
 -dashboard-host=https://dashboard.staging.v.io
   The host of the dashboard server.
 -manifest=
   Name of the project manifest.
 -projects=
   The base names of the remote projects containing the CLs pointed by the refs,
   separated by ':'.
 -refs=
   The review references separated by ':'.

 -color=true
   Use color to format output.
 -host=
   The Jenkins host. Presubmit will not send any CLs to an empty host.
 -job=vanadium-presubmit-test
   The name of the Jenkins job to add presubmit-test builds to.
 -url=https://vanadium-review.googlesource.com
   The base url of the gerrit instance.
 -v=false
   Print verbose output.

Presubmit test - Run tests for a CL

This subcommand pulls the open CLs from Gerrit, runs tests specified in a config
file, and posts test results back to the corresponding Gerrit review thread.

Usage:
   presubmit test [flags]

The presubmit test flags are:
 -build-number=-1
   The number of the Jenkins build.
 -manifest=
   Name of the project manifest.
 -projects=
   The base names of the remote projects containing the CLs pointed by the refs,
   separated by ':'.
 -refs=
   The review references separated by ':'.
 -test=
   The name of a single test to run.

 -color=true
   Use color to format output.
 -host=
   The Jenkins host. Presubmit will not send any CLs to an empty host.
 -job=vanadium-presubmit-test
   The name of the Jenkins job to add presubmit-test builds to.
 -url=https://vanadium-review.googlesource.com
   The base url of the gerrit instance.
 -v=false
   Print verbose output.

Presubmit help - Display help for commands or topics

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

Usage:
   presubmit help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The presubmit help flags are:
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
