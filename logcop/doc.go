// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Command logcop checks for and injects logging statements into Go source code.

When checking, it ensures that all implementations in <packages> of all exported
interfaces declared in packages passed to the -interface flag have an
appropriate logging construct.

When injecting, it modifies the source code to inject such logging constructs.

LIMITATIONS:

logcop requires the "v.io/x/lib/vlog" to be imported as "vlog".  Aliasing the
log package to another name makes logcop ignore the calls.  Importing any other
package with the name "vlog" will invoke undefined behavior.

Usage:
   logcop [flags] <command>

The logcop commands are:
   check       Check for log statements in public API implementations
   inject      Inject log statements in public API implementations
   remove      Remove log statements
   version     Print version
   help        Display help for commands or topics

The logcop flags are:
 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -progress=false
   Print verbose progress information.
 -v=false
   Print verbose output.

Logcop check

Check for log statements in public API implementations.

Usage:
   logcop check [flags] <packages>

<packages> is the list of packages to be checked.

The logcop check flags are:
 -interface=
   Comma-separated list of interface packages (required).

Logcop inject

Inject log statements in public API implementations. Note that inject modifies
<packages> in-place.  It is a good idea to commit changes to version control
before running this tool so you can see the diff or revert the changes.

Usage:
   logcop inject [flags] <packages>

<packages> is the list of packages to inject log statements in.

The logcop inject flags are:
 -diff-only=false
   Show changes that would be made without actually making them.
 -gofmt=true
   Automatically run gofmt on the modified files.
 -interface=
   Comma-separated list of interface packages (required).

Logcop remove

Remove log statements. Note that remove modifies <packages> in-place.  It is a
good idea to commit changes to version control before running this tool so you
can see the diff or revert the changes.

Usage:
   logcop remove [flags] <packages>

<packages> is the list of packages to remove log statements from.

The logcop remove flags are:
 -diff-only=false
   Show changes that would be made without actually making them.
 -gofmt=true
   Automatically run gofmt on the modified files.

Logcop version

Print version of the logcop tool.

Usage:
   logcop version

Logcop help

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

Output is formatted to a target width in runes, determined by checking the
CMDLINE_WIDTH environment variable, falling back on the terminal width, falling
back on 80 chars.  By setting CMDLINE_WIDTH=x, if x > 0 the width is x, if x < 0
the width is unlimited, and if x == 0 or is unset one of the fallbacks is used.

Usage:
   logcop help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The logcop help flags are:
 -style=compact
   The formatting style for help output:
      compact - Good for compact cmdline output.
      full    - Good for cmdline output, shows all global flags.
      godoc   - Good for godoc processing.
   Override the default by setting the CMDLINE_STYLE environment variable.
*/
package main
