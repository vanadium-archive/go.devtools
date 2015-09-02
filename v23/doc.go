// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Command v23 is a multi-purpose tool for Vanadium development.

Usage:
   v23 [flags] <command>

The v23 commands are:
   cl           Manage project changelists
   contributors List project contributors
   project      Manage the vanadium projects
   snapshot     Manage project snapshots
   update       Update all vanadium tools and projects
   version      Print version
   help         Display help for commands or topics

The v23 flags are:
 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

The global flags are:
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.

V23 cl - Manage project changelists

Manage project changelists.

Usage:
   v23 cl <command>

The v23 cl commands are:
   cleanup     Clean up changelists that have been merged
   mail        Mail a changelist for review
   new         Create a new local branch for a changelist
   sync        Bring a changelist up to date

V23 cl cleanup - Clean up changelists that have been merged

Command "cleanup" checks that the given branches have been merged into the
corresponding remote branch. If a branch differs from the corresponding remote
branch, the command reports the difference and stops. Otherwise, it deletes the
given branches.

Usage:
   v23 cl cleanup [flags] <branches>

<branches> is a list of branches to cleanup.

The v23 cl cleanup flags are:
 -f=false
   Ignore unmerged changes.
 -remote-branch=master
   Name of the remote branch the CL pertains to.

V23 cl mail - Mail a changelist for review

Command "mail" squashes all commits of a local branch into a single "changelist"
and mails this changelist to Gerrit as a single commit. First time the command
is invoked, it generates a Change-Id for the changelist, which is appended to
the commit message. Consecutive invocations of the command use the same
Change-Id by default, informing Gerrit that the incomming commit is an update of
an existing changelist.

Usage:
   v23 cl mail [flags]

The v23 cl mail flags are:
 -autosubmit=false
   Automatically submit the changelist when feasiable.
 -cc=
   Comma-seperated list of emails or LDAPs to cc.
 -check-uncommitted=true
   Check that no uncommitted changes exist.
 -d=false
   Send a draft changelist.
 -edit=true
   Open an editor to edit the commit message.
 -host=https://vanadium-review.googlesource.com/
   Gerrit host to use.
 -presubmit=all
   The type of presubmit tests to run. Valid values: none,all.
 -r=
   Comma-seperated list of emails or LDAPs to request review.
 -remote-branch=master
   Name of the remote branch the CL pertains to.
 -set-topic=true
   Set Gerrit CL topic.
 -topic=
   CL topic, defaults to <username>-<branchname>.

V23 cl new - Create a new local branch for a changelist

Command "new" creates a new local branch for a changelist. In particular, it
forks a new branch with the given name from the current branch and records the
relationship between the current branch and the new branch in the .v23 metadata
directory. The information recorded in the .v23 metadata directory tracks
dependencies between CLs and is used by the "v23 cl sync" and "v23 cl mail"
commands.

Usage:
   v23 cl new <name>

<name> is the changelist name.

V23 cl sync - Bring a changelist up to date

Command "sync" brings the CL identified by the current branch up to date with
the branch tracking the remote branch this CL pertains to. To do that, the
command uses the information recorded in the .v23 metadata directory to identify
the sequence of dependent CLs leading to the current branch. The command then
iterates over this sequence bringing each of the CLs up to date with its
ancestor. The end result of this process is that all CLs in the sequence are up
to date with the branch that tracks the remote branch this CL pertains to.

NOTE: It is possible that the command cannot automatically merge changes in an
ancestor into its dependent. When that occurs, the command is aborted and prints
instructions that need to be followed before the command can be retried.

Usage:
   v23 cl sync [flags]

The v23 cl sync flags are:
 -remote-branch=master
   Name of the remote branch the CL pertains to.

V23 contributors - List project contributors

Lists project contributors. Projects to consider can be specified as an
argument. If no projects are specified, all projects in the current manifest are
considered by default.

Usage:
   v23 contributors [flags] <projects>

<projects> is a list of projects to consider.

The v23 contributors flags are:
 -aliases=
   Path to the aliases file.
 -n=false
   Show number of contributions.

V23 project - Manage the vanadium projects

Manage the vanadium projects.

Usage:
   v23 project <command>

The v23 project commands are:
   clean        Restore vanadium projects to their pristine state
   list         List existing vanadium projects and branches
   shell-prompt Print a succinct status of projects, suitable for shell prompts
   poll         Poll existing vanadium projects

V23 project clean - Restore vanadium projects to their pristine state

Restore vanadium projects back to their master branches and get rid of all the
local branches and changes.

Usage:
   v23 project clean [flags] <project ...>

<project ...> is a list of projects to clean up.

The v23 project clean flags are:
 -branches=false
   Delete all non-master branches.

V23 project list - List existing vanadium projects and branches

Inspect the local filesystem and list the existing projects and branches.

Usage:
   v23 project list [flags]

The v23 project list flags are:
 -branches=false
   Show project branches.
 -nopristine=false
   If true, omit pristine projects, i.e. projects with a clean master branch and
   no other branches.

V23 project shell-prompt

Reports current branches of vanadium projects (repositories) as well as an
indication of each project's status:
  *  indicates that a repository contains uncommitted changes
  %  indicates that a repository contains untracked files

Usage:
   v23 project shell-prompt [flags]

The v23 project shell-prompt flags are:
 -check-dirty=true
   If false, don't check for uncommitted changes or untracked files. Setting
   this option to false is dangerous: dirty master branches will not appear in
   the output.
 -show-name=false
   Show the name of the current repo.

V23 project poll - Poll existing vanadium projects

Poll vanadium projects that can affect the outcome of the given tests and report
whether any new changes in these projects exist. If no tests are specified, all
projects are polled by default.

Usage:
   v23 project poll [flags] <test ...>

<test ...> is a list of tests that determine what projects to poll.

The v23 project poll flags are:
 -manifest=
   Name of the project manifest.

V23 snapshot - Manage project snapshots

The "v23 snapshot" command can be used to manage project snapshots. In
particular, it can be used to create new snapshots and to list existing
snapshots.

The command-line flag "-remote" determines whether the command pertains to
"local" snapshots that are only stored locally or "remote" snapshots the are
revisioned in the manifest repository.

Usage:
   v23 snapshot [flags] <command>

The v23 snapshot commands are:
   create      Create a new project snapshot
   list        List existing project snapshots

The v23 snapshot flags are:
 -remote=false
   Manage remote snapshots.

V23 snapshot create - Create a new project snapshot

The "v23 snapshot create <label>" command captures the current project state in
a manifest and, depending on the value of the -remote flag, the command either
stores the manifest in the local $V23_ROOT/.snapshots directory, or in the
manifest repository, pushing the change to the remote repository and thus making
it available globally.

Internally, snapshots are organized as follows:

 <snapshot-dir>/
   labels/
     <label1>/
       <label1-snapshot1>
       <label1-snapshot2>
       ...
     <label2>/
       <label2-snapshot1>
       <label2-snapshot2>
       ...
     <label3>/
     ...
   <label1> # a symlink to the latest <label1-snapshot*>
   <label2> # a symlink to the latest <label2-snapshot*>
   ...

NOTE: Unlike the v23 tool commands, the above internal organization is not an
API. It is an implementation and can change without notice.

Usage:
   v23 snapshot create [flags] <label>

<label> is the snapshot label.

The v23 snapshot create flags are:
 -time-format=2006-01-02T15:04:05Z07:00
   Time format for snapshot file name.

V23 snapshot list - List existing project snapshots

The "snapshot list" command lists existing snapshots of the labels specified as
command-line arguments. If no arguments are provided, the command lists
snapshots for all known labels.

Usage:
   v23 snapshot list <label ...>

<label ...> is a list of snapshot labels.

V23 update - Update all vanadium tools and projects

Updates all vanadium projects, builds the latest version of vanadium tools, and
installs the resulting binaries into $V23_ROOT/devtools/bin. The sequence in
which the individual updates happen guarantees that we end up with a consistent
set of tools and source code.

The set of project and tools to update is describe by a manifest. Vanadium
manifests are revisioned and stored in a "manifest" repository, that is
available locally in $V23_ROOT/.manifest. The manifest uses the following XML
schema:

 <manifest>
   <imports>
     <import name="default"/>
     ...
   </imports>
   <projects>
     <project name="release.go.v23"
              path="release/go/src/v.io/v23"
              protocol="git"
              name="https://vanadium.googlesource.com/release.go.v23"
              revision="HEAD"/>
     ...
   </projects>
   <tools>
     <tool name="v23" package="v.io/x/devtools/v23"/>
     ...
   </tools>
 </manifest>

The <import> element can be used to share settings across multiple manifests.
Import names are interpreted relative to the $V23_ROOT/.manifest/v2 directory.
Import cycles are not allowed and if a project or a tool is specified multiple
times, the last specification takes effect. In particular, the elements <project
name="foo" exclude="true"/> and <tool name="bar" exclude="true"/> can be used to
exclude previously included projects and tools.

The tool identifies which manifest to use using the following algorithm. If the
$V23_ROOT/.local_manifest file exists, then it is used. Otherwise, the
$V23_ROOT/.manifest/v2/<manifest>.xml file is used, which <manifest> is the
value of the -manifest command-line flag, which defaults to "default".

NOTE: Unlike the v23 tool commands, the above manifest file format is not an
API. It is an implementation and can change without notice.

Usage:
   v23 update [flags]

The v23 update flags are:
 -attempts=1
   Number of attempts before failing.
 -gc=false
   Garbage collect obsolete repositories.
 -manifest=
   Name of the project manifest.

V23 version - Print version

Print version of the v23 tool.

Usage:
   v23 version

V23 help - Display help for commands or topics

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

Usage:
   v23 help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The v23 help flags are:
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
