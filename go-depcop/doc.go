// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
The go-depcop tool checks if a package imports respects outgoing and incoming
dependency constraints described in the GO.PACKAGE files.

go-depcop also enforces "internal" package rules.

GO.PACKAGE files are traversed hierarchically, from the deepmost package to
GOROOT, until a matching rule is found.  If no matching rule is found, the
default behavior is to allow the dependency, to stay compatible with existing
packages that do not include dependency rules.

GO.PACKAGE is a JSON file with a structure along the lines of:
   {
     "dependencies": {
       "outgoing": [
         {"allow": "allowpattern1/..."},
         {"deny": "denypattern"},
         {"allow": "pattern2"}
       ],
       "incoming": [
         {"allow": "pattern3"},
         {"deny": "pattern4"}
       ]
     }
   }

Usage:
   go-depcop [flags] <command>

The go-depcop commands are:
   check       Check package dependency constraints
   list        List outgoing package dependencies
   rlist       List incoming package dependencies
   version     Print version
   help        Display help for commands or topics
Run "go-depcop help [command]" for command usage.

The go-depcop flags are:
 -include_tests=false
   Include tests in computing dependencies.
 -v=false
   Print verbose output.

Go-Depcop Check

Check package dependency constraints.

Usage:
   go-depcop check [flags] <packages>

<packages> is a list of packages

The go-depcop check flags are:
 -r=false
   Check dependencies recursively.

Go-Depcop List

List outgoing package dependencies.

Usage:
   go-depcop list [flags] <packages>

<packages> is a list of packages

The go-depcop list flags are:
 -pretty_print=false
   Make output easy to read, indenting nested dependencies.
 -show_goroot=false
   Show packages in goroot.
 -transitive=false
   List transitive dependencies.

Go-Depcop Rlist

List incoming package dependencies.

Usage:
   go-depcop rlist <packages>

<packages> is a list of packages

Go-Depcop Version

Print version of the go-depcop tool.

Usage:
   go-depcop version

Go-Depcop Help

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

The output is formatted to a target width in runes.  The target width is
determined by checking the environment variable CMDLINE_WIDTH, falling back on
the terminal width from the OS, falling back on 80 chars.  By setting
CMDLINE_WIDTH=x, if x > 0 the width is x, if x < 0 the width is unlimited, and
if x == 0 or is unset one of the fallbacks is used.

Usage:
   go-depcop help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The go-depcop help flags are:
 -style=default
   The formatting style for help output, either "default" or "godoc".
*/
package main
