// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
The git-veyron tool facilitates interaction with the Veyron Gerrit server. In
particular, it can be used to export changelists from a local branch to the
Gerrit server.

Usage:
   git-veyron [flags] <command>

The git-veyron commands are:
   cleanup     Clean up branches that have been merged
   review      Send a changelist from a local branch to Gerrit for review
   status      Print a succint status of the veyron repositories
   version     Print version
   help        Display help for commands or topics
Run "git-veyron help [command]" for command usage.

The git-veyron flags are:
 -v=false
   Print verbose output.

Git-Veyron Cleanup

The cleanup command checks that the given branches have been merged into the
master branch. If a branch differs from the master, it reports the difference
and stops. Otherwise, it deletes the branch.

Usage:
   git-veyron cleanup [flags] <branches>

<branches> is a list of branches to cleanup.

The git-veyron cleanup flags are:
 -f=false
   Ignore unmerged changes.

Git-Veyron Review

Squashes all commits of a local branch into a single "changelist" and sends this
changelist to Gerrit as a single commit. First time the command is invoked, it
generates a Change-Id for the changelist, which is appended to the commit
message. Consecutive invocations of the command use the same Change-Id by
default, informing Gerrit that the incomming commit is an update of an existing
changelist.

Usage:
   git-veyron review [flags]

The git-veyron review flags are:
 -cc=
   Comma-seperated list of emails or LDAPs to cc.
 -check_depcop=true
   Check that no go-depcop violations exist.
 -check_gofmt=true
   Check that no go fmt violations exist.
 -check_uncommitted=true
   Check that no uncommitted changes exist.
 -d=false
   Send a draft changelist.
 -r=
   Comma-seperated list of emails or LDAPs to request review.

Git-Veyron Status

Reports current branches of existing veyron repositories as well as an
indication of the status:
  *  indicates whether a repository contains uncommitted changes
  %  indicates whether a repository contains untracked files

Usage:
   git-veyron status [flags]

The git-veyron status flags are:
 -show_current=false
   Show the name of the current repo.
 -show_master=false
   Show master branches in the status.
 -show_uncommitted=true
   Indicate if there are any uncommitted changes.
 -show_untracked=true
   Indicate if there are any untracked files.

Git-Veyron Version

Print version of the git-veyron tool.

Usage:
   git-veyron version

Git-Veyron Help

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

The output is formatted to a target width in runes.  The target width is
determined by checking the environment variable CMDLINE_WIDTH, falling back on
the terminal width from the OS, falling back on 80 chars.  By setting
CMDLINE_WIDTH=x, if x > 0 the width is x, if x < 0 the width is unlimited, and
if x == 0 or is unset one of the fallbacks is used.

Usage:
   git-veyron help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The git-veyron help flags are:
 -style=text
   The formatting style for help output, either "text" or "godoc".
*/
package main
