// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Profiles are used to manage external sofware dependencies and offer a balance
between providing no support at all and a full blown package manager. Profiles
can be built natively as well as being cross compiled. A profile is a named
collection of software required for a given system component or application.
Current example profiles include 'syncbase' which consists of the leveldb and
snappy libraries or 'android' which consists of all of the android components
and downloads needed to build android applications. Profiles are built for
specific targets.

Targets

Profiles generally refer to uncompiled source code that needs to be compiled for
a specific "target". Targets hence represent compiled code and consist of:

1. An 'architecture' that refers to the CPU to be generate code for

2. An 'operating system' that refers to the operating system to generate code
for

3. A lexicographically orderd set of supported versions, one of which is
designated as the default.

4. An 'environment' which is a set of environment variables to use when
compiling the profile

Targets thus provide the basic support needed for cross compilation.

Targets are versioned and multiple versions may be installed and used
simultaneously. Versions are ordered lexicographically and each target specifies
a 'default' version to be used when a specific version is not explicitly
requested. A request to 'upgrade' the profile will result in the installation of
the default version of the targets currently installed if that default version
is not already installed.

The Supported Commands

Profiles, or more correctly, targets for specific profiles may be installed or
removed. When doing so, the name of the profile is required, but the other
components of the target are optional and will default to the values of the
system that the commands are run on (so-called native builds) and the default
version for that target. Once a profile is installed it may be referred to by
its tag for subsequent removals.

The are also update and cleanup commands. Update installs the default version of
the requested profile or for all profiles for the already installed targets.
Cleanup will uninstall targets whose version is older than the default.

Finally, there are commands to list the available and installed profiles and to
access the environment variables specified and stored in each profile
installation and a command (recreate) to generate a list of commands that can be
run to recreate the currently installed profiles.

The Profiles Database

The profiles packages manages a database that tracks the installed profiles and
their configurations. Other command line tools and packages are expected to read
information about the currently installed profiles from this database via the
profiles package. The profile command line tools support displaying the database
(via the list command) or for specifying an alternate version of the file (via
the -profiles-db flag) which is generally useful for debugging.

Adding Profiles

Profiles are intended to be provided as go packages that register themselves
with the profile command line tools via the *v.io/jiri/profiles* package. They
must implement the interfaces defined by that package and be imported (e.g.
import _ "myprofile") by the command line tools that are to use them.

Usage:
   jiri v23-profile [flags] <command>

The jiri v23-profile commands are:
   install     Install the given profiles
   uninstall   Uninstall the given profiles
   update      Install the latest default version of the given profiles
   cleanup     Cleanup the locally installed profiles
   list        List available or installed profiles
   env         Display profile environment variables
   help        Display help for commands or topics

The jiri v23-profile flags are:
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

Jiri v23-profile install - Install the given profiles

Install the given profiles.

Usage:
   jiri v23-profile install [flags] <profiles>

<profiles> is a list of profiles to install.

The jiri v23-profile install flags are:
 -env=
   specify an environment variable in the form: <var>=[<val>],...
 -force=false
   force install the profile even if it is already installed
 -go.sysroot-image=
   sysroot image for cross compiling to the currently specified target
 -go.sysroot-image-dirs-to-use=/lib:/usr/lib:/usr/include
   a colon separated list of directories to use from the sysroot image
 -mojo-dev.dir=
   Path of mojo repo checkout.
 -profile-dir=profiles
   the directory, relative to JIRI_ROOT, that profiles are installed in
 -profiles-db=$JIRI_ROOT/.jiri_v23_profiles
   specify the profiles XML manifest filename.
 -target=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form:
   <arch>-<os>[@<version>]|<arch>-<val>[@<version>]

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri v23-profile uninstall - Uninstall the given profiles

Uninstall the given profiles.

Usage:
   jiri v23-profile uninstall [flags] <profiles>

<profiles> is a list of profiles to uninstall.

The jiri v23-profile uninstall flags are:
 -all-targets=false
   apply to all targets for the specified profile(s)
 -go.sysroot-image=
   sysroot image for cross compiling to the currently specified target
 -go.sysroot-image-dirs-to-use=/lib:/usr/lib:/usr/include
   a colon separated list of directories to use from the sysroot image
 -profile-dir=profiles
   the directory, relative to JIRI_ROOT, that profiles are installed in
 -profiles-db=$JIRI_ROOT/.jiri_v23_profiles
   specify the profiles XML manifest filename.
 -target=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form:
   <arch>-<os>[@<version>]|<arch>-<val>[@<version>]
 -v=false
   print more detailed information

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.

Jiri v23-profile update - Install the latest default version of the given profiles

Install the latest default version of the given profiles.

Usage:
   jiri v23-profile update [flags] <profiles>

<profiles> is a list of profiles to update, if omitted all profiles are updated.

The jiri v23-profile update flags are:
 -profile-dir=profiles
   the directory, relative to JIRI_ROOT, that profiles are installed in
 -profiles-db=$JIRI_ROOT/.jiri_v23_profiles
   specify the profiles XML manifest filename.
 -v=false
   print more detailed information

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.

Jiri v23-profile cleanup - Cleanup the locally installed profiles

Cleanup the locally installed profiles. This is generally required when
recovering from earlier bugs or when preparing for a subsequent change to the
profiles implementation.

Usage:
   jiri v23-profile cleanup [flags] <profiles>

<profiles> is a list of profiles to cleanup, if omitted all profiles are
cleaned.

The jiri v23-profile cleanup flags are:
 -ensure-specific-versions-are-set=false
   ensure that profile targets have a specific version set
 -gc=false
   uninstall profile targets that are older than the current default
 -profile-dir=profiles
   the directory, relative to JIRI_ROOT, that profiles are installed in
 -profiles-db=$JIRI_ROOT/.jiri_v23_profiles
   specify the profiles XML manifest filename.
 -rewrite-profiles-manifest=false
   rewrite the profiles manifest file to use the latest schema version
 -rm-all=false
   remove profiles manifest and all profile generated output files.
 -v=false
   print more detailed information

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.

Jiri v23-profile list - List available or installed profiles

List available or installed profiles.

Usage:
   jiri v23-profile list [flags] [<profiles>]

<profiles> is a list of profiles to list, defaulting to all profiles if none are
specifically requested.

The jiri v23-profile list flags are:
 -available=false
   print the list of available profiles
 -env=
   specify an environment variable in the form: <var>=[<val>],...
 -info=
   The following fields for use with --profile-info are available:
   	SchemaVersion - the version of the profiles implementation.
   	Target.InstallationDir - the installation directory of the requested profile.
   	Target.CommandLineEnv - the environment variables specified via the command line when installing this profile target.
   	Target.Env - the environment variables computed by the profile installation process for this target.
   	Target.Command - a command that can be used to create this profile.
   	Note: if no --target is specified then the requested field will be displayed for all targets.
   	Profile.Description - description of the requested profile.
   	Profile.Root - the root directory of the requested profile.
   	Profile.Versions - the set of supported versions for this profile.
   	Profile.DefaultVersion - the default version of the requested profile.
   	Profile.LatestVersion - the latest version available for the requested profile.
   	Note: if no profiles are specified then the requested field will be displayed for all profiles.
 -merge-policies=+CCFLAGS,+CGO_CFLAGS,+CGO_CXXFLAGS,+CGO_LDFLAGS,+CXXFLAGS,GOARCH,GOOS,GOPATH:,^GOROOT*,+LDFLAGS,:PATH,VDLPATH:
   specify policies for merging environment variables
 -profiles=base,jiri
   a comma separated list of profiles to use
 -profiles-db=$JIRI_ROOT/.jiri_v23_profiles
   specify the profiles XML manifest filename.
 -show-profiles-db=false
   print out the profiles database file
 -skip-profiles=false
   if set, no profiles will be used
 -target=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form:
   <arch>-<os>[@<version>]|<arch>-<val>[@<version>]
 -v=false
   print more detailed information

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.

Jiri v23-profile env - Display profile environment variables

List profile specific and target specific environment variables. If the
requested environment variable name ends in = then only the value will be
printed, otherwise both name and value are printed, i.e. GOPATH="foo" vs just
"foo".

If no environment variable names are requested then all will be printed in
<name>=<val> format.

Usage:
   jiri v23-profile env [flags] [<environment variable names>]

[<environment variable names>] is an optional list of environment variables to
display

The jiri v23-profile env flags are:
 -env=
   specify an environment variable in the form: <var>=[<val>],...
 -merge-policies=+CCFLAGS,+CGO_CFLAGS,+CGO_CXXFLAGS,+CGO_LDFLAGS,+CXXFLAGS,GOARCH,GOOS,GOPATH:,^GOROOT*,+LDFLAGS,:PATH,VDLPATH:
   specify policies for merging environment variables
 -profiles=base,jiri
   a comma separated list of profiles to use
 -profiles-db=$JIRI_ROOT/.jiri_v23_profiles
   specify the profiles XML manifest filename.
 -skip-profiles=false
   if set, no profiles will be used
 -target=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form:
   <arch>-<os>[@<version>]|<arch>-<val>[@<version>]
 -v=false
   print more detailed information

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.

Jiri v23-profile help - Display help for commands or topics

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

Usage:
   jiri v23-profile help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The jiri v23-profile help flags are:
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
