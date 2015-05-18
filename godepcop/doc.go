// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Command godepcop checks Go package dependencies against constraints described in
GO.PACKAGE files.  In addition to user-defined constraints, the Go 1.5 internal
package rules are also enforced.

Usage:
   godepcop <command>

The godepcop commands are:
   check          Check package dependency constraints
   list           List packages imported by the given packages
   list-importers List packages that import the given packages
   version        Print version
   help           Display help for commands or topics

The global flags are:
 -v23.metadata=<just specify -v23.metadata to activate>
   Displays metadata for the program and exits.

Godepcop check

Check package dependency constraints.

Every Go package directory may contain an optional GO.PACKAGE file.  Each file
specifies dependency rules, which either allow or deny imports by that package.
The files are traversed hierarchically, from the deepmost package to the root of
the source tree, until a matching rule is found.  If no matching rule is found,
the default behavior is to allow the dependency, to stay compatible with
existing packages that do not include dependency rules.

GO.PACKAGE is a JSON file that looks like this:
   {
     "imports": [
       {"allow": "pattern1/..."},
       {"allow": "pattern2"},
       {"deny":  "..."}
     ]
   }

Each item in "imports" is a rule, which either allows or denies imports based on
the given pattern.  Patterns that end with "/..." are special: "foo/..." means
that foo and all its subpackages match the rule.  The special-case pattern "..."
means that all packages in GOPATH, but not GOROOT, match the rule.

Usage:
   godepcop check <packages>

<packages> is a list of packages to check

Godepcop list

List packages imported by the given <packages>.

Lists all transitive imports by default; set the -direct flag to limit the
listing to direct imports by the given <packages>.

Elides $GOROOT packages by default; set the -show-goroot flag to show packages
in $GOROOT.  If any of the given <packages> are $GOROOT packages, list behaves
as if -show-goroot were set to true.

Lists each imported package exactly once.  Set the -indent flag for pretty
indentation to help visualize the dependency hierarchy.  Setting -indent may
cause the same package to be listed multiple times.

Usage:
   godepcop list [flags] <packages>

<packages> is a list of packages

The godepcop list flags are:
 -direct=false
   Only consider direct dependencies, rather than transitive dependencies.
 -indent=false
   List dependencies with pretty indentation.
 -show-goroot=false
   Show packages in goroot.

Godepcop list-importers

List packages that import the given <packages>; the reverse of "list".  The
listed packages are called "importers".

Lists all transitive importers by default; set the -direct flag to limit the
listing to importers that directly import the given <packages>.

Elides $GOROOT packages by default; set the -show-goroot flag to show importers
in $GOROOT.  If any of the given <packages> are $GOROOT packages, list-reverse
behaves as if -show-goroot were set to true.

Lists each importer package exactly once.

Usage:
   godepcop list-importers [flags] <packages>

<packages> is a list of packages

The godepcop list-importers flags are:
 -direct=false
   Only consider direct dependencies, rather than transitive dependencies.
 -show-goroot=false
   Show packages in goroot.

Godepcop version

Print version of the godepcop tool.

Usage:
   godepcop version

Godepcop help

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

Usage:
   godepcop help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The godepcop help flags are:
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
