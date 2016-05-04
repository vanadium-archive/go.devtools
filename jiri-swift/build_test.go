// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin,disabled

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"v.io/jiri"
	"v.io/jiri/tool"
)

var (
	checkExportedSymbols = []string{"swift_io_v_v23_V_nativeInitGlobal", "swift_io_v_v23_context_VContext_nativeWithCancel"}
	checkSharedTypes     = []string{"SwiftByteArray", "SwiftByteArrayArray", "GoContextHandle"}
)

func resetVars() {
	buildCgo = false
	buildFramework = false

	if sh != nil {
		sh.Cleanup()
	}
	sh = newShell()

	flagBuildMode = buildModeArchive
	flagBuildDirCgo = ""
	flagOutDirSwift = sh.MakeTempDir()
	flagReleaseMode = false
	flagTargetArch = targetArchAll

	targetArchs = []string{} // gets set by parseBuildFlags()
	parseBuildFlags()
}

func TestMain(m *testing.M) {
	flag.Parse()
	resetVars()

	// Ensure we have our necessary go profiles installed
	installProfiles()

	ret := m.Run()
	if sh != nil {
		sh.Cleanup()
	}
	os.Exit(ret)
}

func installProfiles() {
	targets := []string{"arm64-ios", "amd64-ios"}
	for _, target := range targets {
		sh.Cmd("jiri", "profile", "install", "-target="+target, "v23:base").Run()
	}
}

func initForTest(t *testing.T) *jiri.X {
	resetVars()

	// Capture JIRI_ROOT using a relative path.  We need the real JIRI_ROOT as
	// the real of jiri-swift needs the proper profiles installed and itself calls
	// out to jiri via GOSH.
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	// Sanity check that's correct
	if _, err := os.Stat(filepath.Join(root, "release", "swift")); err != nil {
		t.Fatal("Real JIRI_ROOT was not properly set: ", root)
	}

	verboseFlag := true
	jirix := &jiri.X{Context: tool.NewContext(tool.ContextOpts{Verbose: &verboseFlag}), Root: root}
	// Clean before testing
	runClean(jirix, []string{})
	return jirix
}

func TestParseBuildFlags(t *testing.T) {
	resetVars()
	// Test shared only working on amd64
	flagBuildMode = buildModeShared // should fail on all
	flagTargetArch = targetArchAll
	if err := parseBuildFlags(); err == nil {
		t.Errorf("Expected error in building all in shared mode")
	}
	flagTargetArch = targetArchArm
	if err := parseBuildFlags(); err == nil {
		t.Errorf("Expected error in building arm in shared mode")
	}
	flagTargetArch = targetArchArm64
	if err := parseBuildFlags(); err == nil {
		t.Errorf("Expected error in building arm64 in shared mode")
	}
	flagTargetArch = targetArchAmd64
	if err := parseBuildFlags(); err != nil {
		t.Errorf("Unexpected error setting to build amd64 in shared mode: %v", err)
	}

	flagBuildMode = buildModeArchive // should work on all
	flagTargetArch = targetArchAll
	if err := parseBuildFlags(); err != nil {
		t.Errorf("Unexpected error setting to build all in archive mode: %v", err)
	}
	flagTargetArch = targetArchArm
	if err := parseBuildFlags(); err == nil {
		t.Errorf("Expected 32-bit arm to fail as unsupported in archive mode: %v", err)
	}
	flagTargetArch = targetArchArm64
	if err := parseBuildFlags(); err != nil {
		t.Errorf("Unexpected error setting to build arm64 in archive mode: %v", err)
	}
	flagTargetArch = targetArchAmd64
	if err := parseBuildFlags(); err != nil {
		t.Errorf("Unexpected error setting to build amd64 in archive mode: %v", err)
	}
}

func TestParseBuildArgs(t *testing.T) {
	jirix := initForTest(t)
	// Default case -- no args
	if err := parseBuildArgs(jirix, []string{}); err != nil {
		t.Error(err)
		return
	}
	if !buildCgo || !buildFramework {
		t.Error("Default no args case didn't result in building cgo & the framework")
		return
	}
	// Cgo binary
	resetVars()
	if err := parseBuildArgs(jirix, []string{"cgo"}); err != nil {
		t.Error(err)
		return
	}
	if !buildCgo || buildFramework {
		t.Error("Should only build the cgo binary")
		return
	}
	// Cgo binary + framework
	resetVars()
	if err := parseBuildArgs(jirix, []string{"cgo", "framework"}); err != nil {
		t.Error(err)
		return
	}
	if !buildCgo || !buildFramework {
		t.Error("Should the cgo binary & framework")
		return
	}
	// Framework requires universal
	resetVars()
	flagTargetArch = targetArchAmd64
	if err := parseBuildArgs(jirix, []string{"cgo", "framework"}); err == nil {
		t.Error("Expected error building framework for 1 architecture")
		return
	}
}

func TestCgoBuildForSimulator64(t *testing.T) {
	jirix := initForTest(t)
	if err := testCgoBuildForArch(jirix, targetArchAmd64, buildModeArchive); err != nil {
		t.Error(err)
	}
	if err := testCgoBuildForArch(jirix, targetArchAmd64, buildModeShared); err != nil {
		t.Error(err)
	}
}

func TestCgoBuildForArm(t *testing.T) {
	jirix := initForTest(t)
	// Expect error for ARM currently as of Go 1.5
	if err := testCgoBuildForArch(jirix, targetArchArm, buildModeArchive); err == nil {
		t.Error("Expected error for building unsupported 32-bit arm")
	}
}

func TestCgoBuildForArm64(t *testing.T) {
	jirix := initForTest(t)
	if err := testCgoBuildForArch(jirix, targetArchArm64, buildModeArchive); err != nil {
		t.Error(err)
	}
}

func TestCgoBuildForAll(t *testing.T) {
	jirix := initForTest(t)
	if err := testCgoBuildForArch(jirix, targetArchAll, buildModeArchive); err != nil {
		t.Error(err)
	}
}

func testCgoBuildForArch(jirix *jiri.X, arch string, buildMode string) error {
	resetVars()
	buildCgo = true
	flagBuildMode = buildMode
	flagTargetArch = arch
	if err := parseBuildFlags(); err != nil {
		return err
	}
	if err := runBuildCgo(jirix); err != nil {
		return err
	}
	if err := verifyCgoBuild(jirix); err != nil {
		return err
	}
	return nil
}

func verifyCgoBuild(jirix *jiri.X) error {
	// Verify library exists
	for _, targetArch := range targetArchs {
		binaryPath, err := cgoBinaryPath(jirix, targetArch, flagBuildMode)
		if err != nil {
			return err
		}
		if !pathExists(binaryPath) {
			return fmt.Errorf("Could not find binary at %v", binaryPath)
		} else {
			// Verify library is built for iPhone only
			if err := verifyCgoBinaryForIOS(binaryPath); err != nil {
				return err
			}
			// Verify exported symbols are present
			if err := verifyCgoBinaryExports(binaryPath); err != nil {
				return err
			}
			// Verify target architecture
			verifyCgoBinaryArchOrPanic(binaryPath, targetArch)
		}
	}
	// Verify shared header exists
	if err := verifyCgoSharedHeaders(jirix); err != nil {
		return err
	}
	// Verify generated header (simple sanity check)
	if err := verifyCgoGeneratedHeader(jirix); err != nil {
		return err
	}
	return nil
}

func cgoBinaryPath(jirix *jiri.X, arch string, buildMode string) (string, error) {
	binaryPath := path.Join(getSwiftTargetDir(jirix), fmt.Sprintf("%v_%v", libraryBinaryName, arch))
	switch buildMode {
	case buildModeArchive:
		binaryPath = binaryPath + ".a"
	case buildModeShared:
		binaryPath = binaryPath + ".dylib"
	default:
		return "", fmt.Errorf("Unsupported build mode %v", buildMode)
	}
	return binaryPath, nil
}

func verifyCgoBinaryForIOS(binaryPath string) error {
	stdout := sh.Cmd("otool", "-l", binaryPath).Stdout()
	if strings.Contains(stdout, "LC_VERSION_MIN_MACOSX") {
		return fmt.Errorf("Binary contains LC_VERSION_MIN_MACOSX indicating an OS-X build binary")
	}
	if !strings.Contains(stdout, "LC_VERSION_MIN_IPHONEOS") {
		return fmt.Errorf("Binary is missing LC_VERSION_MIN_IPHONEOS so it's not clear that it's built for the iOS platform")
	}
	return nil
}

// verifyCgoBinaryExports looks at the symbols in the library to look for cgo-wrapper exported functions
func verifyCgoBinaryExports(binaryPath string) error {
	stdout := sh.Cmd("otool", "-l", binaryPath).Stdout()
	// Test a couple of key functions to make sure we're getting our exports
	for _, symbol := range checkExportedSymbols {
		if !strings.Contains(stdout, symbol) {
			fmt.Errorf("Missing %v in %v export table", symbol, binaryPath)
		}
	}
	return nil
}

func verifyCgoSharedHeaders(jirix *jiri.X) error {
	goTypesPath := path.Join(getSwiftTargetDir(jirix), "go_types.h")
	if !pathExists(goTypesPath) {
		return fmt.Errorf("Missing go_types.h at %v", goTypesPath)
	}
	bytes, err := ioutil.ReadFile(goTypesPath)
	if err != nil {
		return err
	}
	goTypes := string(bytes)
	for _, typedef := range checkSharedTypes {
		if !strings.Contains(goTypes, typedef) {
			return fmt.Errorf("Missing shared typedef of %v in %v", typedef, goTypesPath)
		}
	}
	return nil
}

func verifyCgoGeneratedHeader(jirix *jiri.X) error {
	cgoExportsPath := path.Join(getSwiftTargetDir(jirix), "cgo_exports.h")
	if !pathExists(cgoExportsPath) {
		return fmt.Errorf("Missing cgo_exports.h at %v", cgoExportsPath)
	}
	bytes, err := ioutil.ReadFile(cgoExportsPath)
	if err != nil {
		return err
	}
	cgoExports := string(bytes)
	for _, symbol := range checkExportedSymbols {
		if !strings.Contains(cgoExports, symbol) {
			return fmt.Errorf("Missing symbol %v in %v", symbol, cgoExportsPath)
		}
	}
	if !strings.Contains(cgoExports, "#ifdef __LP64__") {
		return fmt.Errorf("Missing __LP64__ guard for 32/64-bit cleaness in %v", cgoExportsPath)
	}
	if strings.Count(cgoExports, "_check_for_32_bit_pointer_matching_GoInt") != 1 {
		return fmt.Errorf("32-bit check should only occur once %v", cgoExportsPath)
	}
	if strings.Count(cgoExports, "_check_for_64_bit_pointer_matching_GoInt") != 1 {
		return fmt.Errorf("64-bit check should only occur once %v", cgoExportsPath)
	}
	s, e := strings.Index(cgoExports, "/* Start of preamble"), strings.Index(cgoExports, "/* End of preamble")
	if s == -1 || e == -1 {
		return fmt.Errorf("Missing preamble section")
	}
	for _, line := range strings.Split(cgoExports[s:e], "\n") {
		switch {
		case strings.TrimSpace(line) == "":
			continue
		case strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t"):
			return fmt.Errorf("Looks like indented code in preamble")
		}
	}
	return nil
}

func TestUniversalFrameworkBuilds(t *testing.T) {
	jirix := initForTest(t)
	flagTargetArch = targetArchAll
	if err := parseBuildFlags(); err != nil {
		t.Error(err)
		return
	}
	// Make sure VanadiumCore exports exist
	if err := runBuildCgo(jirix); err != nil {
		t.Error(err)
		return
	}
	if err := runBuildFramework(jirix); err != nil {
		t.Error(err)
		return
	}
	binaryPath := filepath.Join(flagOutDirSwift, frameworkName, frameworkBinaryName)
	if err := verifyCgoBinaryForIOS(binaryPath); err != nil {
		t.Error(err)
		return
	}
	for _, targetArch := range targetArchs {
		appleArch, _ := appleArchFromGoArch(targetArch)
		sh.Cmd("lipo", binaryPath, "-verify_arch", appleArch).Run()
		if !pathExists(filepath.Join(flagOutDirSwift, frameworkName, "Modules", frameworkBinaryName+".swiftmodule", appleArch+".swiftdoc")) {
			t.Errorf("Missing swift moduledoc for architecture %v", targetArch)
		}
		if !pathExists(filepath.Join(flagOutDirSwift, frameworkName, "Modules", frameworkBinaryName+".swiftmodule", appleArch+".swiftmodule")) {
			t.Errorf("Missing swift module for architecture %v", targetArch)
		}
	}
}
