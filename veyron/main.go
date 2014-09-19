// Below is the output from $(veyron help -style=godoc ...)

/*
The veyron tool facilitates interaction with veyron projects.

Usage:
   veyron [flags] <command>

The veyron commands are:
   profile     Manage veyron profiles
   project     Manage veyron projects
   run         Execute a command using the veyron environment
   go          Execute the go build tool using the veyron environment
   selfupdate  Update the veyron tool
   version     Print version
   help        Display help for commands

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
   help        Display help for commands

Veyron Profile List

Inspect the host platform and list supported profiles.

Usage:
   veyron profile list

Veyron Profile Setup

Set up the given veyron profiles.

Usage:
   veyron profile setup <profiles>

<profiles> is a list of profiles to set up.

Veyron Profile Help

Help displays usage descriptions for this command, or usage descriptions for
sub-commands.

Usage:
   veyron profile help [flags] <command>

<command> is an optional sequence of commands to display detailed per-command
usage.  The special-case "help ..." recursively displays help for this command
and all sub-commands.

The help flags are:
   -style=text: The formatting style for help output, either "text" or "godoc".

Veyron Project

Manage veyron projects.

Usage:
   veyron project <command>

The project commands are:
   list        List existing veyron projects
   update      Update veyron projects
   help        Display help for commands

Veyron Project List

Inspect the local filesystem and list the existing projects.

Usage:
   veyron project list [flags]

The list flags are:
   -branches=none: Determines what project branches to list (none, all).

Veyron Project Update

Update the local master branch of veyron projects by pulling from
the remote master. The projects to be updated are specified as a list
of arguments. If no project is specified, the default behavior is to
update all projects.

Usage:
   veyron project update [flags] <projects>

<projects> is a list of projects to update.

The update flags are:
   -gc=false: Garbage collect obsolete repositories.
   -manifest=absolute: Name of the project manifest.

Veyron Project Help

Help displays usage descriptions for this command, or usage descriptions for
sub-commands.

Usage:
   veyron project help [flags] <command>

<command> is an optional sequence of commands to display detailed per-command
usage.  The special-case "help ..." recursively displays help for this command
and all sub-commands.

The help flags are:
   -style=text: The formatting style for help output, either "text" or "godoc".

Veyron Run

Execute a command using the veyron environment.

Usage:
   veyron run <command> <args>

<command> <args> is a command and list of its arguments

Veyron Go

Wrapper around the 'go' tool that takes care of veyron-specific setup,
such as setting up the GOPATH or making sure that VDL generated files
are regenerated before compilation.

Usage:
   veyron go [flags] <args>

<args> is a list for the arguments for the Go tool.

The go flags are:
   -novdl=false: Disable automatic generation of vdl files.

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

Help displays usage descriptions for this command, or usage descriptions for
sub-commands.

Usage:
   veyron help [flags] <command>

<command> is an optional sequence of commands to display detailed per-command
usage.  The special-case "help ..." recursively displays help for this command
and all sub-commands.

The help flags are:
   -style=text: The formatting style for help output, either "text" or "godoc".
*/
package main

import (
	"tools/veyron/impl"
)

func main() {
	impl.Root().Main()
}
