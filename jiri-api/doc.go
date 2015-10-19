// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Use this command to ensure that no unintended changes are made to the vanadium
public API.

Usage:
   jiri api [flags] <command>

The jiri api commands are:
   check       Check if any changes have been made to the public API
   fix         Update api files to reflect changes to the public API
   help        Display help for commands or topics

The jiri api flags are:
 -color=true
   Use color to format output.
 -gotools-bin=
   The path to the gotools binary to use. If empty, gotools will be built if
   necessary.
 -manifest=
   Name of the project manifest.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

The global flags are:
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.
 -time=false
   Dump timing information to stderr before exiting the program.

Jiri api check - Check if any changes have been made to the public API

Check if any changes have been made to the public API.

Usage:
   jiri api check [flags] <projects>

<projects> is a list of vanadium projects to check. If none are specified, all
projects that require a public API check upon presubmit are checked.

The jiri api check flags are:
 -detailed=true
   If true, shows each API change in an expanded form. Otherwise, only a summary
   is shown.

Jiri api fix - Update api files to reflect changes to the public API

Update .api files to reflect changes to the public API.

Usage:
   jiri api fix <projects>

<projects> is a list of vanadium projects to update. If none are specified, all
project APIs are updated.

Jiri api help - Display help for commands or topics

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

Usage:
   jiri api help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The jiri api help flags are:
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
