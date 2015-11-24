// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Command vkiosk runs Chrome in a virtual X11 environtment for a given url, takes
its screenshots periodically, exports them to Google Storage, and serves them in
a http server.

This tool is only tested in Debian/Ubuntu.

Usage:
   vkiosk [flags] <command>

The vkiosk commands are:
   collect     Takes screenshots of a given URL in Chrome and stores them in the
               given export dir
   serve       Serve screenshots from local file system or Google Storage
   help        Display help for commands or topics

The vkiosk flags are:
 -color=true
   Use color to format output.
 -export-dir=gs://vanadium-kiosk
   Directory for storing/retrieving screenshots. Dirs that start with 'gs://'
   point to Google Storage buckets.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

The global flags are:
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.
 -time=false
   Dump timing information to stderr before exiting the program.

Vkiosk collect - Takes screenshots of a given URL in Chrome and stores them in the given export dir

The collect commands takes screenshots of a given URL in Chrome and stores them
in the given export dir.

To use this command, the following programs need to be installed: Google
Chrome,Xvfb, and Fluxbox.

Usage:
   vkiosk collect [flags]

The vkiosk collect flags are:
 -display=:1365
   The value of DISPLAY environment variable for Xvfb.
 -interval=5s
   The interval between screenshots.
 -name=
   The name of the screenshot file.
 -resolution=1920x1080x24
   The resolution string for Xvfb.
 -url=
   The url to take screenshots for.

 -color=true
   Use color to format output.
 -export-dir=gs://vanadium-kiosk
   Directory for storing/retrieving screenshots. Dirs that start with 'gs://'
   point to Google Storage buckets.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Vkiosk serve - Serve screenshots from local file system or Google Storage

Serve screenshots from local file system or Google Storage.

Usage:
   vkiosk serve [flags]

The vkiosk serve flags are:
 -port=8000
   Port for the server.

 -color=true
   Use color to format output.
 -export-dir=gs://vanadium-kiosk
   Directory for storing/retrieving screenshots. Dirs that start with 'gs://'
   point to Google Storage buckets.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Vkiosk help - Display help for commands or topics

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

Usage:
   vkiosk help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The vkiosk help flags are:
 -style=compact
   The formatting style for help output:
      compact   - Good for compact cmdline output.
      full      - Good for cmdline output, shows all global flags.
      godoc     - Good for godoc processing.
      shortonly - Only output short description.
   Override the default by setting the CMDLINE_STYLE environment variable.
 -width=<terminal width>
   Format output to this target width in runes, or unlimited if width < 0.
   Defaults to the terminal width if available.  Override the default by setting
   the CMDLINE_WIDTH environment variable.
*/
package main
