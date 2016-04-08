// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Command vmon interacts with Google Cloud Monitoring.

Usage:
   vmon [flags] <command>

The vmon commands are:
   md          Manage metric descriptors in the given GCM instance
   check       Manage checks used for alerting and graphing
   help        Display help for commands or topics

The vmon flags are:
 -color=true
   Use color to format output.
 -key=
   The path to the service account's JSON credentials file.
 -project=
   The GCM's corresponding GCE project ID.
 -v=false
   Print verbose output.

The global flags are:
 -alsologtostderr=true
   log to standard error as well as files
 -log_backtrace_at=:0
   when logging hits line file:N, emit a stack trace
 -log_dir=
   if non-empty, write log files to this directory
 -logtostderr=false
   log to standard error instead of files
 -max_stack_buf_size=4292608
   max size in bytes of the buffer to use for logging stack traces
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.
 -stderrthreshold=2
   logs at or above this threshold go to stderr
 -time=false
   Dump timing information to stderr before exiting the program.
 -v=0
   log level for V logs
 -v23.credentials=
   directory to use for storing security credentials
 -v23.i18n-catalogue=
   18n catalogue files to load, comma separated
 -v23.namespace.root=[/(dev.v.io:role:vprod:service:mounttabled)@ns.dev.v.io:8101]
   local namespace root; can be repeated to provided multiple roots
 -v23.proxy=
   object name of proxy service to use to export services across network
   boundaries
 -v23.tcp.address=
   address to listen on
 -v23.tcp.protocol=wsh
   protocol to listen with
 -v23.vtrace.cache-size=1024
   The number of vtrace traces to store in memory.
 -v23.vtrace.collect-regexp=
   Spans and annotations that match this regular expression will trigger trace
   collection.
 -v23.vtrace.dump-on-shutdown=true
   If true, dump all stored traces on runtime shutdown.
 -v23.vtrace.sample-rate=0
   Rate (from 0.0 to 1.0) to sample vtrace traces.
 -v23.vtrace.v=0
   The verbosity level of the log messages to be captured in traces
 -vmodule=
   comma-separated list of globpattern=N settings for filename-filtered logging
   (without the .go suffix).  E.g. foo/bar/baz.go is matched by patterns baz or
   *az or b* but not by bar/baz or baz.go or az or b.*
 -vpath=
   comma-separated list of regexppattern=N settings for file pathname-filtered
   logging (without the .go suffix).  E.g. foo/bar/baz.go is matched by patterns
   foo/bar/baz or fo.*az or oo/ba or b.z but not by foo/bar/baz.go or fo*az

Vmon md - Manage metric descriptors in the given GCM instance

Metric descriptor defines the metadata for a custom metric. It includes the
metric's name, description, a set of labels, and its type. Before adding custom
metric data points to GCM, we need to create its metric descriptor (once).

Usage:
   vmon md [flags] <command>

The vmon md commands are:
   create      Create the given metric descriptor in GCM
   delete      Delete the given metric descriptor from GCM
   list        List known custom metric descriptors
   query       Query metric descriptors from GCM using the given filter

The vmon md flags are:
 -color=true
   Use color to format output.
 -key=
   The path to the service account's JSON credentials file.
 -project=
   The GCM's corresponding GCE project ID.
 -v=false
   Print verbose output.

Vmon md create - Create the given metric descriptor in GCM

Create the given metric descriptor in GCM.

Usage:
   vmon md create [flags] <names>

<names> is a list of metric descriptor names to create. Available: gce-instance,
jenkins, nginx, rpc-load-test, service-counters, service-counters-agg,
service-latency, service-latency-agg, service-metadata, service-metadata-agg,
service-permethod-latency, service-permethod-latency-agg, service-qps-method,
service-qps-method-agg, service-qps-total, service-qps-total-agg

The vmon md create flags are:
 -color=true
   Use color to format output.
 -key=
   The path to the service account's JSON credentials file.
 -project=
   The GCM's corresponding GCE project ID.
 -v=false
   Print verbose output.

Vmon md delete - Delete the given metric descriptor from GCM

Delete the given metric descriptor from GCM.

Usage:
   vmon md delete [flags] <names>

<names> is a list of metric descriptor names to delete. Available: gce-instance,
jenkins, nginx, rpc-load-test, service-counters, service-counters-agg,
service-latency, service-latency-agg, service-metadata, service-metadata-agg,
service-permethod-latency, service-permethod-latency-agg, service-qps-method,
service-qps-method-agg, service-qps-total, service-qps-total-agg

The vmon md delete flags are:
 -color=true
   Use color to format output.
 -key=
   The path to the service account's JSON credentials file.
 -project=
   The GCM's corresponding GCE project ID.
 -v=false
   Print verbose output.

Vmon md list - List known custom metric descriptors

List known custom metric descriptors.

Usage:
   vmon md list [flags]

The vmon md list flags are:
 -color=true
   Use color to format output.
 -key=
   The path to the service account's JSON credentials file.
 -project=
   The GCM's corresponding GCE project ID.
 -v=false
   Print verbose output.

Vmon md query - Query metric descriptors from GCM using the given filter

Query metric descriptors from GCM using the given filter.

Usage:
   vmon md query [flags]

The vmon md query flags are:
 -filter=metric.type=starts_with("custom.googleapis.com")
   The filter used for query. Default to only query custom metrics.

 -color=true
   Use color to format output.
 -key=
   The path to the service account's JSON credentials file.
 -project=
   The GCM's corresponding GCE project ID.
 -v=false
   Print verbose output.

Vmon check - Manage checks used for alerting and graphing

Manage checks whose results are used in GCM for alerting and graphing.

Usage:
   vmon check [flags] <command>

The vmon check commands are:
   list        List known checks
   run         Run the given checks

The vmon check flags are:
 -bin-dir=
   The path where all binaries are downloaded.
 -root=dev.v.io
   The blessings root.
 -v23.credentials=
   The path to v23 credentials.
 -v23.namespace.root=/ns.dev.v.io:8101
   The namespace root.

 -color=true
   Use color to format output.
 -key=
   The path to the service account's JSON credentials file.
 -project=
   The GCM's corresponding GCE project ID.
 -v=false
   Print verbose output.

Vmon check list - List known checks

List known checks.

Usage:
   vmon check list [flags]

The vmon check list flags are:
 -bin-dir=
   The path where all binaries are downloaded.
 -color=true
   Use color to format output.
 -key=
   The path to the service account's JSON credentials file.
 -project=
   The GCM's corresponding GCE project ID.
 -root=dev.v.io
   The blessings root.
 -v=false
   Print verbose output.
 -v23.credentials=
   The path to v23 credentials.
 -v23.namespace.root=/ns.dev.v.io:8101
   The namespace root.

Vmon check run - Run the given checks

Run the given checks.

Usage:
   vmon check run [flags] <names>

<names> is a list of names identifying the checks to run. Available:
gce-instance, jenkins, rpc-load-test, service-counters, service-latency,
service-metadata, service-permethod-latency, service-qps

The vmon check run flags are:
 -bin-dir=
   The path where all binaries are downloaded.
 -color=true
   Use color to format output.
 -key=
   The path to the service account's JSON credentials file.
 -project=
   The GCM's corresponding GCE project ID.
 -root=dev.v.io
   The blessings root.
 -v=false
   Print verbose output.
 -v23.credentials=
   The path to v23 credentials.
 -v23.namespace.root=/ns.dev.v.io:8101
   The namespace root.

Vmon help - Display help for commands or topics

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

Usage:
   vmon help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The vmon help flags are:
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
