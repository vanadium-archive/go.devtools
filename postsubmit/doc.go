// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
The postsubmit tool performs various postsubmit related functions.

Usage:
   postsubmit [flags] <command>

The postsubmit commands are:
   poll        Poll changes and start corresponding builds on Jenkins
   version     Print version
   help        Display help for commands or topics
Run "postsubmit help [command]" for command usage.

The postsubmit flags are:
 -color=true
   Use color to format output.
 -host=
   The Jenkins host. Presubmit will not send any CLs to an empty host.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Postsubmit Poll

Poll changes and start corresponding builds on Jenkins.

Usage:
   postsubmit poll [flags]

The postsubmit poll flags are:
 -manifest=
   Name of the project manifest.

Postsubmit Version

Print version of the postsubmit tool.

Usage:
   postsubmit version

Postsubmit Help

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

The output is formatted to a target width in runes.  The target width is
determined by checking the environment variable CMDLINE_WIDTH, falling back on
the terminal width from the OS, falling back on 80 chars.  By setting
CMDLINE_WIDTH=x, if x > 0 the width is x, if x < 0 the width is unlimited, and
if x == 0 or is unset one of the fallbacks is used.

Usage:
   postsubmit help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The postsubmit help flags are:
 -style=default
   The formatting style for help output, either "default" or "godoc".
*/
package main
