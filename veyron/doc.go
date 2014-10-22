// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
The veyron tool helps manage veyron development.

Usage:
   veyron [flags] <command>

The veyron commands are:
   contributors List veyron project contributors
   profile     Manage veyron profiles
   project     Manage veyron projects
   update      Update all veyron tools and projects
   env         Print veyron environment variables
   run         Run an executable using the veyron environment
   go          Execute the go tool using the veyron environment
   goext       Veyron extensions of the go tool
   xgo         Execute the go tool using the veyron environment and cross-compilation
   version     Print version
   help        Display help for commands or topics
Run "veyron help [command]" for command usage.

The veyron flags are:
   -v=false: Print verbose output.

The global flags are:
   -host-go=go: Go command for the host platform.
   -target-go=go: Go command for the target platform.

Veyron Contributors

Lists veyron project contributors and the number of their
commits. Veyron projects to consider can be specified as an
argument. If no projects are specified, all veyron projects are
considered by default.

Usage:
   veyron contributors <projects>

<projects> is a list of projects to consider.

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

Veyron Project List

Inspect the local filesystem and list the existing projects.

Usage:
   veyron project list [flags]

The list flags are:
   -branches=false: Show project branches.

Veyron Project Poll

Poll existing veyron projects and report whether any new changes exist.
Projects to poll can be specified as command line arguments.
If no projects are specified, all projects are polled by default.

Usage:
   veyron project poll <project ...>

<project ...> is a list of projects to poll.

Veyron Update

Updates all veyron tools to their latest version, installing them
into $VEYRON_ROOT/bin, and then updates all veyron projects. The
sequence in which the individual updates happen guarantees that we
end up with a consistent set of tools and source code.

Usage:
   veyron update [flags]

The update flags are:
   -gc=false: Garbage collect obsolete repositories.
   -manifest=manifest/v1/default: Name of the project manifest.

Veyron Env

Print veyron environment variables.

If no arguments are given, prints all variables in NAME="VALUE" format,
each on a separate line ordered by name.  This format makes it easy to set
all vars by running the following bash command (or similar for other shells):
   eval $(veyron env)

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
   veyron xgo [flags] <platform> <arg ...>

<platform> is the cross-compilation target and has the general format
<arch><sub>-<os> or <arch><sub>-<os>-<env> where:
- <arch> is the platform architecture (e.g. 386, amd64 or arm)
- <sub> is the platform sub-architecture (e.g. v6 or v7 for arm)
- <os> is the platform operating system (e.g. linux or darwin)
- <env> is the platform environment (e.g. gnu or android)

<arg ...> is a list of arguments for the go tool."

The xgo flags are:
   -novdl=false: Disable automatic generation of vdl files.

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
