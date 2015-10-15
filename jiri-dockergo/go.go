// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go -env=CMDLINE_PREFIX=jiri . -help

package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	"v.io/jiri/profiles"
	"v.io/jiri/project"
	"v.io/jiri/runutil"
	"v.io/jiri/tool"
	"v.io/jiri/util"
	"v.io/x/devtools/internal/golib"
	"v.io/x/devtools/jiri-v23-profile/v23_profile"
	"v.io/x/lib/cmdline"
)

var cmd = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runGo),
	Name:   "dockergo",
	Short:  "Execute the go command in a docker container",
	Long: `
Executes a Go command in a docker container. This is primarily aimed at the
builds of Linux binaries and libraries where there is a dependence on cgo. This
allows for compilation (and cross-compilation) without polluting the host
filesystem with compilers, C-headers, libraries etc. as dependencies are
encapsulated in the docker image.

The docker image is expected to have the appropriate C-compiler
and any pre-built headers/libraries to be linked in.  It is also
expected to have the appropriate environment variables (such as CGO_ENABLED,
CGO_CFLAGS etc) set.

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
`,
	ArgsName: "<arg ...>",
	ArgsLong: "<arg ...> is a list of arguments for the go tool.",
}

var (
	imageFlag                  string
	manifestFlag, profilesFlag string
	profilesModeFlag           profiles.ProfilesMode
	targetFlag                 profiles.Target
	extraLDFlags               string
)

const dockerBin = "docker"

func init() {
	tool.InitializeRunFlags(&cmd.Flags)
	profiles.RegisterProfileFlags(&cmd.Flags, &profilesModeFlag, &manifestFlag, &profilesFlag, v23_profile.DefaultManifestFilename, &targetFlag)
	flag.StringVar(&imageFlag, "image", "", "Name of the docker image to use. If empty, the tool will automatically select an image based on the environment variables, possibly edited by the profile")
	flag.StringVar(&extraLDFlags, "extra-ldflags", "", golib.ExtraLDFlagsFlagDescription)
}

func runGo(cmdlineEnv *cmdline.Env, args []string) error {
	if len(args) == 0 {
		return cmdlineEnv.UsageErrorf("not enough arguments")
	}
	ctx := tool.NewContextFromEnv(cmdlineEnv)
	ch, err := profiles.NewConfigHelper(ctx, profilesModeFlag, manifestFlag)
	if err != nil {
		return err
	}
	ch.SetGoPath()
	ch.SetVDLPath()
	envMap := ch.ToMap()
	// docker can only be used to build linux binaries
	if os, exists := envMap["GOOS"]; exists && os != "linux" {
		return fmt.Errorf("Only GOOS=linux is supported, not %q", os)
	}
	envMap["GOOS"] = "linux"
	// default to the local architecture
	if _, exists := envMap["GOARCH"]; !exists {
		envMap["GOARCH"] = runtime.GOARCH
	}
	if _, err := runutil.LookPath(dockerBin, envMap); err != nil {
		return fmt.Errorf("%v not found in path: %v", dockerBin, err)
	}
	img, err := image(envMap)
	if err != nil {
		return err
	}
	if args, err = golib.PrepareGo(ctx, envMap, args, extraLDFlags); err != nil {
		return err
	}
	if ctx.Verbose() {
		fmt.Fprintf(ctx.Stderr(), "Using docker image: %q\n", img)
	}
	// Run the go tool.
	return runDockerGo(ctx, img, envMap, args)
}

// image returns the name of the docker image to use, or the empty string if
// a containerized build environment should not be used.
func image(env map[string]string) (string, error) {
	if image := imageFlag; len(image) > 0 {
		return image, nil
	}
	switch goarch := env["GOARCH"]; goarch {
	case "arm", "armhf":
		return "asimshankar/testing:armhf", nil
	case "amd64":
		return "asimshankar/testing:amd64", nil
	default:
		return "", fmt.Errorf("unable to auto select docker image to use for GOARCH=%q", goarch)
	}
}

// runDockerGo runs the "go" command on this node via docker, using the
// provided docker image.
//
// Note that many Go-compiler related environment variables (CGO_ENABLED,
// CGO_CXXFLAGS etc.) will be ignored, as those are expected to be set in the
// docker container.
// TODO(ashasnkar): CGO_CFLAGS, CGO_LDFLAGS and CGO_CXXFLAGS should be checked
// and possibly used?
//
// In order to make users of "jiri-dockergo" feel like they are using a local
// build environment (i.e., making the use of docker transparent) a few things
// need to be done:
//   (a) All directores in GOPATH on the host need to be mounted in the container.
//       These are mounted under /jiri/gopath<index>
//       $JIRI_ROOT is mounted specially under /jiri/root
//   (b) The current working directory needs to be mounted in the container
//       (so that relative paths work).
//       Mounted under /jiri/workdir
//   (c) On a Linux host (where docker runs natively, not inside a virtual machine),
//       the -u flag needs to be provided to "docker run" to ensure that files written out
//       to the host match the UID and GID of the filesystem as it would if the build was
//       run on the host.
//
// All this is best effort - there are still command invocations that will not
// work via docker.
// For example: jiri-go build -o /tmp/foo .
// will succeed but the output will not appear in /tmp/foo on the host (it will
// be written to /tmp/foo on the container and then vanish when the container
// dies).
// TODO(ashankar): This and other similar cases can be dealt with by inspecting "args"
// and handling such cases, but that is left as a future excercise.
func runDockerGo(ctx *tool.Context, image string, env map[string]string, args []string) error {
	hostJiriroot, err := project.JiriRoot()
	if err != nil {
		return err
	}
	var (
		volumeroot  = "/jiri" // All volumes are mounted inside here
		jiriroot    = fmt.Sprintf("%v/root", volumeroot)
		gopath      []string
		ctr         int
		workdir     string
		hostWorkdir = env["PWD"]
		dockerargs  = []string{"run", "-v", fmt.Sprintf("%v:%v", hostJiriroot, jiriroot)}
	)
	if strings.HasPrefix(hostWorkdir, hostJiriroot) {
		workdir = strings.Replace(hostWorkdir, hostJiriroot, jiriroot, 1)
	}
	for _, p := range strings.Split(env["GOPATH"], ":") {
		if strings.HasPrefix(p, hostJiriroot) {
			gopath = append(gopath, strings.Replace(p, hostJiriroot, jiriroot, 1))
			continue
		}
		// A non $JIRI_ROOT entry in the GOPATH, include that in the volumes.
		cdir := fmt.Sprintf("%v/gopath%d", volumeroot, ctr)
		ctr++
		dockerargs = append(dockerargs, "-v", fmt.Sprintf("%v:%v", p, cdir))
		gopath = append(gopath, cdir)
		if strings.HasPrefix(hostWorkdir, p) {
			workdir = strings.Replace(hostWorkdir, p, cdir, 1)
		}
	}
	if len(workdir) == 0 {
		// Working directory on host is outside the directores in GOPATH.
		workdir = fmt.Sprintf("%v/workdir", volumeroot)
		dockerargs = append(dockerargs, "-v", fmt.Sprintf("%v:%v", hostWorkdir, workdir))
	}
	// Figure out the uid/gid to run the container with so that files
	// written out to the host filesystem have the right owner/group.
	if gid, ok := fileGid(hostJiriroot); ok {
		dockerargs = append(dockerargs, "-u", fmt.Sprintf("%d:%d", os.Getuid(), gid))
	}
	dockerargs = append(dockerargs,
		"-e", fmt.Sprintf("GOPATH=%v", strings.Join(gopath, ":")),
		"-w", workdir,
		image,
		"go")
	dockerargs = append(dockerargs, args...)
	return util.TranslateExitCode(ctx.Run().Command(dockerBin, dockerargs...))
}

func main() {
	cmdline.Main(cmd)
}
