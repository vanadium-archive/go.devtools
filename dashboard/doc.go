// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Command dashboard runs the Vanadium dashboard web server.

Usage:
   dashboard [flags]

The dashboard flags are:
 -cache=
   Directory to use for caching files.
 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -port=8000
   Port for the server.
 -results-bucket=gs://vanadium-test-results
   Google Storage bucket to use for fetching test results.
 -static=
   Directory to use for serving static files.
 -status-bucket=gs://vanadium-oncall/data
   Google Storage bucket to use for fetching service status data.
 -v=false
   Print verbose output.

The global flags are:
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.
*/
package main
