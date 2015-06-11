// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Command vbinary retrieves daily builds of Vanadium binaries stored in a Google
Storage bucket.

Usage:
   vbinary [flags] <command>

The vbinary commands are:
   list        List existing daily builds of Vanadium binaries
   download    Download an existing daily build of Vanadium binaries
   help        Display help for commands or topics

The vbinary flags are:
 -arch=<runtime.GOARCH>
   Target architecture.  The default is the value of runtime.GOARCH.
 -color=true
   Use color to format output.
 -date-prefix=
   Date prefix to match daily build timestamps. Must be a prefix of YYYY-MM-DD.
 -key-file=
   Google Developers service account JSON key file.
 -n=false
   Show what commands will run but do not execute them.
 -os=<runtime.GOOS>
   Target operating system.  The default is the value of runtime.GOOS.
 -release=false
   Operate on vanadium-release bucket instead of vanadium-binaries.
 -v=false
   Print verbose output.

The global flags are:
 -v23.metadata=<just specify -v23.metadata to activate>
   Displays metadata for the program and exits.

Vbinary list - List existing daily builds of Vanadium binaries

List existing daily builds of Vanadium binaries. The displayed dates can be
limited with the --date-prefix flag.

Usage:
   vbinary list

Vbinary download - Download an existing daily build of Vanadium binaries

Download an existing daily build of Vanadium binaries. The latest snapshot
within the --date-prefix range will be downloaded. If no --date-prefix flag is
provided, the overall latest snapshot will be downloaded.

Usage:
   vbinary download [flags]

The vbinary download flags are:
 -output-dir=
   Directory for storing downloaded binaries.

Vbinary help - Display help for commands or topics

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

Usage:
   vbinary help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The vbinary help flags are:
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
