// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
bendroid executes Go tests and benchmarks on an android device.

It requires that 'adb' (the Android Debug Bridge:
http://developer.android.com/tools/help/adb.html) be available in PATH.

Sample usage:
  GOARCH=arm GOOS=android go test -exec bendroid crypto/sha256
Or, alternatively:
  GOARCH=arm GOOS=android go test -c cryto/sha256
  bendroid ./sha256.test

Additionally, bendroid outputs a preamble of the form:
  BENDROID_<variable_name>=<value>
that describe the characteristics of the connected android device.

WARNING: As of March 2016, bendroid is unable to ensure synchronization between
what the executed program prints on standard output and standard error. This
should hopefully be resolved with the next release of adb.

Usage:
   bendroid [flags] <filename to execute> [<flags provided to 'go test'>]

See 'go help run' for details

The bendroid flags are:
 -bendroid.v=false
   verbose output: print adb commands executed by bendroid
 -bendroid.work=false
   print the name of the directory on the device where all data is copied and do
   not erase it

The global flags are:
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.
 -time=false
   Dump timing information to stderr before exiting the program.
*/
package main
