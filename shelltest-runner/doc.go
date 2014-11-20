// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Tool for running shell test scripts.

Usage:
   shelltest-runner [flags] <command>
   shelltest-runner [flags]

The shelltest-runner commands are:
   version     Print version
   help        Display help for commands or topics
Run "shelltest-runner help [command]" for command usage.

The shelltest-runner flags are:
 -bin_dir=$TMPDIR/bin
   The binary directory.
 -v=false
   Print verbose output.
 -workers=0
   Number of test workers. The default 0 matches the number of CPUs.

Shelltest-Runner Version

Print version of the shelltest-runner tool.

Usage:
   shelltest-runner version

Shelltest-Runner Help

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

The output is formatted to a target width in runes.  The target width is
determined by checking the environment variable CMDLINE_WIDTH, falling back on
the terminal width from the OS, falling back on 80 chars.  By setting
CMDLINE_WIDTH=x, if x > 0 the width is x, if x < 0 the width is unlimited, and
if x == 0 or is unset one of the fallbacks is used.

Usage:
   shelltest-runner help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The shelltest-runner help flags are:
 -style=text
   The formatting style for help output, either "text" or "godoc".
*/
package main
