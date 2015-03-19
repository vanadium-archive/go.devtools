// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
The vmon tool performs various operatios using GCM APIs.

Usage:
   vmon [flags] <command>

The vmon commands are:
   md          The 'md' command manages metric descriptors in the given GCM
               instance
   check       Manage checks whose results are used in GCM for alerting and
               graphing
   help        Display help for commands or topics
Run "vmon help [command]" for command usage.

The vmon flags are:
 -account=
   The service account used to communicate with GCM.
 -color=true
   Use color to format output.
 -key=
   The path to the service account's key file.
 -project=
   The GCM's corresponding GCE project ID.
 -v=false
   Print verbose output.

Vmon Md

Metric descriptor defines the metadata for a custom metric. It includes the
metric's name, description, a set of labels, and its type. Before adding custom
metric data points to GCM, we need to create its metric descriptor (once).

Usage:
   vmon md <command>

The vmon md commands are:
   create      Create the given metric descriptor in GCM
   delete      Delete the given metric descriptor from GCM
   list        List known custom metric descriptors
   query       Query metric descriptors from GCM using the given filter

Vmon Md Create

Create the given metric descriptor in GCM.

Usage:
   vmon md create <names>

<names> is a list of metric descriptor names to create. Available:
service-latency

Vmon Md Delete

Delete the given metric descriptor from GCM.

Usage:
   vmon md delete <names>

<names> is a list of metric descriptor names to delete. Available:
service-latency

Vmon Md List

List known custom metric descriptors.

Usage:
   vmon md list

Vmon Md Query

Query metric descriptors from GCM using the given filter.

Usage:
   vmon md query [flags]

The vmon md query flags are:
 -filter=custom.cloudmonitoring.googleapis.com
   The filter used for query. Default to only query custom metrics.

Vmon Check

Manage checks whose results are used in GCM for alerting and graphing.

Usage:
   vmon check [flags] <command>

The vmon check commands are:
   list        List known checks
   run         Run the given checks

The vmon check flags are:
 -bin_dir=
   The path where all binaries are downloaded.
 -ns=/ns.dev.v.io:8101
   The namespace root.

Vmon Check List

List known checks.

Usage:
   vmon check list

Vmon Check Run

Run the given checks.

Usage:
   vmon check run <names>

<names> is a list of names identifying the checks to run. Available:
service-latency

Vmon Help

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

The output is formatted to a target width in runes.  The target width is
determined by checking the environment variable CMDLINE_WIDTH, falling back on
the terminal width from the OS, falling back on 80 chars.  By setting
CMDLINE_WIDTH=x, if x > 0 the width is x, if x < 0 the width is unlimited, and
if x == 0 or is unset one of the fallbacks is used.

Usage:
   vmon help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The vmon help flags are:
 -style=default
   The formatting style for help output, either "default" or "godoc".
*/
package main
