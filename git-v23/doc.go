// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
The git-v23 tool facilitates interaction with Vanadium code review system. In
particular, it can be used to export changelists from a local branch to the
Gerrit server.

Usage:
   git-v23 [flags] <command>

The git-v23 commands are:
   cleanup     Clean up branches that have been merged
   review      Send a changelist from a local branch to Gerrit for review
   version     Print version
   help        Display help for commands or topics
Run "git-v23 help [command]" for command usage.

The git-v23 flags are:
 -n=false
   Show what commands will run but do not execute them.
 -nocolor=false
   Do not use color to format output.
 -v=false
   Print verbose output.

Git-V23 Cleanup

The cleanup command checks that the given branches have been merged into the
master branch. If a branch differs from the master, it reports the difference
and stops. Otherwise, it deletes the branch.

Usage:
   git-v23 cleanup [flags] <branches>

<branches> is a list of branches to cleanup.

The git-v23 cleanup flags are:
 -f=false
   Ignore unmerged changes.

Git-V23 Review

Squashes all commits of a local branch into a single "changelist" and sends this
changelist to Gerrit as a single commit. First time the command is invoked, it
generates a Change-Id for the changelist, which is appended to the commit
message. Consecutive invocations of the command use the same Change-Id by
default, informing Gerrit that the incomming commit is an update of an existing
changelist.

Usage:
   git-v23 review [flags]

The git-v23 review flags are:
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
 -presubmit=all
   The type of presubmit tests to run. Valid values: none, all.
 -r=
   Comma-seperated list of emails or LDAPs to request review.

Git-V23 Version

Print version of the git-v23 tool.

Usage:
   git-v23 version

Git-V23 Help

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

The output is formatted to a target width in runes.  The target width is
determined by checking the environment variable CMDLINE_WIDTH, falling back on
the terminal width from the OS, falling back on 80 chars.  By setting
CMDLINE_WIDTH=x, if x > 0 the width is x, if x < 0 the width is unlimited, and
if x == 0 or is unset one of the fallbacks is used.

Usage:
   git-v23 help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The git-v23 help flags are:
 -style=text
   The formatting style for help output, either "text" or "godoc".
*/
package main
