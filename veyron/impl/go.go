package impl

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"tools/lib/cmdline"
	"tools/lib/envutil"
	"tools/lib/runutil"
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
	return runGoForPlatform(util.HostPlatform(), command, args)
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
- <arch> is the platform architecture (e.g. 386, amd64 or arm)
- <sub> is the platform sub-architecture (e.g. v6 or v7 for arm)
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
	return runGoForPlatform(platform, command, args[1:])
}

func runGoForPlatform(platform util.Platform, command *cmdline.Command, args []string) error {
	// Generate vdl files, if necessary.
	switch args[0] {
	case "build", "generate", "install", "run", "test":
		if err := generateVDL(args); err != nil {
			return err
		}
	}
	// Run the go tool for the given platform.
	targetEnv, err := util.VeyronEnvironment(platform)
	if err != nil {
		return err
	}
	goCmd := exec.Command(targetGo, args...)
	goCmd.Stdout = command.Stdout()
	goCmd.Stderr = command.Stderr()
	goCmd.Env = targetEnv.Slice()
	return translateExitCode(goCmd.Run())
}

func generateVDL(cmdArgs []string) error {
	if novdlFlag {
		return nil
	}
	hostEnv, err := util.VeyronEnvironment(util.HostPlatform())
	if err != nil {
		return err
	}
	root, err := util.VeyronRoot()
	if err != nil {
		return err
	}
	// Initialize vdlGenArgs with the *.go files under the vdl directory to run.
	vdlGenArgs := []string{"run"}
	vdlDir := filepath.Join(root, "veyron", "go", "src", "veyron.io", "veyron", "veyron2", "vdl", "vdl")
	fis, err := ioutil.ReadDir(vdlDir)
	if err != nil {
		return fmt.Errorf("ReadDir(%v) failed: %v", vdlDir, err)
	}
	for _, fi := range fis {
		if strings.HasSuffix(fi.Name(), ".go") {
			vdlGenArgs = append(vdlGenArgs, filepath.Join(vdlDir, fi.Name()))
		}
	}
	// Generate VDL for the transitive Go package dependencies.
	//
	// Note that the vdl tool takes VDL packages as input, but we're supplying Go
	// packages.  We're assuming the package paths for the VDL packages we want to
	// generate have the same path names as the Go package paths.  Some of the Go
	// package paths may not correspond to a valid VDL package, so we provide the
	// -ignore_unknown flag to silently ignore these paths.
	//
	// It's fine if the VDL packages have dependencies not reflected in the Go
	// packages; the vdl tool will compute the transitive closure of VDL package
	// dependencies, as usual.
	//
	// TODO(toddw): Change the vdl tool to return vdl packages given the full Go
	// dependencies, after vdl config files are implemented.
	goPkgs, goFiles := extractGoPackagesOrFiles(cmdArgs[0], cmdArgs[1:])
	goDeps, err := computeGoDeps(hostEnv, append(goPkgs, goFiles...))
	if err != nil {
		return err
	}
	vdlGenArgs = append(vdlGenArgs, "-ignore_unknown", "generate", "-lang=go")
	vdlGenArgs = append(vdlGenArgs, goDeps...)
	vdlGenCmd := exec.Command(hostGo, vdlGenArgs...)
	vdlGenCmd.Env = hostEnv.Slice()
	if out, err := vdlGenCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to generate vdl: %v\n%v\n%s", err, strings.Join(vdlGenCmd.Args, " "), out)
	}
	return nil
}

// extractGoPackagesOrFiles is given the cmd and args for the go tool, filters
// out flags, and returns the PACKAGES or GOFILES that were specified in args.
// Note that all commands that accept PACKAGES also accept GOFILES.
//
//   go build    [build flags]              [-o out]      [PACKAGES]
//   go generate                            [-run regexp] [PACKAGES]
//   go install  [build flags]                            [PACKAGES]
//   go run      [build flags]              [-exec prog]  [GOFILES]  [run args]
//   go test     [build flags] [test flags] [-exec prog]  [PACKAGES] [testbin flags]
//
// Sadly there's no way to do this syntactically.  It's easy for single token
// -flag and -flag=x, but non-boolean flags may be two tokens "-flag x".
//
// We keep track of all non-boolean flags F, and skip every token that starts
// with - or --, and also skip the next token if the flag is in F and isn't of
// the form -flag=x.  If we forget to update F, we'll still handle the -flag and
// -flag=x cases correctly, but we'll get "-flag x" wrong.
func extractGoPackagesOrFiles(cmd string, args []string) ([]string, []string) {
	var nonBool map[string]bool
	switch cmd {
	case "build":
		nonBool = nonBoolGoBuild
	case "generate":
		nonBool = nonBoolGoGenerate
	case "install":
		nonBool = nonBoolGoInstall
	case "run":
		nonBool = nonBoolGoRun
	case "test":
		nonBool = nonBoolGoTest
	}
	// Move start to the start of PACKAGES or GOFILES, by skipping flags.
	start := 0
	for start < len(args) {
		// Handle special-case terminator --
		if args[start] == "--" {
			start++
			break
		}
		match := goFlagRE.FindStringSubmatch(args[start])
		if match == nil {
			break
		}
		// Skip this flag, and maybe skip the next token for the "-flag x" case.
		//   match[1] is the flag name
		//   match[2] is the optional "=" for the -flag=x case
		start++
		if nonBool[match[1]] && match[2] == "" {
			start++
		}
	}
	// Move end to the end of PACKAGES or GOFILES.
	var end int
	switch cmd {
	case "test":
		// Any arg starting with - is a testbin flag.
		// https://golang.org/cmd/go/#hdr-Test_packages
		for end = start; end < len(args); end++ {
			if strings.HasPrefix(args[end], "-") {
				break
			}
		}
	case "run":
		// Go run takes gofiles, which are defined as a file ending in ".go".
		// https://golang.org/cmd/go/#hdr-Compile_and_run_Go_program
		for end = start; end < len(args); end++ {
			if !strings.HasSuffix(args[end], ".go") {
				break
			}
		}
	default:
		end = len(args)
	}
	// Decide whether these are packages or files.
	switch {
	case start == end:
		return nil, nil
	case (start < len(args) && strings.HasSuffix(args[start], ".go")):
		return nil, args[start:end]
	default:
		return args[start:end], nil
	}
}

var (
	goFlagRE     = regexp.MustCompile(`^--?([^=]+)(=?)`)
	nonBoolBuild = []string{
		"p", "ccflags", "compiler", "gccgoflags", "gcflags", "installsuffix", "ldflags", "tags",
	}
	nonBoolTest = []string{
		"bench", "benchtime", "blockprofile", "blockprofilerate", "covermode", "coverpkg", "coverprofile", "cpu", "cpuprofile", "memprofile", "memprofilerate", "outputdir", "parallel", "run", "timeout",
	}
	nonBoolGoBuild    = makeStringSet(append(nonBoolBuild, "o"))
	nonBoolGoGenerate = makeStringSet([]string{"run"})
	nonBoolGoInstall  = makeStringSet(nonBoolBuild)
	nonBoolGoRun      = makeStringSet(append(nonBoolBuild, "exec"))
	nonBoolGoTest     = makeStringSet(append(append(nonBoolBuild, nonBoolTest...), "exec"))
)

func makeStringSet(values []string) map[string]bool {
	ret := make(map[string]bool)
	for _, v := range values {
		ret[v] = true
	}
	return ret
}

// computeGoDeps computes the transitive Go package dependencies for the given
// set of pkgs.  The strategy is to run "go list <pkgs>" with a special format
// string that dumps the specified pkgs and all deps as space / newline
// separated tokens.  The pkgs may be in any format recognized by "go list"; dir
// paths, import paths, or go files.
func computeGoDeps(env *envutil.Snapshot, pkgs []string) ([]string, error) {
	var stderr bytes.Buffer
	goListArgs := []string{`list`, `-f`, `{{.ImportPath}} {{join .Deps " "}}`}
	goListArgs = append(goListArgs, pkgs...)
	goListCmd := exec.Command(hostGo, goListArgs...)
	goListCmd.Stderr = &stderr
	goListCmd.Env = env.Slice()
	makeErr := func(phase string, err error) error {
		return fmt.Errorf("failed to compute go deps (%s): %v\n%v\n%s", phase, err, strings.Join(goListCmd.Args, " "), stderr.String())
	}
	depsPipe, err := goListCmd.StdoutPipe()
	if err != nil {
		return nil, makeErr("stdout pipe", err)
	}
	if err := goListCmd.Start(); err != nil {
		return nil, makeErr("start", err)
	}
	scanner := bufio.NewScanner(depsPipe)
	scanner.Split(bufio.ScanWords)
	depsMap := make(map[string]bool)
	for scanner.Scan() {
		depsMap[scanner.Text()] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, makeErr("scan", err)
	}
	if err := goListCmd.Wait(); err != nil {
		return nil, makeErr("wait", err)
	}
	var deps []string
	for dep, _ := range depsMap {
		// Filter out bad packages:
		//   command-line-arguments is the dummy import path for "go run".
		switch dep {
		case "command-line-arguments":
			continue
		}
		deps = append(deps, dep)
	}
	return deps, nil
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
	ctx := util.NewContext(verboseFlag, command.Stdout(), command.Stderr())
	env, err := util.VeyronEnvironment(util.HostPlatform())
	if err != nil {
		return err
	}
	failed := false
	for _, workspace := range env.GetTokens("GOPATH", ":") {
		for _, name := range []string{"bin", "pkg"} {
			dir := filepath.Join(workspace, name)
			// TODO(jsimsa): Use the new logging library
			// for this when it is checked in.
			if err := ctx.Run().Function(runutil.RemoveAll(dir)); err != nil {
				failed = true
			}
		}
	}
	if failed {
		return cmdline.ErrExitCode(2)
	}
	return nil
}
