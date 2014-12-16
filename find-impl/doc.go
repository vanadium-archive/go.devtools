// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
A simple tool to find implementations of a specified interface.

It uses golang.org/x/tools/{loader,types} to load and examine the types
of a collection of go files. The input must be valid go packages.

find-impl --interface=veyron2/security.Context <packages> will find all
implementations of veyron2/security.Context in the specified packages.

A common use case will be: cd <repo>/go/src find-impl
--interface=<package>.<interface> $(find * -type d)

The output is of the form: <type> in <file> no attempt is made to sort or dedup
this output, rather existing tools such as awk, sort, uniq should be used.

spaces in package names will confound simple scripts and the flags --only_files,
--only_types should be used in such cases to produce output with one type or one
filename per line.

The implementation is a brute force approach, taking two passes the first over
the entire space of types to find the named interface and then a second over the
set of implicit conversions to see which ones can be implemented using the type
found in the first pass. This appears to be fast enough for our immediate needs.

Usage:
   find-impl [flags] <command>
   find-impl [flags] <pkg ...>

The find-impl commands are:
   version     Print version
   help        Display help for commands or topics
Run "find-impl help [command]" for command usage.

<pkg ...> a list of packages to search

The find-impl flags are:
 -debug=false
   Toggle debugging.
 -interface=
   Name of the interface.
 -only_files=false
   Show only files.
 -only_types=false
   Show only types, takes precedence over only_files.
 -regexp=
   Look for implementations in packages matching this filter.
 -v=false
   Print verbose output.

Find-Impl Version

Print version of the veyron tool.

Usage:
   find-impl version

Find-Impl Help

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

The output is formatted to a target width in runes.  The target width is
determined by checking the environment variable CMDLINE_WIDTH, falling back on
the terminal width from the OS, falling back on 80 chars.  By setting
CMDLINE_WIDTH=x, if x > 0 the width is x, if x < 0 the width is unlimited, and
if x == 0 or is unset one of the fallbacks is used.

Usage:
   find-impl help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The find-impl help flags are:
 -style=text
   The formatting style for help output, either "text" or "godoc".
*/
package main
