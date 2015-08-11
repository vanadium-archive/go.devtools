// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Command gologcop checks for and injects logging statements into Go source code.

When checking, it ensures that all implementations in <packages> of all exported
interfaces declared in packages passed to the -interface flag have an
appropriate logging construct.

When injecting or removing, it modifies the source code to inject or remove such
logging constructs.

LIMITATIONS:

Removal will not automatically remove the package import for the call to be
removed.

Usage:
   gologcop [flags] <command>

The gologcop commands are:
   check       Check for log statements in public API implementations
   inject      Inject log statements in public API implementations
   remove      Remove log statements
   version     Print version
   help        Display help for commands or topics

The gologcop flags are:
 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -progress=false
   Print verbose progress information.
 -use-v23-context=true
   Pass a context.T argument (which must be of type v.io/v23/context.T), if
   available, to the injected call as its first parameter.
 -v=false
   Print verbose output.

The global flags are:
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.

Gologcop check - Check for log statements in public API implementations

Check for log statements in public API implementations.

Usage:
   gologcop check [flags] <packages>

<packages> is the list of packages to be checked.

The gologcop check flags are:
 -call=LogCall
   The function call to be checked for as defer <pkg>.<call>()() and defer
   <pkg>.<call>f(...)(...). The value of <pkg> is determined from --import.
 -import=v.io/x/ref/lib/apilog
   Import path for the injected call.
 -interface=
   Comma-separated list of interface packages (required).

Gologcop inject - Inject log statements in public API implementations

Inject log statements in public API implementations. Note that inject modifies
<packages> in-place.  It is a good idea to commit changes to version control
before running this tool so you can see the diff or revert the changes.

Usage:
   gologcop inject [flags] <packages>

<packages> is the list of packages to inject log statements in.

The gologcop inject flags are:
 -call=LogCall
   The function call to be injected as defer <pkg>.<call>()() and defer
   <pkg>.<call>f(...)(...). The value of <pkg> is determined from --import.
 -diff-only=false
   Show changes that would be made without actually making them.
 -gofmt=true
   Automatically run gofmt on the modified files.
 -import=v.io/x/ref/lib/apilog
   Import path for the injected call.
 -interface=
   Comma-separated list of interface packages (required).

Gologcop remove - Remove log statements

Remove log statements. Note that remove modifies <packages> in-place.  It is a
good idea to commit changes to version control before running this tool so you
can see the diff or revert the changes.

Usage:
   gologcop remove [flags] <packages>

<packages> is the list of packages to remove log statements from.

The gologcop remove flags are:
 -call=apilog.LogCall
   The function call to be removed. Note, that the package selector must be
   included. No attempt is made to remove the import declaration if the package
   is no longer used as a result of the removal.
 -diff-only=false
   Show changes that would be made without actually making them.
 -gofmt=true
   Automatically run gofmt on the modified files.

Gologcop version - Print version

Print version of the gologcop tool.

Usage:
   gologcop version

Gologcop help - Display help for commands or topics

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

Usage:
   gologcop help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The gologcop help flags are:
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
