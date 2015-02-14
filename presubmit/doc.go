// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
The presubmit tool performs various presubmit related functions.

Usage:
   presubmit [flags] <command>

The presubmit commands are:
   query       Query open CLs from Gerrit
   result      Process and post test results.
   test        Run tests for a CL
   version     Print version
   help        Display help for commands or topics
Run "presubmit help [command]" for command usage.

The presubmit flags are:
 -host=
   The Jenkins host. Presubmit will not send any CLs to an empty host.
 -job=vanadium-presubmit-test
   The name of the Jenkins job to add presubmit-test builds to.
 -n=false
   Show what commands will run but do not execute them.
 -netrc=$HOME/.netrc
   The path to the .netrc file that stores Gerrit's credentials.
 -nocolor=false
   Do not use color to format output.
 -url=https://vanadium-review.googlesource.com
   The base url of the gerrit instance.
 -v=false
   Print verbose output.

Presubmit Query

This subcommand queries open CLs from Gerrit, calculates diffs from the previous
query results, and sends each one with related metadata (ref, project, changeId)
to a Jenkins job which will run tests against the corresponding CL and post
review with test results.

Usage:
   presubmit query [flags]

The presubmit query flags are:
 -log_file=$HOME/tmp/presubmit_log
   The file that stores the refs from the previous Gerrit query.
 -query=(status:open -project:experimental)
   The string used to query Gerrit for open CLs.

Presubmit Result

Result processes all the test statuses and results files collected from all the
presubmit test configuration builds, creates a result summary, and posts the
summary back to the corresponding Gerrit review thread.

Usage:
   presubmit result [flags]

The presubmit result flags are:
 -build_number=-1
   The number of the Jenkins build.
 -projects=
   The base names of the remote projects containing the CLs pointed by the refs,
   separated by ':'.
 -refs=
   The review references separated by ':'.

Presubmit Test

This subcommand pulls the open CLs from Gerrit, runs tests specified in a config
file, and posts test results back to the corresponding Gerrit review thread.

Usage:
   presubmit test [flags]

The presubmit test flags are:
 -build_number=-1
   The number of the Jenkins build.
 -manifest=default
   Name of the project manifest.
 -projects=
   The base names of the remote projects containing the CLs pointed by the refs,
   separated by ':'.
 -refs=
   The review references separated by ':'.
 -test=
   The name of a single test to run.

Presubmit Version

Print version of the presubmit tool.

Usage:
   presubmit version

Presubmit Help

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

The output is formatted to a target width in runes.  The target width is
determined by checking the environment variable CMDLINE_WIDTH, falling back on
the terminal width from the OS, falling back on 80 chars.  By setting
CMDLINE_WIDTH=x, if x > 0 the width is x, if x < 0 the width is unlimited, and
if x == 0 or is unset one of the fallbacks is used.

Usage:
   presubmit help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The presubmit help flags are:
 -style=text
   The formatting style for help output, either "text" or "godoc".
*/
package main
