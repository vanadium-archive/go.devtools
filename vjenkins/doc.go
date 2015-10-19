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

The vjenkins flags are:
 -color=true
   Use color to format output.
 -jenkins=http://localhost:8080/jenkins
   The host of the Jenkins master.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

The global flags are:
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.
 -time=false
   Dump timing information to stderr before exiting the program.

Vjenkins node - Manage Jenkins slave nodes

Manage Jenkins slave nodes.

Usage:
   vjenkins node <command>

The vjenkins node commands are:
   create      Create Jenkins slave nodes
   delete      Delete Jenkins slave nodes

Vjenkins node create - Create Jenkins slave nodes

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

Vjenkins node delete - Delete Jenkins slave nodes

Delete Jenkins nodes. Uses the Jenkins REST API to delete existing slave nodes.

Usage:
   vjenkins node delete <names>

<names> is a list of names identifying nodes to be deleted.

Vjenkins help - Display help for commands or topics

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

Usage:
   vjenkins help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The vjenkins help flags are:
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
