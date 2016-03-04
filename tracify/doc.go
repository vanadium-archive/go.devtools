// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
tracify adds vtrace annotations to all functions in the given packages that have
a context as the first argument.

TODO(mattr): We will eventually support various options like excluding certain
functions or including specific information in the span name.

Usage:
   tracify [flags] [-t] [packages]

The global flags are:
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.
 -t=false
   include transitive dependencies of named packages.
 -time=false
   Dump timing information to stderr before exiting the program.
*/
package main
