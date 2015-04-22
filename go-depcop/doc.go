// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Command go-depcop checks Go package dependencies against constraints described
in GO.PACKAGE files.  Both incoming and outgoing dependencies may be configured,
and Go "internal" package rules are enforced.

GO.PACKAGE files are traversed hierarchically, from the deepmost package to
GOROOT, until a matching rule is found.  If no matching rule is found, the
default behavior is to allow the dependency, to stay compatible with existing
packages that do not include dependency rules.

GO.PACKAGE is a JSON file that looks like this:
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

The go-depcop flags are:
 -include-tests=false
   Include tests in computing dependencies.
 -v=false
   Print verbose output.

Go-depcop check

Check package dependency constraints.

Usage:
   go-depcop check [flags] <packages>

<packages> is a list of packages

The go-depcop check flags are:
 -r=false
   Check dependencies recursively.

Go-depcop list

List outgoing package dependencies.

Usage:
   go-depcop list [flags] <packages>

<packages> is a list of packages

The go-depcop list flags are:
 -pretty-print=false
   Make output easy to read, indenting nested dependencies.
 -show-goroot=false
   Show packages in goroot.
 -transitive=false
   List transitive dependencies.

Go-depcop rlist

List incoming package dependencies.

Usage:
   go-depcop rlist <packages>

<packages> is a list of packages

Go-depcop version

Print version of the go-depcop tool.

Usage:
   go-depcop version

Go-depcop help

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

Output is formatted to a target width in runes, determined by checking the
CMDLINE_WIDTH environment variable, falling back on the terminal width, falling
back on 80 chars.  By setting CMDLINE_WIDTH=x, if x > 0 the width is x, if x < 0
the width is unlimited, and if x == 0 or is unset one of the fallbacks is used.

Usage:
   go-depcop help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The go-depcop help flags are:
 -style=compact
   The formatting style for help output:
      compact - Good for compact cmdline output.
      full    - Good for cmdline output, shows all global flags.
      godoc   - Good for godoc processing.
   Override the default by setting the CMDLINE_STYLE environment variable.
*/
package main
