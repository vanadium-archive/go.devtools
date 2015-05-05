// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Command postsubmit performs Vanadium postsubmit related functions.

Usage:
   postsubmit [flags] <command>

The postsubmit commands are:
   poll        Poll changes and start corresponding builds on Jenkins
   version     Print version
   help        Display help for commands or topics

The postsubmit flags are:
 -color=true
   Use color to format output.
 -host=
   The Jenkins host. Presubmit will not send any CLs to an empty host.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

The global flags are:
 -v23.metadata=<just specify -v23.metadata to activate>
   Displays metadata for the program and exits.

Postsubmit poll

Poll changes and start corresponding builds on Jenkins.

Usage:
   postsubmit poll [flags]

The postsubmit poll flags are:
 -manifest=
   Name of the project manifest.

Postsubmit version

Print version of the postsubmit tool.

Usage:
   postsubmit version

Postsubmit help

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

Output is formatted to a target width in runes, determined by checking the
CMDLINE_WIDTH environment variable, falling back on the terminal width, falling
back on 80 chars.  By setting CMDLINE_WIDTH=x, if x > 0 the width is x, if x < 0
the width is unlimited, and if x == 0 or is unset one of the fallbacks is used.

Usage:
   postsubmit help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The postsubmit help flags are:
 -style=compact
   The formatting style for help output:
      compact - Good for compact cmdline output.
      full    - Good for cmdline output, shows all global flags.
      godoc   - Good for godoc processing.
   Override the default by setting the CMDLINE_STYLE environment variable.
*/
package main
