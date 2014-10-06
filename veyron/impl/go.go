package impl

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"tools/lib/cmdline"
	"tools/lib/util"
)

// cmdGo represents the 'go' command of the veyron tool.
var cmdGo = &cmdline.Command{
	Run:   runGo,
	Name:  "go",
	Short: "Execute the go tool using the veyron environment",
	Long: `
Wrapper around the 'go' tool that can be used for compilation of
veyron Go sources. It takes care of veyron-specific setup, such as
setting up the Go specific environment variables or making sure that
VDL generated files are regenerated before compilation.

In particular, the tool invokes the following command before invoking
any go tool commands that compile veyron Go code:

vdl generate -lang=go all
`,
	ArgsName: "<arg ...>",
	ArgsLong: "<arg ...> is a list of arguments for the go tool.",
}

func runGo(command *cmdline.Command, args []string) error {
	if len(args) == 0 {
		return command.UsageErrorf("not enough arguments")
	}
	return setupAndGo(util.HostPlatform(), command, args)
}

// cmdXGo represents the 'xgo' command of the veyron tool.
var cmdXGo = &cmdline.Command{
	Run:   runXGo,
	Name:  "xgo",
	Short: "Execute the go tool using the veyron environment and cross-compilation",
	Long: `
Wrapper around the 'go' tool that can be used for cross-compilation of
veyron Go sources. It takes care of veyron-specific setup, such as
setting up the Go specific environment variables or making sure that
VDL generated files are regenerated before compilation.

In particular, the tool invokes the following command before invoking
any go tool commands that compile veyron Go code:

vdl generate -lang=go all

`,
	ArgsName: "<platform> <arg ...>",
	ArgsLong: `
<platform> is the cross-compilation target and has the general format
<arch><sub>-<os> or <arch><sub>-<os>-<env> where:
- <arch> is the platform architecture (e.g. x86, amd64 or arm)
- <sub> is the platform sub-architecture (e.g. v6 for arm)
- <os> is the platform operating system (e.g. linux or darwin)
- <env> is the platform environment (e.g. gnu or android)

<arg ...> is a list of arguments for the go tool."
`,
}

func runXGo(command *cmdline.Command, args []string) error {
	if len(args) < 2 {
		return command.UsageErrorf("not enough arguments")
	}
	platform, err := util.ParsePlatform(args[0])
	if err != nil {
		return err
	}
	return setupAndGo(platform, command, args[1:])
}

func setupAndGo(platform util.Platform, command *cmdline.Command, args []string) error {
	switch args[0] {
	case "build", "install", "run", "test":
		if err := generateVDL(); err != nil {
			return err
		}
	}
	if err := util.SetupVeyronEnvironment(platform); err != nil {
		return err
	}
	goCmd := exec.Command("go", args...)
	goCmd.Stdout = command.Stdout()
	goCmd.Stderr = command.Stderr()
	return translateExitCode(goCmd.Run())
}

func generateVDL() error {
	if novdlFlag {
		return nil
	}
	root, err := util.VeyronRoot()
	if err != nil {
		return err
	}
	vdlDir := filepath.Join(root, "veyron", "go", "src", "veyron.io", "veyron", "veyron2", "vdl", "vdl")
	args := []string{"run"}
	fis, err := ioutil.ReadDir(vdlDir)
	if err != nil {
		return fmt.Errorf("ReadDir(%v) failed: %v", vdlDir, err)
	}
	for _, fi := range fis {
		if strings.HasSuffix(fi.Name(), ".go") {
			args = append(args, filepath.Join(vdlDir, fi.Name()))
		}
	}
	// TODO(toddw): We should probably only generate vdl for the packages
	// specified for the corresponding "go" command.  This isn't trivial; we'd
	// need to grab the transitive go dependencies for the specified packages, and
	// then look for transitive vdl dependencies based on that set.
	args = append(args, "generate", "-lang=go", "all")
	vdlCmd := exec.Command("go", args...)
	conf, err := util.VeyronConfig()
	if err != nil {
		return err
	}
	gopath := []string{}
	for _, repo := range conf.GoRepos {
		gopath = append(gopath, filepath.Join(root, repo, "go"))
	}
	vdlCmd.Env = append(vdlCmd.Env, fmt.Sprintf("GOPATH=%v", strings.Join(gopath, ":")))
	if out, err := vdlCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to generate vdl: %v\n%v\n%s", err, strings.Join(vdlCmd.Args, " "), out)
	}
	return nil
}

// cmdGoExt represents the 'goext' command of the veyron tool.
var cmdGoExt = &cmdline.Command{
	Name:     "goext",
	Short:    "Veyron extensions of the go tool",
	Long:     "Veyron extension of the go tool.",
	Children: []*cmdline.Command{cmdGoExtDistClean},
}

// cmdGoExtDistClean represents the 'distclean' sub-command of 'goext'
// command of the veyron tool.
var cmdGoExtDistClean = &cmdline.Command{
	Run:   runGoExtDistClean,
	Name:  "distclean",
	Short: "Restore the veyron Go repositories to their pristine state",
	Long: `
Unlike the 'go clean' command, which only removes object files for
packages in the source tree, the 'goext disclean' command removes all
object files from veyron Go workspaces. This functionality is needed
to avoid accidental use of stale object files that correspond to
packages that no longer exist in the source tree.
`,
}

func runGoExtDistClean(command *cmdline.Command, _ []string) error {
	if err := util.SetupVeyronEnvironment(util.HostPlatform()); err != nil {
		return err
	}
	goPath := os.Getenv("GOPATH")
	failed := false
	for _, workspace := range strings.Split(goPath, ":") {
		if workspace == "" {
			continue
		}
		for _, name := range []string{"bin", "pkg"} {
			dir := filepath.Join(workspace, name)
			// TODO(jsimsa): Use the new logging library
			// for this when it is checked in.
			fmt.Fprintf(command.Stdout(), "Removing %v\n", dir)
			if err := os.RemoveAll(dir); err != nil {
				failed = true
				fmt.Fprintf(command.Stderr(), "RemoveAll(%v) failed: %v", dir, err)
			}
		}
	}
	if failed {
		return cmdline.ErrExitCode(2)
	}
	return nil
}
