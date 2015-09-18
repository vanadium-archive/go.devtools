// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Profiles provide a means of managing software dependencies that can be built
natively as well as being cross compiled. A profile generally manages a suite of
related software components that are required for a particular application (e.g.
for android development).

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
   xprofile [flags] <command>

The xprofile commands are:
   install     Install the given profiles
   list        List supported and installed profiles
   env         Display profile environment variables
   uninstall   Uninstall the given profiles
   update      Update the given profiles
   help        Display help for commands or topics

The xprofile flags are:
 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

The global flags are:
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.

Xprofile install - Install the given profiles

Install the given profiles.

Usage:
   xprofile install [flags] <profiles>

<profiles> is a list of profiles to install.

The xprofile install flags are:
 -env=
   specifcy an environment variable in the form: <var>=[<val>],...
 -manifest=$JIRI_ROOT/.jiri_profiles
   specify the XML manifest to file read/write from.
 -target=native=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form: [<tag>=]<arch>-<os>

Xprofile list - List supported and installed profiles

List supported and installed profiles.

Usage:
   xprofile list [flags]

The xprofile list flags are:
 -manifest=$JIRI_ROOT/.jiri_profiles
   specify the XML manifest to file read/write from.
 -show-manifest=false
   print out the manifest file

Xprofile env - Display profile environment variables

List profile specific and target specific environment variables. env
--profile=<profile-name> --tag=<tag as appears in a target> [env var name]*

If no environment variable names are requested then all will be printed.

Usage:
   xprofile env [flags] [<environment variable names>]

[<environment variable names>] is an optional list of environment variables to
display

The xprofile env flags are:
 -manifest=$JIRI_ROOT/.jiri_profiles
   specify the XML manifest to file read/write from.
 -profile=
   the profile whose environment is to be displayed
 -target=native=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form: [<tag>=]<arch>-<os>

Xprofile uninstall - Uninstall the given profiles

Uninstall the given profiles.

Usage:
   xprofile uninstall [flags] <profiles>

<profiles> is a list of profiles to uninstall.

The xprofile uninstall flags are:
 -env=
   specifcy an environment variable in the form: <var>=[<val>],...
 -manifest=$JIRI_ROOT/.jiri_profiles
   specify the XML manifest to file read/write from.
 -target=native=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form: [<tag>=]<arch>-<os>

Xprofile update - Update the given profiles

Update the given profiles.

Usage:
   xprofile update [flags] <profiles>

<profiles> is a list of profiles to update.

The xprofile update flags are:
 -env=
   specifcy an environment variable in the form: <var>=[<val>],...
 -force=false
   force an uninstall followed by install
 -manifest=$JIRI_ROOT/.jiri_profiles
   specify the XML manifest to file read/write from.
 -target=native=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form: [<tag>=]<arch>-<os>

Xprofile help - Display help for commands or topics

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

Usage:
   xprofile help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The xprofile help flags are:
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
