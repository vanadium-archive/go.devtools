// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Command vcloud is a wrapper over the Google Compute Engine gcloud tool.  It
simplifies common usage scenarios and provides some Vanadium-specific support.

Usage:
   vcloud <command>

The vcloud commands are:
   list        List GCE node information
   cp          Copy files to/from GCE node(s)
   node        Manage GCE nodes
   run         Copy file(s) to GCE node(s) and run
   sh          Start a shell or run a command on GCE node(s)
   help        Display help for commands or topics

The global flags are:
 -color=false
   Format output in color.
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.
 -n=false
   Show what commands will run, but do not execute them.
 -project=vanadium-internal
   Specify the gcloud project.
 -user=veyron
   Run operations as the given user on each node.
 -v=false
   Print verbose output.

Vcloud list - List GCE node information

List GCE node information.  Runs 'gcloud compute instances list'.

Usage:
   vcloud list [flags] [nodes]

[nodes] is a comma-separated list of node name(s).  Each node name is a regular
expression, with matches performed on the full node name.  We select nodes that
match any of the regexps.  The comma-separated list allows you to easily specify
a list of specific node names, without using regexp alternation.  We assume node
names do not have embedded commas.

If [nodes] is not provided, lists information for all nodes.

The vcloud list flags are:
 -fields=
   Only display these fields, specified as comma-separated column header names.
 -noheader=false
   Don't print list table header.

Vcloud cp

Copy files to GCE node(s).  Runs 'gcloud compute copy-files'.  The default is to
copy to/from all nodes in parallel.

Usage:
   vcloud cp [flags] <nodes> <src...> <dst>

<nodes> is a comma-separated list of node name(s).  Each node name is a regular
expression, with matches performed on the full node name.  We select nodes that
match any of the regexps.  The comma-separated list allows you to easily specify
a list of specific node names, without using regexp alternation.  We assume node
names do not have embedded commas.

<src...> are the source file argument(s) to 'gcloud compute copy-files', and
<dst> is the destination.  The syntax for each file is:
  [:]file

Files with the ':' prefix are remote; files without any such prefix are local.

As with 'gcloud compute copy-files', if <dst> is local, all <src...> must be
remote.  If <dst> is remote, all <src...> must be local.

Each matching node in <nodes> is applied to the remote side of the copy
operation, either src or dst.  If <dst> is local and there is more than one
matching node, sub directories will be automatically created under <dst>.

E.g. if <nodes> matches A, B and C:
  // Copies local src{1,2,3} to {A,B,C}:dst
  vcloud cp src1 src2 src3 :dst
  // Copies remote {A,B,C}:src{1,2,3} to dst/{A,B,C} respectively.
  vcloud cp :src1 :src2 :src3 dst

The vcloud cp flags are:
 -failfast=false
   Skip unstarted nodes after the first failing node.
 -p=-1
   Copy to/from this many nodes in parallel.
     <0   means all nodes in parallel
      0,1 means sequentially
      2+  means at most this many nodes in parallel

Vcloud node - Manage GCE nodes

Manage GCE nodes.

Usage:
   vcloud node <command>

The vcloud node commands are:
   authorize   Authorize a user to login to a GCE node
   deauthorize Deauthorize a user to login to a GCE node
   create      Create GCE nodes
   delete      Delete GCE nodes

Vcloud node authorize - Authorize a user to login to a GCE node

Authorizes a user to login to a GCE node (possibly as other user). For instance,
this mechanism is used to give Jenkins slave nodes access to the GCE mirror of
Vanadium repositories.

Usage:
   vcloud node authorize <userA>@<hostA> [<userB>@]<hostB>

<userA>@<hostA> [<userB>@]<hostB> authorizes userA to log into GCE node hostB
from GCE node hostA as user userB. The default value for userB is userA.

Vcloud node deauthorize - Deauthorize a user to login to a GCE node

Deuthorizes a user to login to a GCE node (possibly as other user). For
instance, this mechanism is used to revoke access of give Jenkins slave nodes to
the GCE mirror of Vanadium repositories.

Usage:
   vcloud node deauthorize <userA>@<hostA> [<userB>@]<hostB>

<userA>@<hostA> [<userB>@]<hostB> deauthorizes userA to log into GCE node hostB
from GCE node hostA as user userB. The default value for userB is userA.

Vcloud node create - Create GCE nodes

Create GCE nodes. Runs 'gcloud compute instances create'.

Usage:
   vcloud node create [flags] <names>

<names> is a list of names identifying nodes to be created.

The vcloud node create flags are:
 -boot-disk-size=500GB
   Size of the machine boot disk.
 -image=ubuntu-14-04
   Image to create the machine from.
 -machine-type=n1-standard-8
   Machine type to create.
 -setup-script=
   Script to set up the machine.
 -zone=us-central1-f
   Zone to create the machine in.

Vcloud node delete - Delete GCE nodes

Delete GCE nodes. Runs 'gcloud compute instances delete'.

Usage:
   vcloud node delete [flags] <names>

<names> is a list of names identifying nodes to be deleted.

The vcloud node delete flags are:
 -zone=us-central1-f
   Zone to delete the machine in.

Vcloud run

Copy file(s) to GCE node(s) and run.  Uses the logic of both cp and sh.

Usage:
   vcloud run [flags] <nodes> <files...> [++ [command...]]

<nodes> is a comma-separated list of node name(s).  Each node name is a regular
expression, with matches performed on the full node name.  We select nodes that
match any of the regexps.  The comma-separated list allows you to easily specify
a list of specific node names, without using regexp alternation.  We assume node
names do not have embedded commas.

<files...> are the local source file argument(s) to copy to each matching node.

[command...] is the shell command line to run on each node.  Specify the entire
command line without extra quoting, just like 'vcloud sh'.  If a command is
specified, it must be preceeded by a single ++ argument, to distinguish it from
the files.  If no command is given, runs the first file from <files...>.

We run the following logic on each matching node, in parallel by default:
  1) Create a temporary directory TMPDIR based on a random number.
  2) Copy run files to TMPDIR.
  3) Change current directory to TMPDIR.
  4) Runs the [command...], or if no command is given, runs the first run file.
  5) If -outdir is specified, remove run files from TMPDIR, and copy TMPDIR from
     the node to the local -outdir.
  6) Delete TMPDIR.

The vcloud run flags are:
 -failfast=false
   Skip unstarted nodes after the first failing node.
 -outdir=
   Output directory to store results from each node.
 -p=-1
   Copy/run on this many nodes in parallel.
     <0   means all nodes in parallel
      0,1 means sequentially
      2+  means at most this many nodes in parallel

Vcloud sh

Start a shell or run a command on GCE node(s).  Runs 'gcloud compute ssh'.

Usage:
   vcloud sh [flags] <nodes> [command...]

<nodes> is a comma-separated list of node name(s).  Each node name is a regular
expression, with matches performed on the full node name.  We select nodes that
match any of the regexps.  The comma-separated list allows you to easily specify
a list of specific node names, without using regexp alternation.  We assume node
names do not have embedded commas.

[command...] is the shell command line to run on each node.  Specify the entire
command line without extra quoting, e.g. like this:
  vcloud sh jenkins-node uname -a
But NOT like this:
  vcloud sh jenkins-node 'uname -a'
If quoting and escaping becomes too complicated, use 'vcloud run' instead.

If <nodes> matches exactly one node and no [command] is given, sh starts a shell
on the specified node.

Otherwise [command...] is required; sh runs the command on all matching nodes.
The default is to run on all nodes in parallel.

The vcloud sh flags are:
 -failfast=false
   Skip unstarted nodes after the first failing node.
 -p=-1
   Run command on this many nodes in parallel.
     <0   means all nodes in parallel
      0,1 means sequentially
      2+  means at most this many nodes in parallel

Vcloud help - Display help for commands or topics

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

Usage:
   vcloud help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The vcloud help flags are:
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
