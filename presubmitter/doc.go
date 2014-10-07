// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
The presubmitter tool performs various presubmit related functions.

Usage:
   presubmitter [flags] <command>

The presubmitter commands are:
   query       Query open CLs from Gerrit
   post        Post review with the test results to Gerrit
   test        Run tests for a CL
   selfupdate  Update the presubmitter tool
   version     Print version
   help        Display help for commands or topics
Run "presubmitter help [command]" for command usage.

The presubmitter flags are:
   -host=: The Jenkins host. Presubmitter will not send any CLs to an empty host.
   -netrc=/var/veyron/.netrc: The path to the .netrc file that stores Gerrit's credentials.
   -token=: The Jenkins API token.
   -url=https://veyron-review.googlesource.com: The base url of the gerrit instance.
   -v=false: Print verbose output.

Presubmitter Query

This subcommand queries open CLs from Gerrit, calculates diffs from the previous
query results, and sends each one with related metadata (ref, repo, changeId) to
a Jenkins project which will run tests against the corresponding CL and post review
with test results.

Usage:
   presubmitter query [flags]

The query flags are:
   -log_file=/var/veyron/tmp/presubmitter_log: The file that stores the refs from the previous Gerrit query.
   -project=veyron-presubmit-test: The name of the Jenkins project to add presubmit-test builds to.
   -query=(status:open -project:experimental): The string used to query Gerrit for open CLs.

Presubmitter Post

This subcommand posts review with the test results to Gerrit.

Usage:
   presubmitter post [flags]

The post flags are:
   -msg=: The review message to post to Gerrit.
   -ref=: The ref where the review is posted.

Presubmitter Test

This subcommand pulls the open CLs from Gerrit, runs tests specified in a config
file, and posts test results back to the corresponding Gerrit review thread.

Usage:
   presubmitter test [flags]

The test flags are:
   -build_number=-1: The number of the Jenkins build.
   -conf=$VEYRON_ROOT/tools/go/src/tools/presubmitter/presubmit_tests.conf: The config file for presubmit tests.
   -manifest=absolute: Name of the project manifest.
   -ref=: The ref where the review is posted.
   -repo=: The URL of the repository containing the CL pointed by the ref.
   -tests_base_path=$VEYRON_ROOT/scripts/jenkins: The base path of all the test scripts.

Presubmitter Selfupdate

Download and install the latest version of the presubmitter tool.

Usage:
   presubmitter selfupdate [flags]

The selfupdate flags are:
   -manifest=absolute: Name of the project manifest.

Presubmitter Version

Print version of the presubmitter tool.

Usage:
   presubmitter version

Presubmitter Help

Help with no args displays the usage of the parent command.
Help with args displays the usage of the specified sub-command or help topic.
"help ..." recursively displays help for all commands and topics.

Usage:
   presubmitter help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The help flags are:
   -style=text: The formatting style for help output, either "text" or "godoc".
*/
package main
