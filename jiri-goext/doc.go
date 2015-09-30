// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Vanadium extension of the go tool.

Usage:
   jiri goext [flags] <command>

The jiri goext commands are:
   distclean   Restore the vanadium Go workspaces to their pristine state
   help        Display help for commands or topics

The jiri goext flags are:
 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

The global flags are:
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.

Jiri goext distclean - Restore the vanadium Go workspaces to their pristine state

Unlike the 'go clean' command, which only removes object files for packages in
the source tree, the 'goext disclean' command removes all object files from
vanadium Go workspaces. This functionality is needed to avoid accidental use of
stale object files that correspond to packages that no longer exist in the
source tree.

Usage:
   jiri goext distclean

Jiri goext help - Display help for commands or topics

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

Usage:
   jiri goext help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The jiri goext help flags are:
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
