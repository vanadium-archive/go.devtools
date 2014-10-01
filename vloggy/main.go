// Below is the output from $(vloggy help -style=godoc ...)

/*
The vloggy tool can be used to:

1) ensure that all implementations in <packages> of all exported
interfaces declared in packages passed to the -interface flag have
an appropriate logging construct, and
2) automatically inject such logging constructs.

LIMITATIONS:

vloggy requires the "veyron.io/veyron/veyron2/vlog" to be
imported as "vlog".  Aliasing the log package
to another name makes vloggy ignore the calls.  Importing any
other package with the name "vlog" will
invoke undefined behavior.

Usage:
   vloggy [flags] <command>

The vloggy commands are:
   check       Check for log statements in public API implementations
   inject      Inject log statements in public API implementations
   selfupdate  Update the vloggy tool
   version     Print version
   help        Display help for commands

The vloggy flags are:
   -v=false: Print verbose output.

Vloggy Check

Check for log statements in public API implementations.

Usage:
   vloggy check [flags] <packages>

<packages> is the list of packages to be checked.

The check flags are:
   -interface=: Comma-separated list of interface packages (required)

Vloggy Inject

Inject log statements in public API implementations.
Note that inject modifies <packages> in-place.  It is a good idea
to commit changes to version control before running this tool so
you can see the diff or revert the changes.

Usage:
   vloggy inject [flags] <packages>

<packages> is the list of packages to inject log statements in.

The inject flags are:
   -gofmt=true: Automatically run gofmt on the modified files
   -interface=: Comma-separated list of interface packages (required)

Vloggy Selfupdate

Download and install the latest version of the vloggy tool.

Usage:
   vloggy selfupdate [flags]

The selfupdate flags are:
   -manifest=absolute: Name of the project manifest.

Vloggy Version

Print version of the vloggy tool.

Usage:
   vloggy version

Vloggy Help

Help displays usage descriptions for this command, or usage descriptions for
sub-commands.

Usage:
   vloggy help [flags] [command ...]

[command ...] is an optional sequence of commands to display detailed usage.
The special-case "help ..." recursively displays help for this command and all
sub-commands.

The help flags are:
   -style=text: The formatting style for help output, either "text" or "godoc".
*/
package main

import "tools/vloggy/impl"

func main() {
	impl.Root().Main()
}
