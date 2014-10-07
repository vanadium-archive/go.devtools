// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
The veyron tool helps manage veyron development.

Usage:
   veyron [flags] <command>

The veyron commands are:
   profile     Manage veyron profiles
   project     Manage veyron projects
   env         Print veyron environment variables
   run         Run an executable using the veyron environment
   go          Execute the go tool using the veyron environment
   goext       Veyron extensions of the go tool
   xgo         Execute the go tool using the veyron environment and cross-compilation
   selfupdate  Update the veyron tool
   version     Print version
   help        Display help for commands or topics
Run "veyron help [command]" for command usage.

The veyron flags are:
   -v=false: Print verbose output.

Veyron Profile

To facilitate development across different platforms, veyron defines
platform-independent profiles that map different platforms to a set
of libraries and tools that can be used for a factor of veyron
development.

Usage:
   veyron profile <command>

The profile commands are:
   list        List supported veyron profiles
   setup       Set up the given veyron profiles

Veyron Profile List

Inspect the host platform and list supported profiles.

Usage:
   veyron profile list

Veyron Profile Setup

Set up the given veyron profiles.

Usage:
   veyron profile setup <profiles>

<profiles> is a list of profiles to set up.

Veyron Project

Manage veyron projects.

Usage:
   veyron project <command>

The project commands are:
   list        List existing veyron projects
   poll        Poll existing veyron projects
   update      Update veyron projects

Veyron Project List

Inspect the local filesystem and list the existing projects.

Usage:
   veyron project list [flags]

The list flags are:
   -branches=false: Show project branches.

Veyron Project Poll

Poll existing veyron projects and report whether any new changes exist.

Usage:
   veyron project poll

Veyron Project Update

Update the local projects to match the state of the remote projects
identified by a project manifest. The projects to be updated are
specified as a list of arguments. If no project is specified, the
default behavior is to update all projects.

Usage:
   veyron project update [flags] <projects>

<projects> is a list of projects to update.

The update flags are:
   -gc=false: Garbage collect obsolete repositories.
   -manifest=absolute: Name of the project manifest.

Veyron Env

Print veyron environment variables.

If no arguments are given, prints all variables in NAME=VALUE format,
each on a separate line ordered by name.

If arguments are given, prints only the value of each named variable,
each on a separate line in the same order as the arguments.

Usage:
   veyron env [flags] [name ...]

[name ...] is an optional list of variable names.

The env flags are:
   -platform=: Target platform.

Veyron Run

Run an executable using the veyron environment.

Usage:
   veyron run <executable> [arg ...]

<executable> [arg ...] is the executable to run and any arguments to pass
verbatim to the executable.

Veyron Go

Wrapper around the 'go' tool that can be used for compilation of
veyron Go sources. It takes care of veyron-specific setup, such as
setting up the Go specific environment variables or making sure that
VDL generated files are regenerated before compilation.

In particular, the tool invokes the following command before invoking
any go tool commands that compile veyron Go code:

vdl generate -lang=go all

Usage:
   veyron go [flags] <arg ...>

<arg ...> is a list of arguments for the go tool.

The go flags are:
   -novdl=false: Disable automatic generation of vdl files.

Veyron Goext

Veyron extension of the go tool.

Usage:
   veyron goext <command>

The goext commands are:
   distclean   Restore the veyron Go repositories to their pristine state

Veyron Goext Distclean

Unlike the 'go clean' command, which only removes object files for
packages in the source tree, the 'goext disclean' command removes all
object files from veyron Go workspaces. This functionality is needed
to avoid accidental use of stale object files that correspond to
packages that no longer exist in the source tree.

Usage:
   veyron goext distclean

Veyron Xgo

Wrapper around the 'go' tool that can be used for cross-compilation of
veyron Go sources. It takes care of veyron-specific setup, such as
setting up the Go specific environment variables or making sure that
VDL generated files are regenerated before compilation.

In particular, the tool invokes the following command before invoking
any go tool commands that compile veyron Go code:

vdl generate -lang=go all

Usage:
   veyron xgo <platform> <arg ...>

<platform> is the cross-compilation target and has the general format
<arch><sub>-<os> or <arch><sub>-<os>-<env> where:
- <arch> is the platform architecture (e.g. 386, amd64 or arm)
- <sub> is the platform sub-architecture (e.g. v6 or v7 for arm)
- <os> is the platform operating system (e.g. linux or darwin)
- <env> is the platform environment (e.g. gnu or android)

<arg ...> is a list of arguments for the go tool."

Veyron Selfupdate

Download and install the latest version of the veyron tool.

Usage:
   veyron selfupdate [flags]

The selfupdate flags are:
   -manifest=absolute: Name of the project manifest.

Veyron Version

Print version of the veyron tool.

Usage:
   veyron version

Veyron Help

Help with no args displays the usage of the parent command.
Help with args displays the usage of the specified sub-command or help topic.
"help ..." recursively displays help for all commands and topics.

Usage:
   veyron help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The help flags are:
   -style=text: The formatting style for help output, either "text" or "godoc".
*/
package main
