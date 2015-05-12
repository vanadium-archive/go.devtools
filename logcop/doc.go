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

When injecting or removing, it modifies the source code to inject or remove such
logging constructs.

LIMITATIONS:

Removal will not automatically remove the package import for the call to be
removed.

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
 -use-v23-context=false
   Pass a context.T argument (which must be of type v.io/v23/context.T), if
   available, to the injected call as its first parameter.
 -v=false
   Print verbose output.

The global flags are:
 -v23.metadata=<just specify -v23.metadata to activate>
   Displays metadata for the program and exits.

Logcop check

Check for log statements in public API implementations.

Usage:
   logcop check [flags] <packages>

<packages> is the list of packages to be checked.

The logcop check flags are:
 -call=LogCall
   The function call to be checked for as defer <pkg>.<call>()() and defer
   <pkg>.<call>f(...)(...). The value of <pkg> is determined from --import.
 -import=v.io/x/lib/vlog
   Import path for the injected call.
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
 -call=LogCall
   The function call to be injected as defer <pkg>.<call>()() and defer
   <pkg>.<call>f(...)(...). The value of <pkg> is determined from --import.
 -diff-only=false
   Show changes that would be made without actually making them.
 -gofmt=true
   Automatically run gofmt on the modified files.
 -import=v.io/x/lib/vlog
   Import path for the injected call.
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
 -call=vlog.LogCall
   The function call to be removed. Note, that the package selector must be
   included. No attempt is made to remove the import declaration if the package
   is no longer used as a result of the removal.
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
 -width=<terminal width>
   Format output to this target width in runes, or unlimited if width < 0.
   Defaults to the terminal width if available.  Override the default by setting
   the CMDLINE_WIDTH environment variable.
*/
package main
