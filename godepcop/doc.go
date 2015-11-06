// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Command godepcop checks Go package dependencies against constraints described in
.godepcop files.  In addition to user-defined constraints, the Go 1.5 internal
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
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.
 -time=false
   Dump timing information to stderr before exiting the program.

Godepcop check - Check package dependency constraints

Check package dependency constraints.

Every Go package directory may contain an optional .godepcop file.  Each file
specifies dependency rules, which either allow or deny imports by that package.
The files are traversed hierarchically, from the deepmost package to the root of
the source tree, until a matching rule is found.  If no matching rule is found,
the default behavior is to allow the dependency, to support packages that do not
have any dependency rules.

The .godepcop file is encoded in XML:

  <godepcop>
    <pkg allow="pattern1/..."/>
    <pkg allow="pattern2"/>
    <pkg deny="..."/>
    <test allow="pattern3"/>
    <xtest allow="..."/>
  </godepcop>

Each element in godepcop is a rule, which either allows or denies imports based
on the given pattern.  Patterns that end with "/..." are special: "foo/..."
means that foo and all its subpackages match the rule.  The special-case pattern
"..."  means that all packages in GOPATH, but not GOROOT, match the rule.

There are three groups of rules:
  pkg   - Rules applied to all imports from the package.
  test  - Extra rules for imports from all test files.
  xtest - Extra rules for imports from test files in the *_test package.

Rules in each group are processed in the order they appear in the .godepcop
file.  The transitive closure of the following imports are checked for each
package P:
  P.Imports                              - check pkg rules
  P.Imports+P.TestImports                - check test and pkg rules
  P.Imports+P.TestImports+P.XTestImports - check xtest, test and pkg rules

Usage:
   godepcop check <packages>

<packages> is a list of packages to check

Godepcop list - List packages imported by the given packages

List packages imported by the given <packages>.

Lists all transitive imports by default; set the -direct flag to limit the
listing to direct imports by the given <packages>.

Elides $GOROOT packages by default; set the -goroot flag to include packages in
$GOROOT.  If any of the given <packages> are $GOROOT packages, list behaves as
if -goroot were set to true.

Lists each imported package exactly once when using the default -style=set.  See
the -style flag for alternate output styles.

Usage:
   godepcop list [flags] <packages>

<packages> is a list of packages

The godepcop list flags are:
 -direct=false
   Only show direct dependencies, rather than showing transitive dependencies.
 -goroot=false
   Show $GOROOT packages.
 -merge-policies=+CCFLAGS,+CGO_CFLAGS,+CGO_CXXFLAGS,+CGO_LDFLAGS,+CXXFLAGS,GOARCH,GOOS,GOPATH:,^GOROOT*,+LDFLAGS,:PATH,VDLPATH:
   specify policies for merging environment variables
 -style=set
   List dependencies with the given style:
      set    - As a sorted set of unique packages.
      indent - As a hierarchical list with pretty indentation.
      dot    - As a DOT graph (http://www.graphviz.org)
 -test=false
   Show imports from test files in the same package.
 -xtest=false
   Show imports from test files in the same package or in the *_test package.

Godepcop list-importers - List packages that import the given packages

List packages that import the given <packages>; the reverse of "list".  The
listed packages are called "importers".

Lists all transitive importers by default; set the -direct flag to limit the
listing to importers that directly import the given <packages>.

Elides $GOROOT packages by default; set the -goroot flag to include importers in
$GOROOT.  If any of the given <packages> are $GOROOT packages, list-importers
behaves as if -goroot were set to true.

Lists each importer package exactly once.

Usage:
   godepcop list-importers [flags] <packages>

<packages> is a list of packages

The godepcop list-importers flags are:
 -direct=false
   Only show direct dependencies, rather than showing transitive dependencies.
 -goroot=false
   Show $GOROOT packages.
 -merge-policies=+CCFLAGS,+CGO_CFLAGS,+CGO_CXXFLAGS,+CGO_LDFLAGS,+CXXFLAGS,GOARCH,GOOS,GOPATH:,^GOROOT*,+LDFLAGS,:PATH,VDLPATH:
   specify policies for merging environment variables
 -test=false
   Show imports from test files in the same package.
 -xtest=false
   Show imports from test files in the same package or in the *_test package.

Godepcop version - Print version

Print version of the godepcop tool.

Usage:
   godepcop version

Godepcop help - Display help for commands or topics

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
