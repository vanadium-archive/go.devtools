// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
List project contributors.

Usage:
   jiri contributors [flags] <command>

The jiri contributors commands are:
   contributors List project contributors
   help         Display help for commands or topics

The global flags are:
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.
 -time=false
   Dump timing information to stderr before exiting the program.

Jiri contributors contributors - List project contributors

Lists project contributors. Projects to consider can be specified as an
argument. If no projects are specified, all projects in the current manifest are
considered by default.

Usage:
   jiri contributors contributors [flags] <projects>

<projects> is a list of projects to consider.

The jiri contributors contributors flags are:
 -aliases=
   Path to the aliases file.
 -n=false
   Show number of contributions.

Jiri contributors help - Display help for commands or topics

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

Usage:
   jiri contributors help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The jiri contributors help flags are:
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
