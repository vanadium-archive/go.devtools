// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
The git-veyron tool facilitates interaction with the Veyron Gerrit server.
In particular, it can be used to export changelists from a local branch
to the Gerrit server.

Usage:
   git-veyron [flags] <command>

The git-veyron commands are:
   cleanup     Clean up branches that have been merged
   review      Send a changelist from a local branch to Gerrit for review
   selfupdate  Update the veyron tool
   status      Print a succint status of the veyron repositories
   version     Print version
   help        Display help for commands or topics
Run "git-veyron help [command]" for command usage.

The git-veyron flags are:
   -v=false: Print verbose output.

Git-Veyron Cleanup

The cleanup command checks that the given branches have been merged
into the master branch. If a branch differs from the master, it
reports the difference and stops. Otherwise, it deletes the branch.

Usage:
   git-veyron cleanup [flags] <branches>

<branches> is a list of branches to cleanup.

The cleanup flags are:
   -f=false: Ignore unmerged changes.

Git-Veyron Review

Squashes all commits of a local branch into a single "changelist" and
sends this changelist to Gerrit as a single commit. First time the
command is invoked, it generates a Change-Id for the changelist, which
is appended to the commit message. Consecutive invocations of the
command use the same Change-Id by default, informing Gerrit that the
incomming commit is an update of an existing changelist.

Usage:
   git-veyron review [flags]

The review flags are:
   -cc=: Comma-seperated list of emails or LDAPs to cc.
   -d=false: Send a draft changelist.
   -r=: Comma-seperated list of emails or LDAPs to request review.

Git-Veyron Selfupdate

Download and install the latest version of the veyron tool.

Usage:
   git-veyron selfupdate [flags]

The selfupdate flags are:
   -manifest=absolute: Name of the project manifest.

Git-Veyron Status

Reports current branches of existing veyron repositories as well as an
indication of the status:
  *  indicates whether a repository contains uncommitted changes
  %  indicates whether a repository contains untracked files

Usage:
   git-veyron status [flags]

The status flags are:
   -show-current=false: Show the name of the current repo.
   -show-master=false: Show master branches in the status.
   -show-uncommitted=true: Indicate if there are any uncommitted changes.
   -show-untracked=true: Indicate if there are any untracked files.

Git-Veyron Version

Print version of the git-veyron tool.

Usage:
   git-veyron version

Git-Veyron Help

Help with no args displays the usage of the parent command.
Help with args displays the usage of the specified sub-command or help topic.
"help ..." recursively displays help for all commands and topics.

Usage:
   git-veyron help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The help flags are:
   -style=text: The formatting style for help output, either "text" or "godoc".
*/
package main
