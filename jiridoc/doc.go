// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Command jiri is a multi-purpose tool for multi-repo development.

Usage:
   jiri [flags] <command>

The jiri commands are:
   cl           Manage project changelists
   contributors List project contributors
   import       Adds imports to .jiri_manifest file
   project      Manage the jiri projects
   rebuild      Rebuild all jiri tools
   snapshot     Manage project snapshots
   update       Update all jiri tools and projects
   upgrade      Upgrade jiri to new-style manifests
   help         Display help for commands or topics
The jiri external commands are:
   api          Manage vanadium public API
   copyright    Manage vanadium copyright
   dockergo     Execute the go command in a docker container
   go           Execute the go tool using the vanadium environment
   goext        Vanadium extensions of the go tool
   oncall       Manage vanadium oncall schedule
   run          Run an executable using the specified profile and target's
                environment
   test         Manage vanadium tests
   v23-profile  Manage profiles

The jiri additional help topics are:
   filesystem  Description of jiri file system layout
   manifest    Description of manifest files

The jiri flags are:
 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

The global flags are:
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.
 -time=false
   Dump timing information to stderr before exiting the program.

Jiri cl - Manage project changelists

Manage project changelists.

Usage:
   jiri cl [flags] <command>

The jiri cl commands are:
   cleanup     Clean up changelists that have been merged
   mail        Mail a changelist for review
   new         Create a new local branch for a changelist
   sync        Bring a changelist up to date

The jiri cl flags are:
 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri cl cleanup - Clean up changelists that have been merged

Command "cleanup" checks that the given branches have been merged into the
corresponding remote branch. If a branch differs from the corresponding remote
branch, the command reports the difference and stops. Otherwise, it deletes the
given branches.

Usage:
   jiri cl cleanup [flags] <branches>

<branches> is a list of branches to cleanup.

The jiri cl cleanup flags are:
 -f=false
   Ignore unmerged changes.
 -remote-branch=master
   Name of the remote branch the CL pertains to.

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri cl mail - Mail a changelist for review

Command "mail" squashes all commits of a local branch into a single "changelist"
and mails this changelist to Gerrit as a single commit. First time the command
is invoked, it generates a Change-Id for the changelist, which is appended to
the commit message. Consecutive invocations of the command use the same
Change-Id by default, informing Gerrit that the incomming commit is an update of
an existing changelist.

Usage:
   jiri cl mail [flags]

The jiri cl mail flags are:
 -autosubmit=false
   Automatically submit the changelist when feasiable.
 -cc=
   Comma-seperated list of emails or LDAPs to cc.
 -check-uncommitted=true
   Check that no uncommitted changes exist.
 -d=false
   Send a draft changelist.
 -edit=true
   Open an editor to edit the CL description.
 -host=
   Gerrit host to use.  Defaults to gerrit host specified in manifest.
 -m=
   CL description.
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
 -verify=true
   Run pre-push git hooks.

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri cl new - Create a new local branch for a changelist

Command "new" creates a new local branch for a changelist. In particular, it
forks a new branch with the given name from the current branch and records the
relationship between the current branch and the new branch in the .jiri metadata
directory. The information recorded in the .jiri metadata directory tracks
dependencies between CLs and is used by the "jiri cl sync" and "jiri cl mail"
commands.

Usage:
   jiri cl new [flags] <name>

<name> is the changelist name.

The jiri cl new flags are:
 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri cl sync - Bring a changelist up to date

Command "sync" brings the CL identified by the current branch up to date with
the branch tracking the remote branch this CL pertains to. To do that, the
command uses the information recorded in the .jiri metadata directory to
identify the sequence of dependent CLs leading to the current branch. The
command then iterates over this sequence bringing each of the CLs up to date
with its ancestor. The end result of this process is that all CLs in the
sequence are up to date with the branch that tracks the remote branch this CL
pertains to.

NOTE: It is possible that the command cannot automatically merge changes in an
ancestor into its dependent. When that occurs, the command is aborted and prints
instructions that need to be followed before the command can be retried.

Usage:
   jiri cl sync [flags]

The jiri cl sync flags are:
 -remote-branch=master
   Name of the remote branch the CL pertains to.

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri contributors - List project contributors

Lists project contributors. Projects to consider can be specified as an
argument. If no projects are specified, all projects in the current manifest are
considered by default.

Usage:
   jiri contributors [flags] <projects>

<projects> is a list of projects to consider.

The jiri contributors flags are:
 -aliases=
   Path to the aliases file.
 -n=false
   Show number of contributions.

 -color=true
   Use color to format output.
 -v=false
   Print verbose output.

Jiri import

Command "import" adds imports to the $JIRI_ROOT/.jiri_manifest file, which
specifies manifest information for the jiri tool.  The file is created if it
doesn't already exist, otherwise additional imports are added to the existing
file.  The arguments and flags configure the <import> element that is added to
the manifest.

Run "jiri help manifest" for details on manifests.

Usage:
   jiri import [flags] <remote> <manifest>

<remote> specifies the remote repository that contains your manifest project.

<manifest> specifies the manifest file to use from the manifest project.

The jiri import flags are:
 -mode=append
   The import mode:
      append    - Create file if it doesn't exist, or append to existing file.
      overwrite - Write file regardless of whether it already exists.
 -name=
   The name of the remote manifest project, used to disambiguate manifest
   projects with the same remote.  Typically empty.
 -out=
   The output file.  Uses $JIRI_ROOT/.jiri_manifest if unspecified.  Uses stdout
   if set to "-".
 -path=
   Path to store the manifest project locally.  Uses "manifest" if unspecified.
 -protocol=git
   The version control protocol used by the remote manifest project.
 -remotebranch=master
   The branch of the remote manifest project to track.
 -revision=HEAD
   The revision of the remote manifest project to reset to during "jiri update".
 -root=
   Root to store the manifest project locally.

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri project - Manage the jiri projects

Manage the jiri projects.

Usage:
   jiri project [flags] <command>

The jiri project commands are:
   clean        Restore jiri projects to their pristine state
   list         List existing jiri projects and branches
   shell-prompt Print a succinct status of projects suitable for shell prompts
   poll         Poll existing jiri projects

The jiri project flags are:
 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri project clean - Restore jiri projects to their pristine state

Restore jiri projects back to their master branches and get rid of all the local
branches and changes.

Usage:
   jiri project clean [flags] <project ...>

<project ...> is a list of projects to clean up.

The jiri project clean flags are:
 -branches=false
   Delete all non-master branches.

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri project list - List existing jiri projects and branches

Inspect the local filesystem and list the existing projects and branches.

Usage:
   jiri project list [flags]

The jiri project list flags are:
 -branches=false
   Show project branches.
 -nopristine=false
   If true, omit pristine projects, i.e. projects with a clean master branch and
   no other branches.

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri project shell-prompt - Print a succinct status of projects suitable for shell prompts

Reports current branches of jiri projects (repositories) as well as an
indication of each project's status:
  *  indicates that a repository contains uncommitted changes
  %  indicates that a repository contains untracked files

Usage:
   jiri project shell-prompt [flags]

The jiri project shell-prompt flags are:
 -check-dirty=true
   If false, don't check for uncommitted changes or untracked files. Setting
   this option to false is dangerous: dirty master branches will not appear in
   the output.
 -show-name=false
   Show the name of the current repo.

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri project poll - Poll existing jiri projects

Poll jiri projects that can affect the outcome of the given tests and report
whether any new changes in these projects exist. If no tests are specified, all
projects are polled by default.

Usage:
   jiri project poll [flags] <test ...>

<test ...> is a list of tests that determine what projects to poll.

The jiri project poll flags are:
 -manifest=
   Name of the project manifest.

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri rebuild - Rebuild all jiri tools

Rebuilds all jiri tools and installs the resulting binaries into
$JIRI_ROOT/.jiri_root/bin. This is similar to "jiri update", but does not update
any projects before building the tools. The set of tools to rebuild is described
in the manifest.

Run "jiri help manifest" for details on manifests.

Usage:
   jiri rebuild [flags]

The jiri rebuild flags are:
 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri snapshot - Manage project snapshots

The "jiri snapshot" command can be used to manage project snapshots. In
particular, it can be used to create new snapshots and to list existing
snapshots.

The command-line flag "-remote" determines whether the command pertains to
"local" snapshots that are only stored locally or "remote" snapshots the are
revisioned in the manifest repository.

Usage:
   jiri snapshot [flags] <command>

The jiri snapshot commands are:
   create      Create a new project snapshot
   list        List existing project snapshots

The jiri snapshot flags are:
 -remote=false
   Manage remote snapshots.

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri snapshot create - Create a new project snapshot

The "jiri snapshot create <label>" command captures the current project state in
a manifest and, depending on the value of the -remote flag, the command either
stores the manifest in the local $JIRI_ROOT/.snapshots directory, or in the
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

NOTE: Unlike the jiri tool commands, the above internal organization is not an
API. It is an implementation and can change without notice.

Usage:
   jiri snapshot create [flags] <label>

<label> is the snapshot label.

The jiri snapshot create flags are:
 -time-format=2006-01-02T15:04:05Z07:00
   Time format for snapshot file name.

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -remote=false
   Manage remote snapshots.
 -v=false
   Print verbose output.

Jiri snapshot list - List existing project snapshots

The "snapshot list" command lists existing snapshots of the labels specified as
command-line arguments. If no arguments are provided, the command lists
snapshots for all known labels.

Usage:
   jiri snapshot list [flags] <label ...>

<label ...> is a list of snapshot labels.

The jiri snapshot list flags are:
 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -remote=false
   Manage remote snapshots.
 -v=false
   Print verbose output.

Jiri update - Update all jiri tools and projects

Updates all projects, builds the latest version of all tools, and installs the
resulting binaries into $JIRI_ROOT/.jiri_root/bin. The sequence in which the
individual updates happen guarantees that we end up with a consistent set of
tools and source code. The set of projects and tools to update is described in
the manifest.

Run "jiri help manifest" for details on manifests.

Usage:
   jiri update [flags]

The jiri update flags are:
 -attempts=1
   Number of attempts before failing.
 -gc=false
   Garbage collect obsolete repositories.
 -manifest=
   Name of the project manifest.

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri upgrade - Upgrade jiri to new-style manifests

Upgrades jiri to use new-style manifests.

The old (deprecated) behavior only allowed a single manifest repository, located
in $JIRI_ROOT/.manifest.  The initial manifest file is located as follows:
  1) Use -manifest flag, if non-empty.  If it's empty...
  2) Use $JIRI_ROOT/.local_manifest file.  If it doesn't exist...
  3) Use $JIRI_ROOT/.manifest/v2/default.

The new behavior allows multiple manifest repositories, by allowing imports to
specify project attributes describing the remote repository.  The -manifest flag
is no longer allowed to be set; the initial manifest file is always located in
$JIRI_ROOT/.jiri_manifest.  The .local_manifest file is ignored.

During the transition phase, both old and new behaviors are supported.  The jiri
tool uses the existence of the $JIRI_ROOT/.jiri_manifest file as the signal; if
it exists we run the new behavior, otherwise we run the old behavior.

The new behavior includes a "jiri import" command, which writes or updates the
.jiri_manifest file.  The new bootstrap procedure runs "jiri import", and it is
intended as a regular command to add imports to your jiri environment.

This upgrade command eases the transition by writing an initial .jiri_manifest
file for you.  If you have an existing .local_manifest file, its contents will
be incorporated into the new .jiri_manifest file, and it will be renamed to
.local_manifest.BACKUP.  The -revert flag deletes the .jiri_manifest file, and
restores the .local_manifest file.

Usage:
   jiri upgrade [flags] <kind>

<kind> specifies the kind of upgrade, one of "v23" or "fuchsia".

The jiri upgrade flags are:
 -revert=false
   Revert the upgrade by deleting the $JIRI_ROOT/.jiri_manifest file.

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri help - Display help for commands or topics

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

Usage:
   jiri help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The jiri help flags are:
 -style=compact
   The formatting style for help output:
      compact   - Good for compact cmdline output.
      full      - Good for cmdline output, shows all global flags.
      godoc     - Good for godoc processing.
      shortonly - Only output short description.
   Override the default by setting the CMDLINE_STYLE environment variable.
 -width=<terminal width>
   Format output to this target width in runes, or unlimited if width < 0.
   Defaults to the terminal width if available.  Override the default by setting
   the CMDLINE_WIDTH environment variable.

Jiri api - Manage vanadium public API

Use this command to ensure that no unintended changes are made to the vanadium
public API.

Usage:
   jiri api [flags] <command>

The jiri api commands are:
   check       Check if any changes have been made to the public API
   fix         Update api files to reflect changes to the public API

The jiri api flags are:
 -color=true
   Use color to format output.
 -gotools-bin=
   The path to the gotools binary to use. If empty, gotools will be built if
   necessary.
 -manifest=
   Name of the project manifest.
 -merge-policies=+CCFLAGS,+CGO_CFLAGS,+CGO_CXXFLAGS,+CGO_LDFLAGS,+CXXFLAGS,GOARCH,GOOS,GOPATH:,^GOROOT*,+LDFLAGS,:PATH,VDLPATH:
   specify policies for merging environment variables
 -n=false
   Show what commands will run but do not execute them.
 -profiles-manifest=$JIRI_ROOT/.jiri_v23_profiles
   specify the profiles XML manifest filename.
 -v=false
   Print verbose output.

Jiri api check - Check if any changes have been made to the public API

Check if any changes have been made to the public API.

Usage:
   jiri api check [flags] <projects>

<projects> is a list of vanadium projects to check. If none are specified, all
projects that require a public API check upon presubmit are checked.

The jiri api check flags are:
 -detailed=true
   If true, shows each API change in an expanded form. Otherwise, only a summary
   is shown.

 -color=true
   Use color to format output.
 -gotools-bin=
   The path to the gotools binary to use. If empty, gotools will be built if
   necessary.
 -manifest=
   Name of the project manifest.
 -merge-policies=+CCFLAGS,+CGO_CFLAGS,+CGO_CXXFLAGS,+CGO_LDFLAGS,+CXXFLAGS,GOARCH,GOOS,GOPATH:,^GOROOT*,+LDFLAGS,:PATH,VDLPATH:
   specify policies for merging environment variables
 -n=false
   Show what commands will run but do not execute them.
 -profiles-manifest=$JIRI_ROOT/.jiri_v23_profiles
   specify the profiles XML manifest filename.
 -v=false
   Print verbose output.

Jiri api fix - Update api files to reflect changes to the public API

Update .api files to reflect changes to the public API.

Usage:
   jiri api fix [flags] <projects>

<projects> is a list of vanadium projects to update. If none are specified, all
project APIs are updated.

The jiri api fix flags are:
 -color=true
   Use color to format output.
 -gotools-bin=
   The path to the gotools binary to use. If empty, gotools will be built if
   necessary.
 -manifest=
   Name of the project manifest.
 -merge-policies=+CCFLAGS,+CGO_CFLAGS,+CGO_CXXFLAGS,+CGO_LDFLAGS,+CXXFLAGS,GOARCH,GOOS,GOPATH:,^GOROOT*,+LDFLAGS,:PATH,VDLPATH:
   specify policies for merging environment variables
 -n=false
   Show what commands will run but do not execute them.
 -profiles-manifest=$JIRI_ROOT/.jiri_v23_profiles
   specify the profiles XML manifest filename.
 -v=false
   Print verbose output.

Jiri copyright - Manage vanadium copyright

This command can be used to check if all source code files of Vanadium projects
contain the appropriate copyright header and also if all projects contains the
appropriate licensing files. Optionally, the command can be used to fix the
appropriate copyright headers and licensing files.

In order to ignore checked in third-party assets which have their own copyright
and licensing headers a ".jiriignore" file can be added to a project. The
".jiriignore" file is expected to contain a single regular expression pattern
per line.

Usage:
   jiri copyright [flags] <command>

The jiri copyright commands are:
   check       Check copyright headers and licensing files
   fix         Fix copyright headers and licensing files

The jiri copyright flags are:
 -color=true
   Use color to format output.
 -manifest=
   Name of the project manifest.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri copyright check - Check copyright headers and licensing files

Check copyright headers and licensing files.

Usage:
   jiri copyright check [flags] <projects>

<projects> is a list of projects to check.

The jiri copyright check flags are:
 -color=true
   Use color to format output.
 -manifest=
   Name of the project manifest.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri copyright fix - Fix copyright headers and licensing files

Fix copyright headers and licensing files.

Usage:
   jiri copyright fix [flags] <projects>

<projects> is a list of projects to fix.

The jiri copyright fix flags are:
 -color=true
   Use color to format output.
 -manifest=
   Name of the project manifest.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri dockergo - Execute the go command in a docker container

Executes a Go command in a docker container. This is primarily aimed at the
builds of Linux binaries and libraries where there is a dependence on cgo. This
allows for compilation (and cross-compilation) without polluting the host
filesystem with compilers, C-headers, libraries etc. as dependencies are
encapsulated in the docker image.

The docker image is expected to have the appropriate C-compiler and any
pre-built headers/libraries to be linked in.  It is also expected to have the
appropriate environment variables (such as CGO_ENABLED, CGO_CFLAGS etc) set.

Sample usage on *all* platforms (Linux/OS X):

Build the "./foo" package for the host architecture and linux (command works
from OS X as well):

    jiri-dockergo build

Build for linux/arm from any host (including OS X):

    GOARCH=arm jiri-dockergo build

For more information on docker see https://www.docker.com.

For more information on the design of this particular tool including the
definitions of default images, see:
https://docs.google.com/document/d/1Ud-QUVOjsaya57kgq0j24wDwTzKKE7o_PShQQs0DR5w/edit?usp=sharing

While the targets are built using the toolchain in the docker image, a local Go
installation is still required for Vanadium-specific compilation prep work -
such as invoking the VDL compiler on packages to generate up-to-date .go files.

Usage:
   jiri dockergo [flags] <arg ...>

<arg ...> is a list of arguments for the go tool.

The jiri dockergo flags are:
 -color=true
   Use color to format output.
 -merge-policies=+CCFLAGS,+CGO_CFLAGS,+CGO_CXXFLAGS,+CGO_LDFLAGS,+CXXFLAGS,GOARCH,GOOS,GOPATH:,^GOROOT*,+LDFLAGS,:PATH,VDLPATH:
   specify policies for merging environment variables
 -n=false
   Show what commands will run but do not execute them.
 -profiles=base,jiri
   a comma separated list of profiles to use
 -profiles-manifest=$JIRI_ROOT/.jiri_v23_profiles
   specify the profiles XML manifest filename.
 -skip-profiles=false
   if set, no profiles will be used
 -target=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form:
   <arch>-<os>[@<version>]|<arch>-<val>[@<version>]
 -v=false
   Print verbose output.

Jiri go - Execute the go tool using the vanadium environment

Wrapper around the 'go' tool that can be used for compilation of vanadium Go
sources. It takes care of vanadium-specific setup, such as setting up the Go
specific environment variables or making sure that VDL generated files are
regenerated before compilation.

Usage:
   jiri go [flags] <arg ...>

<arg ...> is a list of arguments for the go tool.

The jiri go flags are:
 -color=true
   Use color to format output.
 -merge-policies=+CCFLAGS,+CGO_CFLAGS,+CGO_CXXFLAGS,+CGO_LDFLAGS,+CXXFLAGS,GOARCH,GOOS,GOPATH:,^GOROOT*,+LDFLAGS,:PATH,VDLPATH:
   specify policies for merging environment variables
 -n=false
   Show what commands will run but do not execute them.
 -profiles=base,jiri
   a comma separated list of profiles to use
 -profiles-manifest=$JIRI_ROOT/.jiri_v23_profiles
   specify the profiles XML manifest filename.
 -skip-profiles=false
   if set, no profiles will be used
 -target=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form:
   <arch>-<os>[@<version>]|<arch>-<val>[@<version>]
 -v=false
   Print verbose output.

Jiri goext - Vanadium extensions of the go tool

Vanadium extensions of the go tool.

Usage:
   jiri goext [flags] <command>

The jiri goext commands are:
   distclean   Restore the vanadium Go workspaces to their pristine state

The jiri goext flags are:
 -color=true
   Use color to format output.
 -merge-policies=+CCFLAGS,+CGO_CFLAGS,+CGO_CXXFLAGS,+CGO_LDFLAGS,+CXXFLAGS,GOARCH,GOOS,GOPATH:,^GOROOT*,+LDFLAGS,:PATH,VDLPATH:
   specify policies for merging environment variables
 -n=false
   Show what commands will run but do not execute them.
 -profiles-manifest=$JIRI_ROOT/.jiri_v23_profiles
   specify the profiles XML manifest filename.
 -v=false
   Print verbose output.

Jiri goext distclean - Restore the vanadium Go workspaces to their pristine
state

Unlike the 'go clean' command, which only removes object files for packages in
the source tree, the 'goext disclean' command removes all object files from
vanadium Go workspaces. This functionality is needed to avoid accidental use of
stale object files that correspond to packages that no longer exist in the
source tree.

Usage:
   jiri goext distclean [flags]

The jiri goext distclean flags are:
 -color=true
   Use color to format output.
 -merge-policies=+CCFLAGS,+CGO_CFLAGS,+CGO_CXXFLAGS,+CGO_LDFLAGS,+CXXFLAGS,GOARCH,GOOS,GOPATH:,^GOROOT*,+LDFLAGS,:PATH,VDLPATH:
   specify policies for merging environment variables
 -n=false
   Show what commands will run but do not execute them.
 -profiles-manifest=$JIRI_ROOT/.jiri_v23_profiles
   specify the profiles XML manifest filename.
 -v=false
   Print verbose output.

Jiri oncall - Manage vanadium oncall schedule

Manage vanadium oncall schedule. If no subcommand is given, it shows the LDAP of
the current oncall.

Usage:
   jiri oncall [flags]
   jiri oncall [flags] <command>

The jiri oncall commands are:
   list        List available oncall schedule

The jiri oncall flags are:
 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri oncall list - List available oncall schedule

List available oncall schedule.

Usage:
   jiri oncall list [flags]

The jiri oncall list flags are:
 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri run - Run an executable using the specified profile and target's
environment

Run an executable using the specified profile and target's environment.

Usage:
   jiri run [flags] <executable> [arg ...]

<executable> [arg ...] is the executable to run and any arguments to pass
verbatim to the executable.

The jiri run flags are:
 -color=true
   Use color to format output.
 -merge-policies=+CCFLAGS,+CGO_CFLAGS,+CGO_CXXFLAGS,+CGO_LDFLAGS,+CXXFLAGS,GOARCH,GOOS,GOPATH:,^GOROOT*,+LDFLAGS,:PATH,VDLPATH:
   specify policies for merging environment variables
 -n=false
   Show what commands will run but do not execute them.
 -profiles=base,jiri
   a comma separated list of profiles to use
 -profiles-manifest=$JIRI_ROOT/.jiri_v23_profiles
   specify the profiles XML manifest filename.
 -skip-profiles=false
   if set, no profiles will be used
 -target=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form:
   <arch>-<os>[@<version>]|<arch>-<val>[@<version>]
 -v=false
   Print verbose output.

Jiri test - Manage vanadium tests

Manage vanadium tests.

Usage:
   jiri test [flags] <command>

The jiri test commands are:
   generate    Generate supporting code for v23 integration tests
   project     Run tests for a vanadium project
   run         Run vanadium tests
   list        List vanadium tests

The jiri test flags are:
 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri test generate - Generate supporting code for v23 integration tests

The generate command supports the vanadium integration test framework and unit
tests by generating go files that contain supporting code.  jiri test generate
is intended to be invoked via the 'go generate' mechanism and the resulting
files are to be checked in.

Integration tests are functions of the following form:

    func V23Test<x>(i *v23tests.T)

These functions are typically defined in 'external' *_test packages, to ensure
better isolation.  But they may also be defined directly in the 'internal' *
package.  The following helper functions will be generated:

    func TestV23<x>(t *testing.T) {
      v23tests.RunTest(t, V23Test<x>)
    }

In addition a TestMain function is generated, if it doesn't already exist.  Note
that Go requires that at most one TestMain function is defined across both the
internal and external test packages.

The generated TestMain performs common initialization, and also performs child
process dispatching for tests that use "v.io/veyron/test/modules".

Usage:
   jiri test generate [flags] [packages]

list of go packages

The jiri test generate flags are:
 -merge-policies=
   specify policies for merging environment variables
 -prefix=v23
   Specifies the prefix to use for generated files. Up to two files may
   generated, the defaults are v23_test.go and v23_internal_test.go, or
   <prefix>_test.go and <prefix>_internal_test.go.
 -progress=false
   Print verbose progress information.

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri test project - Run tests for a vanadium project

Runs tests for a vanadium project that is by the remote URL specified as the
command-line argument. Projects hosted on googlesource.com, can be specified
using the basename of the URL (e.g. "vanadium.go.core" implies
"https://vanadium.googlesource.com/vanadium.go.core").

Usage:
   jiri test project [flags] <project>

<project> identifies the project for which to run tests.

The jiri test project flags are:
 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri test run - Run vanadium tests

Run vanadium tests.

Usage:
   jiri test run [flags] <name...>

<name...> is a list names identifying the tests to run.

The jiri test run flags are:
 -blessings-root=dev.v.io
   The blessings root.
 -clean-go=true
   Specify whether to remove Go object files and binaries before running the
   tests. Setting this flag to 'false' may lead to faster Go builds, but it may
   also result in some source code changes not being reflected in the tests
   (e.g., if the change was made in a different Go workspace).
 -merge-policies=+CCFLAGS,+CGO_CFLAGS,+CGO_CXXFLAGS,+CGO_LDFLAGS,+CXXFLAGS,GOARCH,GOOS,GOPATH:,^GOROOT*,+LDFLAGS,:PATH,VDLPATH:
   specify policies for merging environment variables
 -num-test-workers=<runtime.NumCPU()>
   Set the number of test workers to use; use 1 to serialize all tests.
 -output-dir=
   Directory to output test results into.
 -part=-1
   Specify which part of the test to run.
 -pkgs=
   Comma-separated list of Go package expressions that identify a subset of
   tests to run; only relevant for Go-based tests
 -profiles-manifest=$JIRI_ROOT/.jiri_v23_profiles
   specify the profiles XML manifest filename.
 -v23.namespace.root=/ns.dev.v.io:8101
   The namespace root.

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri test list - List vanadium tests

List vanadium tests.

Usage:
   jiri test list [flags]

The jiri test list flags are:
 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri v23-profile - Manage profiles

Profiles are used to manage external sofware dependencies and offer a balance
between providing no support at all and a full blown package manager. Profiles
can be built natively as well as being cross compiled. A profile is a named
collection of software required for a given system component or application.
Current example profiles include 'syncbase' which consists of the leveldb and
snappy libraries or 'android' which consists of all of the android components
and downloads needed to build android applications. Profiles are built for
specific targets.

Targets

Profiles generally refer to uncompiled source code that needs to be compiled for
a specific "target". Targets hence represent compiled code and consist of:

1. A 'tag' that can be used a short hand for refering to a target

2. An 'architecture' that refers to the CPU to be generate code for

3. An 'operating system' that refers to the operating system to generate code
for

4. A lexicographically orderd set of supported versions, one of which is
designated as the default.

5. An 'environment' which is a set of environment variables to use when
compiling the profile

Targets thus provide the basic support needed for cross compilation.

Targets are versioned and multiple versions may be installed and used
simultaneously. Versions are ordered lexicographically and each target specifies
a 'default' version to be used when a specific version is not explicitly
requested. A request to 'upgrade' the profile will result in the installation of
the default version of the targets currently installed if that default version
is not already installed.

The Supported Commands

Profiles, or more correctly, targets for specific profiles may be installed or
removed. When doing so, the name of the profile is required, but the other
components of the target are optional and will default to the values of the
system that the commands are run on (so-called native builds) and the default
version for that target. Once a profile is installed it may be referred to by
its tag for subsequent removals.

The are also update and cleanup commands. Update installs the default version of
the requested profile or for all profiles for the already installed targets.
Cleanup will uninstall targets whose version is older than the default.

Finally, there are commands to list the available and installed profiles and to
access the environment variables specified and stored in each profile
installation and a command (recreate) to generate a list of commands that can be
run to recreate the currently installed profiles.

The Manifest

The profiles packages manages a manifest that tracks the installed profiles and
their configurations. Other command line tools and packages are expected to read
information about the currently installed profiles from this manifest via the
profiles package. The profile command line tools support displaying the manifest
(via the list command) or for specifying an alternate version of the file (via
the -manifest flag) which is generally useful for debugging.

Adding Profiles

Profiles are intended to be provided as go packages that register themselves
with the profile command line tools via the *v.io/jiri/profiles* package. They
must implement the interfaces defined by that package and be imported (e.g.
import _ "myprofile") by the command line tools that are to use them.

Usage:
   jiri v23-profile [flags] <command>

The jiri v23-profile commands are:
   install     Install the given profiles
   list        List available or installed profiles
   env         Display profile environment variables
   uninstall   Uninstall the given profiles
   update      Install the latest default version of the given profiles
   cleanup     Cleanup the locally installed profiles

The jiri v23-profile flags are:
 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri v23-profile install - Install the given profiles

Install the given profiles.

Usage:
   jiri v23-profile install [flags] <profiles>

<profiles> is a list of profiles to install.

The jiri v23-profile install flags are:
 -env=
   specifcy an environment variable in the form: <var>=[<val>],...
 -go.sysroot-image=
   sysroot image for cross compiling to the currently specified target
 -go.sysroot-image-dirs-to-use=/lib:/usr/lib:/usr/include
   a colon separated list of directories to use from the sysroot image
 -mojo-dev.dir=
   Path of mojo repo checkout.
 -profiles-manifest=$JIRI_ROOT/.jiri_v23_profiles
   specify the profiles XML manifest filename.
 -target=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form:
   <arch>-<os>[@<version>]|<arch>-<val>[@<version>]

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri v23-profile list - List available or installed profiles

List available or installed profiles.

Usage:
   jiri v23-profile list [flags] [<profiles>]

<profiles> is a list of profiles to list, defaulting to all profiles if none are
specifically requested.

The jiri v23-profile list flags are:
 -available=false
   print the list of available profiles
 -info=
   The following fields for use with --profile-info are available:
   	SchemaVersion - the version of the profiles implementation.
   	Target.InstallationDir - the installation directory of the requested profile.
   	Target.CommandLineEnv - the environment variables specified via the command line when installing this profile target.
   	Target.Env - the environment variables computed by the profile installation process for this target.
   	Target.Command - a command that can be used to create this profile.
   	Note: if no --target is specified then the requested field will be displayed for all targets.
   	Profile.Description - description of the requested profile.
   	Profile.Root - the root directory of the requested profile.
   	Profile.Versions - the set of supported versions for this profile.
   	Profile.DefaultVersion - the default version of the requested profile.
   	Profile.LatestVersion - the latest version available for the requested profile.
   	Note: if no profiles are specified then the requested field will be displayed for all profiles.
 -profiles-manifest=$JIRI_ROOT/.jiri_v23_profiles
   specify the profiles XML manifest filename.
 -show-profiles-manifest=false
   print out the manifest file
 -target=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form:
   <arch>-<os>[@<version>]|<arch>-<val>[@<version>]
 -v=false
   print more detailed information

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.

Jiri v23-profile env - Display profile environment variables

List profile specific and target specific environment variables. If the
requested environment variable name ends in = then only the value will be
printed, otherwise both name and value are printed, i.e. GOPATH="foo" vs just
"foo".

If no environment variable names are requested then all will be printed in
<name>=<val> format.

Usage:
   jiri v23-profile env [flags] [<environment variable names>]

[<environment variable names>] is an optional list of environment variables to
display

The jiri v23-profile env flags are:
 -merge-policies=+CCFLAGS,+CGO_CFLAGS,+CGO_CXXFLAGS,+CGO_LDFLAGS,+CXXFLAGS,GOARCH,GOOS,GOPATH:,^GOROOT*,+LDFLAGS,:PATH,VDLPATH:
   specify policies for merging environment variables
 -profiles=base,jiri
   a comma separated list of profiles to use
 -profiles-manifest=$JIRI_ROOT/.jiri_v23_profiles
   specify the profiles XML manifest filename.
 -target=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form:
   <arch>-<os>[@<version>]|<arch>-<val>[@<version>]
 -v=false
   print more detailed information

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.

Jiri v23-profile uninstall - Uninstall the given profiles

Uninstall the given profiles.

Usage:
   jiri v23-profile uninstall [flags] <profiles>

<profiles> is a list of profiles to uninstall.

The jiri v23-profile uninstall flags are:
 -all-targets=false
   apply to all targets for the specified profile(s)
 -go.sysroot-image=
   sysroot image for cross compiling to the currently specified target
 -go.sysroot-image-dirs-to-use=/lib:/usr/lib:/usr/include
   a colon separated list of directories to use from the sysroot image
 -profiles-manifest=$JIRI_ROOT/.jiri_v23_profiles
   specify the profiles XML manifest filename.
 -target=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form:
   <arch>-<os>[@<version>]|<arch>-<val>[@<version>]

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

Jiri v23-profile update - Install the latest default version of the given
profiles

Install the latest default version of the given profiles.

Usage:
   jiri v23-profile update [flags] <profiles>

<profiles> is a list of profiles to update, if omitted all profiles are updated.

The jiri v23-profile update flags are:
 -profiles-manifest=$JIRI_ROOT/.jiri_v23_profiles
   specify the profiles XML manifest filename.
 -v=false
   print more detailed information

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.

Jiri v23-profile cleanup - Cleanup the locally installed profiles

Cleanup the locally installed profiles. This is generally required when
recovering from earlier bugs or when preparing for a subsequent change to the
profiles implementation.

Usage:
   jiri v23-profile cleanup [flags] <profiles>

<profiles> is a list of profiles to cleanup, if omitted all profiles are
cleaned.

The jiri v23-profile cleanup flags are:
 -ensure-specific-versions-are-set=false
   ensure that profile targets have a specific version set
 -gc=false
   uninstall profile targets that are older than the current default
 -profiles-manifest=$JIRI_ROOT/.jiri_v23_profiles
   specify the profiles XML manifest filename.
 -rewrite-profiles-manifest=false
   rewrite the profiles manifest file to use the latest schema version
 -rm-all=false
   remove profiles manifest and all profile generated output files.
 -v=false
   print more detailed information

 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.

Jiri filesystem - Description of jiri file system layout

All data managed by the jiri tool is located in the file system under a root
directory, colloquially called the jiri root directory.  The file system layout
looks like this:

 [root]                              # root directory (name picked by user)
 [root]/.jiri_root                   # root metadata directory
 [root]/.jiri_root/bin               # contains tool binaries (jiri, etc.)
 [root]/.jiri_root/update_history    # contains history of update snapshots
 [root]/.manifest                    # contains jiri manifests
 [root]/[project1]                   # project directory (name picked by user)
 [root]/[project1]/.jiri             # project metadata directory
 [root]/[project1]/.jiri/metadata.v2 # project metadata file
 [root]/[project1]/.jiri/<<cls>>     # project per-cl metadata directories
 [root]/[project1]/<<files>>         # project files
 [root]/[project2]...

The [root] and [projectN] directory names are picked by the user.  The <<cls>>
are named via jiri cl new, and the <<files>> are named as the user adds files
and directories to their project.  All other names above have special meaning to
the jiri tool, and cannot be changed; you must ensure your path names don't
collide with these special names.

There are two ways to run the jiri tool:

1) Shim script (recommended approach).  This is a shell script that looks for
the [root] directory.  If the JIRI_ROOT environment variable is set, that is
assumed to be the [root] directory.  Otherwise the script looks for the
.jiri_root directory, starting in the current working directory and walking up
the directory chain.  The search is terminated successfully when the .jiri_root
directory is found; it fails after it reaches the root of the file system.  Thus
the shim must be invoked from the [root] directory or one of its subdirectories.

Once the [root] is found, the JIRI_ROOT environment variable is set to its
location, and [root]/.jiri_root/bin/jiri is invoked.  That file contains the
actual jiri binary.

The point of the shim script is to make it easy to use the jiri tool with
multiple [root] directories on your file system.  Keep in mind that when "jiri
update" is run, the jiri tool itself is automatically updated along with all
projects.  By using the shim script, you only need to remember to invoke the
jiri tool from within the appropriate [root] directory, and the projects and
tools under that [root] directory will be updated.

The shim script is located at [root]/release/go/src/v.io/jiri/scripts/jiri

2) Direct binary.  This is the jiri binary, containing all of the actual jiri
tool logic.  The binary requires the JIRI_ROOT environment variable to point to
the [root] directory.

Note that if you have multiple [root] directories on your file system, you must
remember to run the jiri binary corresponding to the setting of your JIRI_ROOT
environment variable.  Things may fail if you mix things up, since the jiri
binary is updated with each call to "jiri update", and you may encounter version
mismatches between the jiri binary and the various metadata files or other
logic.  This is the reason the shim script is recommended over running the
binary directly.

The binary is located at [root]/.jiri_root/bin/jiri

Jiri manifest - Description of manifest files

Jiri manifests are revisioned and stored in a "manifest" repository, that is
available locally in $JIRI_ROOT/.manifest. The manifest uses the following XML
schema:

 <manifest>
   <imports>
     <import name="default"/>
     ...
   </imports>
   <projects>
     <project name="release.go.jiri"
              path="release/go/src/v.io/jiri"
              protocol="git"
              name="https://vanadium.googlesource.com/release.go.jiri"
              revision="HEAD"/>
     ...
   </projects>
   <tools>
     <tool name="jiri" package="v.io/jiri"/>
     ...
   </tools>
 </manifest>

The <import> element can be used to share settings across multiple manifests.
Import names are interpreted relative to the $JIRI_ROOT/.manifest/v2 directory.
Import cycles are not allowed and if a project or a tool is specified multiple
times, the last specification takes effect. In particular, the elements <project
name="foo" exclude="true"/> and <tool name="bar" exclude="true"/> can be used to
exclude previously included projects and tools.

The tool identifies which manifest to use using the following algorithm. If the
$JIRI_ROOT/.local_manifest file exists, then it is used. Otherwise, the
$JIRI_ROOT/.manifest/v2/<manifest>.xml file is used, where <manifest> is the
value of the -manifest command-line flag, which defaults to "default".
*/
package main
