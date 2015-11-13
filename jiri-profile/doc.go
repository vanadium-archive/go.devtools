// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
NOTE: this command is deprecated, please use jiri v23-profile instead.

To facilitate development across different host platforms, vanadium defines
platform-independent "profiles" that map different platforms to a set of
libraries and tools that can be used for a facet of vanadium development.

Each profile can be in one of three states: absent, up-to-date, or out-of-date.
The subcommands of the profile command realize the following transitions:

  install:   absent => up-to-date
  update:    out-of-date => up-to-date
  uninstall: up-to-date or out-of-date => absent

In addition, a profile can transition from being up-to-date to out-of-date by
the virtue of a new version of the profile being released.

To enable cross-compilation, a profile can be installed for multiple targets. If
a profile supports multiple targets the above state transitions are applied on a
profile + target basis.

Usage:
   jiri profile [flags] <command>

The jiri profile commands are:
   install     Install the given vanadium profiles
   list        List known vanadium profiles
   setup       Set up the given vanadium profiles
   uninstall   Uninstall the given vanadium profiles
   update      Update the given vanadium profiles
   help        Display help for commands or topics

The jiri profile flags are:
 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

The global flags are:
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.
 -time=false
   Dump timing information to stderr before exiting the program.

Jiri profile install - Install the given vanadium profiles

Install the given vanadium profiles.

Usage:
   jiri profile install <profiles>

<profiles> is a list of profiles to install.

Jiri profile list - List known vanadium profiles

List known vanadium profiles.

Usage:
   jiri profile list

Jiri profile setup - Set up the given vanadium profiles

Set up the given vanadium profiles. This command is identical to 'install' and
is provided for backwards compatibility.

Usage:
   jiri profile setup <profiles>

<profiles> is a list of profiles to set up.

Jiri profile uninstall - Uninstall the given vanadium profiles

Uninstall the given vanadium profiles.

Usage:
   jiri profile uninstall <profiles>

<profiles> is a list of profiles to uninstall.

Jiri profile update - Update the given vanadium profiles

Update the given vanadium profiles.

Usage:
   jiri profile update <profiles>

<profiles> is a list of profiles to update.

Jiri profile help - Display help for commands or topics

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

Usage:
   jiri profile help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The jiri profile help flags are:
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
