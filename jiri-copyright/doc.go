// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
This command can be used to check if all source code files of Vanadium projects
contain the appropriate copyright header and also if all projects contains the
appropriate licensing files. Optionally, the command can be used to fix the
appropriate copyright headers and licensing files.

In order to ignore checked in third-party assets which have their own copyright
and licensing headers a ".jiriignore" file can be added to a project. The
".jiriignore" file is expected to contain a single regular expression pattern
per line.

Usage:
   jiri copyright [flags] <command>

The jiri copyright commands are:
   check       Check copyright headers and licensing files
   fix         Fix copyright headers and licensing files
   help        Display help for commands or topics

The jiri copyright flags are:
 -color=true
   Use color to format output.
 -manifest=
   Name of the project manifest.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

The global flags are:
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.

Jiri copyright check - Check copyright headers and licensing files

Check copyright headers and licensing files.

Usage:
   jiri copyright check <projects>

<projects> is a list of projects to check.

Jiri copyright fix - Fix copyright headers and licensing files

Fix copyright headers and licensing files.

Usage:
   jiri copyright fix <projects>

<projects> is a list of projects to fix.

Jiri copyright help - Display help for commands or topics

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

Usage:
   jiri copyright help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The jiri copyright help flags are:
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
