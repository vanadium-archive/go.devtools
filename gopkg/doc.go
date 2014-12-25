// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Print information about go package(s).

Example of printing all top-level information about the vdl package:
  veyron run gopkg v.io/veyron/veyron2/vdl

Example of printing the names of all Test* funcs from the vdl package:
  veyron run gopkg -test -kind=func -name_re 'Test.*' -type_re 'func\(.*testing\.T\)' -noheader -notype v.io/veyron/veyron2/vdl

Usage:
   gopkg [flags] <args>

<args> is a list of arguments denoting a set of initial packages. It may take
one of two forms:

1. A list of *.go source files.

   All of the specified files are loaded, parsed and type-checked
   as a single package.  All the files must belong to the same directory.

2. A list of import paths, each denoting a package.

   The package's directory is found relative to the $GOROOT and
   $GOPATH using similar logic to 'go build', and the *.go files in
   that directory are loaded, parsed and type-checked as a single
   package.

   In addition, all *_test.go files in the directory are then loaded
   and parsed.  Those files whose package declaration equals that of
   the non-*_test.go files are included in the primary package.  Test
   files whose package declaration ends with "_test" are type-checked
   as another package, the 'external' test package, so that a single
   import path may denote two packages.  (Whether this behaviour is
   enabled is tool-specific, and may depend on additional flags.)

   Due to current limitations in the type-checker, only the first
   import path of the command line will contribute any tests.

A '--' argument terminates the list of packages.

The gopkg flags are:
 -kind=const,var,func,type
   Print information for the specified kinds, in the order listed.
 -name_re=.*
   Filter out identifier names that don't match this regexp.
 -noheader=false
   Don't print headers.
 -noname=false
   Don't print identifier names.
 -notype=false
   Don't print type descriptions.
 -test=false
   Load test code (*_test.go) for packages.
 -type_re=.*
   Filter out type descriptions that don't match this regexp.
*/
package main
