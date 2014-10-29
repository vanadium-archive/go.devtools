// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
The veyron tool helps manage veyron development.

Usage:
   veyron [flags] <command>

The veyron commands are:
   build       Tool for managing veyron builds
   contributors List veyron project contributors
   profile     Manage veyron profiles
   project     Manage veyron projects
   update      Update all veyron tools and projects
   env         Print veyron environment variables
   run         Run an executable using the veyron environment
   go          Execute the go tool using the veyron environment
   goext       Veyron extensions of the go tool
   xgo         Execute the go tool using the veyron environment and cross-compilation
   integration-test Manage integration tests
   version     Print version
   help        Display help for commands or topics
Run "veyron help [command]" for command usage.

The veyron flags are:
   -v=false: Print verbose output.

The global flags are:
   -host-go=go: Go command for the host platform.
   -target-go=go: Go command for the target platform.

Veyron Build

The build command can be used to manage veyron builds. In particular,
it can be used to list known builds and generate new builds.

The builds are represented as manifests and are revisioned using the
manifest repository located in $VEYRON_ROOT/.manifest. Each build is
identified with a tag, which the $VEYRON_ROOT/tools/conf/veyron
configuration file associates with a set of jenkins projects that
determine the stability of the build.

Internally, build manifests are currently organized as follows:

 <manifest-dir>/
   builds/
     <tag1>/
       <tag1-build1>
       <tag1-build2>
       ...
     <tag2>/
       <tag2-build1>
       <tag2-build2>
       ...
     <tag3>/
     ...
   <tag1> # a symlink to a one of <tag1-build*>
   <tag2> # a symlink to a one of <tag2-build*>
   ...

NOTE: Unlike the veyron tool commands, the above internal organization
is not an API. It is an implementation and can change without notice.

Usage:
   veyron build <command>

The build commands are:
   generate    Generate a new veyron build
   list        List existing veyron builds

Veyron Build Generate

Given a build tag, the "buildbot generate" command checks whether all
tests associated with the tag in the $VEYRON_ROOT/tools/conf/buildbot
config file pass. If so, the tool creates a new manifest that captures
the current state of the veyron universe repositories, commits this
manifest to the manifest repository, and updates the build "symlink"
to point to the latest build.

Usage:
   veyron build generate <tag>

<tag> is a build tag.

Veyron Build List

The "buildbot list" command lists existing veyron builds for the tags
specified as command-line arguments. If no arguments are provided, the
command lists builds for all known tags.

Usage:
   veyron build list <tag ...>

<tag ...> is a list of build tags.

Veyron Contributors

Lists veyron project contributors and the number of their
commits. Veyron projects to consider can be specified as an
argument. If no projects are specified, all veyron projects are
considered by default.

Usage:
   veyron contributors <projects>

<projects> is a list of projects to consider.

Veyron Profile

To facilitate development across different platforms, veyron defines
platform-independent profiles that map different platforms to a set
of libraries and tools that can be used for a factor of veyron
development.

Usage:
   veyron profile <command>

The profile commands are:
   list        List supported veyron profiles
   setup       Set up the given veyron profiles

Veyron Profile List

Inspect the host platform and list supported profiles.

Usage:
   veyron profile list

Veyron Profile Setup

Set up the given veyron profiles.

Usage:
   veyron profile setup <profiles>

<profiles> is a list of profiles to set up.

Veyron Project

Manage veyron projects.

Usage:
   veyron project <command>

The project commands are:
   list        List existing veyron projects
   poll        Poll existing veyron projects

Veyron Project List

Inspect the local filesystem and list the existing projects.

Usage:
   veyron project list [flags]

The list flags are:
   -branches=false: Show project branches.

Veyron Project Poll

Poll existing veyron projects and report whether any new changes exist.
Projects to poll can be specified as command line arguments.
If no projects are specified, all projects are polled by default.

Usage:
   veyron project poll <project ...>

<project ...> is a list of projects to poll.

Veyron Update

Updates all veyron projects, builds the latest version of veyron
tools, and installs the resulting binaries into $VEYRON_ROOT/bin. The
sequence in which the individual updates happen guarantees that we end
up with a consistent set of tools and source code.

The set of project and tools to update is describe by a
manifest. Veyron manifests are revisioned and stored in a "manifest"
repository, that is available locally in $VEYRON_ROOT/.manifest. The
manifest uses the following XML schema:

 <manifest>
   <imports>
     <import name="default"/>
     ...
   </imports>
   <projects>
     <project name="https://veyron.googlesource.com/veyrong.go"
              path="veyron/go/src/veyron.io/veyron"
              protocol="git"
              revision="HEAD"/>
     ...
   </projects>
   <tools>
     <tool name="veyron" package="tools/veyron"/>
     ...
   </tools>
 </manifest>

The <import> element can be used to share settings across multiple
manifests. Import names are interpreted relative to the
$VEYRON_ROOT/.manifest/v1 directory. Import cycles are not allowed and
if a project or a tool is specified multiple times, the last
specification takes effect. In particular, the elements <project
name="foo" exclude="true"/> and <tool name="bar" exclude="true"/> can
be used to exclude previously included projects and tools.

The tool identifies which manifest to use using the following
algorithm. If the $VEYRON_ROOT/.local_manifest file exists, then it is
used. Otherwise, the $VEYRON_ROOT/.manifest/v1/<manifest>.xml file is
used, which <manifest> is the value of the -manifest command-line
flag, which defaults to "default".

NOTE: Unlike the veyron tool commands, the above manifest file format
is not an API. It is an implementation and can change without notice.

Usage:
   veyron update [flags]

The update flags are:
   -gc=false: Garbage collect obsolete repositories.
   -manifest=default: Name of the project manifest.

Veyron Env

Print veyron environment variables.

If no arguments are given, prints all variables in NAME="VALUE" format,
each on a separate line ordered by name.  This format makes it easy to set
all vars by running the following bash command (or similar for other shells):
   eval $(veyron env)

If arguments are given, prints only the value of each named variable,
each on a separate line in the same order as the arguments.

Usage:
   veyron env [flags] [name ...]

[name ...] is an optional list of variable names.

The env flags are:
   -platform=: Target platform.

Veyron Run

Run an executable using the veyron environment.

Usage:
   veyron run <executable> [arg ...]

<executable> [arg ...] is the executable to run and any arguments to pass
verbatim to the executable.

Veyron Go

Wrapper around the 'go' tool that can be used for compilation of
veyron Go sources. It takes care of veyron-specific setup, such as
setting up the Go specific environment variables or making sure that
VDL generated files are regenerated before compilation.

In particular, the tool invokes the following command before invoking
any go tool commands that compile veyron Go code:

vdl generate -lang=go all

Usage:
   veyron go [flags] <arg ...>

<arg ...> is a list of arguments for the go tool.

The go flags are:
   -novdl=false: Disable automatic generation of vdl files.

Veyron Goext

Veyron extension of the go tool.

Usage:
   veyron goext <command>

The goext commands are:
   distclean   Restore the veyron Go repositories to their pristine state

Veyron Goext Distclean

Unlike the 'go clean' command, which only removes object files for
packages in the source tree, the 'goext disclean' command removes all
object files from veyron Go workspaces. This functionality is needed
to avoid accidental use of stale object files that correspond to
packages that no longer exist in the source tree.

Usage:
   veyron goext distclean

Veyron Xgo

Wrapper around the 'go' tool that can be used for cross-compilation of
veyron Go sources. It takes care of veyron-specific setup, such as
setting up the Go specific environment variables or making sure that
VDL generated files are regenerated before compilation.

In particular, the tool invokes the following command before invoking
any go tool commands that compile veyron Go code:

vdl generate -lang=go all

Usage:
   veyron xgo [flags] <platform> <arg ...>

<platform> is the cross-compilation target and has the general format
<arch><sub>-<os> or <arch><sub>-<os>-<env> where:
- <arch> is the platform architecture (e.g. 386, amd64 or arm)
- <sub> is the platform sub-architecture (e.g. v6 or v7 for arm)
- <os> is the platform operating system (e.g. linux or darwin)
- <env> is the platform environment (e.g. gnu or android)

<arg ...> is a list of arguments for the go tool."

The xgo flags are:
   -novdl=false: Disable automatic generation of vdl files.

Veyron Integration-Test

Manage integration tests.

Usage:
   veyron integration-test <command>

The integration-test commands are:
   run         Run integration tests

Veyron Integration-Test Run

Run integration tests.

Usage:
   veyron integration-test run

Veyron Version

Print version of the veyron tool.

Usage:
   veyron version

Veyron Help

Help with no args displays the usage of the parent command.
Help with args displays the usage of the specified sub-command or help topic.
"help ..." recursively displays help for all commands and topics.

Usage:
   veyron help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The help flags are:
   -style=text: The formatting style for help output, either "text" or "godoc".
*/
package main
