// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go -env=CMDLINE_PREFIX=jiri . -help

package main

import (
	"fmt"
	"runtime"
	"strings"

	"v.io/jiri"
	"v.io/jiri/tool"
	"v.io/x/lib/cmdline"
	"v.io/x/lib/gosh"
)

var (
	buildCgo       bool
	buildFramework bool

	sh *gosh.Shell

	flagBuildMode   string
	flagBuildDirCgo string
	flagOutDirSwift string
	flagProject     string
	flagReleaseMode bool
	flagTargetArch  string

	targetArchs []string

	selectedProject *project
	projects        = []project{
		{
			name:                       "VanadiumCore",
			commonHeaderPath:           "release/go/src/v.io/x/swift/types.h",
			description:                "Core bindings from Swift to Vanadium; incompatible with SyncbaseCore",
			directoryName:              "VanadiumCore",
			exportedHeadersPackageRoot: "v.io/x",
			frameworkName:              "VanadiumCore.framework",
			frameworkBinaryName:        "VanadiumCore",
			libraryBinaryName:          "v23",
			mainPackage:                "v.io/x/swift/main",
			testCheckExportedSymbols:   []string{"swift_io_v_v23_V_nativeInitGlobal", "swift_io_v_v23_context_VContext_nativeWithCancel"},
			testCheckSharedTypes:       []string{"SwiftByteArray", "SwiftByteArrayArray", "GoContextHandle"},
		},
		{
			name:                       "SyncbaseCore",
			commonHeaderPath:           "release/go/src/v.io/x/ref/services/syncbase/bridge/cgo/lib.h",
			description:                "Core bindings from Swift to Syncbase; incompatible with VanadiumCore",
			directoryName:              "SyncbaseCore",
			exportedHeadersPackageRoot: "v.io/x",
			frameworkName:              "SyncbaseCore.framework",
			frameworkBinaryName:        "SyncbaseCore",
			jiriProfiles:               []string{"v23:syncbase"},
			libraryBinaryName:          "sbcore",
			mainPackage:                "v.io/x/ref/services/syncbase/bridge/cgo",
			testCheckExportedSymbols:   []string{"v23_syncbase_Init", "v23_syncbase_DbLeaveSyncgroup", "v23_syncbase_RowDelete"},
			testCheckSharedTypes:       []string{"v23_syncbase_String", "v23_syncbase_Bytes", "v23_syncbase_Strings", "v23_syncbase_VError"},
		},
	}
)

type project struct {
	name                       string
	commonHeaderPath           string
	description                string
	directoryName              string
	exportedHeadersPackageRoot string
	frameworkName              string
	frameworkBinaryName        string
	jiriProfiles               []string
	libraryBinaryName          string
	mainPackage                string
	testCheckExportedSymbols   []string
	testCheckSharedTypes       []string
}

const (
	// darwin/386 is not a supported configuration for Go 1.5.1
	// TODO(zinman): Support mac target instead of pure iOS
	//	targetArch386 = "386"
	targetArchAmd64 = "amd64"
	targetArchArm   = "arm"
	targetArchArm64 = "arm64"
	targetArchAll   = "all"

	buildModeArchive = "c-archive"
	buildModeShared  = "c-shared"

	stageBuildCgo       = "cgo"
	stageBuildFramework = "framework"

	descBuildMode   = "The build mode for cgo, either c-archive or c-shared. Defaults to c-archive."
	descBuildDirCgo = "The directory for all generated artifacts during the cgo building phase. Defaults to a temp dir."
	descOutDirSwift = "The directory for the generated Swift framework."
	descProject     = "Selects which project to build (VanadiumCore, SyncbaseCore). Must be set."
	descReleaseMode = "If set xcode is built in release mode. Defaults to false, which is debug mode."
	descTargetArch  = "The architecture you wish to build for (arm, arm64, amd64), or 'all'. Defaults to amd64."
)

func init() {
	tool.InitializeRunFlags(&cmdRoot.Flags)
	cmdBuild.Flags.StringVar(&flagBuildMode, "build-mode", buildModeArchive, descBuildMode)
	cmdBuild.Flags.StringVar(&flagBuildDirCgo, "build-dir-cgo", "", descBuildDirCgo)
	cmdBuild.Flags.StringVar(&flagOutDirSwift, "out-dir-swift", "", descOutDirSwift)
	cmdBuild.Flags.StringVar(&flagProject, "project", "", descProject)
	cmdBuild.Flags.BoolVar(&flagReleaseMode, "release-mode", false, descReleaseMode)
	cmdBuild.Flags.StringVar(&flagTargetArch, "target", targetArchAmd64, descTargetArch)
}

func main() {
	sh = newShell()
	defer sh.Cleanup()

	cmdline.Main(cmdRoot)
}

// cmdRun represents the "jiri run" command.
var cmdRoot = &cmdline.Command{
	Name:     "swift",
	Short:    "Compile Swift frameworks and apps",
	Long:     "Manages the build pipeline for the Swift framework/app, from CGO bindings to fattening the binaries.",
	Children: []*cmdline.Command{cmdBuild, cmdClean},
}

var cmdBuild = &cmdline.Command{
	Runner: jiri.RunnerFunc(runBuild),
	Name:   "build",
	Short:  "Builds and installs the cgo wrapper, as well as the Swift framework/app",
	Long: `The complete build pipeline from creating the CGO library, manipulating the headers for Swift,
	and building the Swift framework/app using Xcode for the selected project.`,
	ArgsName: "[stage ...] (cgo, framework)",
	ArgsLong: `
	[stage ...] are the pipelines stage to run and any arguments to pass to that stage. If left empty defaults
	to building all stages. Project must be set.

	Available stages:
		cgo: Builds and installs the cgo library
		framework: Builds a Swift framework using Xcode
	`,
}

var cmdClean = &cmdline.Command{
	Runner: jiri.RunnerFunc(runClean),
	Name:   "clean",
	Short:  "Removes generated cgo binaries and headers",
	Long:   "Removes generated cgo binaries and headers that fall under $JIRI_ROOT/release/swift/$PROJECT/Generated",
}

func parseProjectFlag() error {
	for _, p := range projects {
		if strings.ToLower(flagProject) == strings.ToLower(p.name) {
			selectedProject = &p
			return nil
		}
	}
	names := []string{}
	for _, p := range projects {
		names = append(names, p.name)
	}
	return fmt.Errorf("You must set a project -- one of the following (case-insensitive): %v", names)
}

func parseBuildFlags() error {
	// Validate build modes
	switch flagBuildMode {
	case buildModeArchive, buildModeShared:
		break
	default:
		return fmt.Errorf("Invalid build mode (%v)", flagBuildMode)
	}
	// Validate build mode + architecture
	switch flagTargetArch {
	case targetArchAmd64:
		break
	case targetArchArm:
		return fmt.Errorf(
			"32-bit ARM is currently unsupported as Go is unable to generate PIC code (as of 1.5); See https://github.com/golang/go/issues/12681")
	case targetArchAll, targetArchArm64:
		if flagBuildMode != buildModeArchive {
			return fmt.Errorf("Invalid build mode %v for ARM architecture (only archive is supported by Go 1.5)", flagBuildMode)
		}
	default:
		return fmt.Errorf("Unsupported target architecture (%v). Must pass %v, %v or %v",
			flagTargetArch,
			targetArchArm64,
			targetArchAmd64,
			targetArchAll)
	}
	// Parse architectures
	targetArchs = []string{flagTargetArch}
	if flagTargetArch == targetArchAll {
		targetArchs = []string{targetArchArm64, targetArchAmd64}
	}
	return nil
}

func parseBuildArgs(jirix *jiri.X, args []string) error {
	// Defaults to all
	if len(args) == 0 {
		verbose(jirix, "No stages specified: building cgo and the swift framework\n")
		buildCgo = true
		buildFramework = true
	}
	// Turn on a stage for each argument
	for _, arg := range args {
		switch arg {
		case stageBuildCgo:
			buildCgo = true
		case stageBuildFramework:
			buildFramework = true
		default:
			return fmt.Errorf("Invalid build stage: %v", arg)
		}
	}
	// Follow up on dependencies
	if buildFramework {
		if !buildCgo {
			verbose(jirix, "Turning on building cgo as it's a dependency of the framework\n")
			buildCgo = true // Dependency of framework... for now always build.
		}
		if flagOutDirSwift == "" {
			return fmt.Errorf("-out-dir-swift must be defined if building the framework")
		}
		if flagTargetArch != targetArchAll {
			return fmt.Errorf("Framework builds are always universal -- target must be all")
		}
	}
	return nil
}

func runClean(jirix *jiri.X, args []string) error {
	if err := parseProjectFlag(); err != nil {
		return err
	}
	swiftTargetDir := getSwiftTargetDir(jirix)
	if pathExists(swiftTargetDir) {
		sanityCheckDir(swiftTargetDir)
		verbose(jirix, "Removing generated swift library path %v\n", swiftTargetDir)
		sh.Cmd("rm", "-r", swiftTargetDir).Run()
	}
	return nil
}

func runBuild(jirix *jiri.X, args []string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("Only darwin is currently supported")
	}
	if err := parseProjectFlag(); err != nil {
		return err
	}
	if err := parseBuildFlags(); err != nil {
		return err
	}
	if err := parseBuildArgs(jirix, args); err != nil {
		return err
	}
	if buildCgo {
		if err := runBuildCgo(jirix); err != nil {
			return err
		}
	}
	if buildFramework {
		return runBuildFramework(jirix)
	}
	return nil
}
