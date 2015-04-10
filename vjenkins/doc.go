// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Command vjenkins implements Vanadium-specific utilities for interacting with
Jenkins.

Usage:
   vjenkins [flags] <command>

The vjenkins commands are:
   node        Manage Jenkins slave nodes
   help        Display help for commands or topics
Run "vjenkins help [command]" for command usage.

The vjenkins flags are:
 -jenkins=http://localhost:8080/jenkins
   The host of the Jenkins master.

The global flags are:
 -color=false
   Format output in color.
 -n=false
   Show what commands will run, but do not execute them.
 -v=false
   Print verbose output.

Vjenkins Node

Manage Jenkins slave nodes.

Usage:
   vjenkins node <command>

The vjenkins node commands are:
   create      Create Jenkins slave nodes
   delete      Delete Jenkins slave nodes

Vjenkins Node Create

Create Jenkins nodes. Uses the Jenkins REST API to create new slave nodes.

Usage:
   vjenkins node create [flags] <names>

<names> is a list of names identifying nodes to be created.

The vjenkins node create flags are:
 -credentials-id=73f76f53-8332-4259-bc08-d6f0b8521a5b
   The credentials ID used to connect the master to the node.
 -description=
   Node description.
 -project=vanadium-internal
   GCE project of the machine.
 -zone=us-central1-f
   GCE zone of the machine.

Vjenkins Node Delete

Delete Jenkins nodes. Uses the Jenkins REST API to delete existing slave nodes.

Usage:
   vjenkins node delete <names>

<names> is a list of names identifying nodes to be deleted.

Vjenkins Help

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

The output is formatted to a target width in runes.  The target width is
determined by checking the environment variable CMDLINE_WIDTH, falling back on
the terminal width from the OS, falling back on 80 chars.  By setting
CMDLINE_WIDTH=x, if x > 0 the width is x, if x < 0 the width is unlimited, and
if x == 0 or is unset one of the fallbacks is used.

Usage:
   vjenkins help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The vjenkins help flags are:
 -style=default
   The formatting style for help output, either "default" or "godoc".
*/
package main
