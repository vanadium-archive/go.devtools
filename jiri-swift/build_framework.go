// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"v.io/jiri"
)

func runBuildFramework(jirix *jiri.X) error {
	if flagTargetArch != targetArchAll {
		// The target itself requires cgo binaries of each architecture and thus xcode
		// will fail otherwise.
		return fmt.Errorf("Framework builds are always universal -- target must be all")
	}
	xcodeTarget := frameworkBinaryName
	sh.Pushd(filepath.Join(jirix.Root, "release/swift/lib"))
	// Make sure target directory exists
	sh.Cmd("mkdir", "-p", flagOutDirSwift).Run()
	// Clear out the old framework
	sanityCheckDir(flagOutDirSwift)
	targetPath := filepath.Join(flagOutDirSwift, frameworkName)
	verbose(jirix, "Building framework at %v\n", targetPath)
	if pathExists(targetPath) {
		verbose(jirix, "Removing old framework at %v\n", targetPath)
		if err := os.RemoveAll(targetPath); err != nil {
			return err
		}
	}
	return buildUniversalFramework(jirix, xcodeTarget)
}

func buildUniversalFramework(jirix *jiri.X, xcodeTarget string) error {
	fatBinaryPath := filepath.Join(flagOutDirSwift, frameworkName, frameworkBinaryName)
	didCopyFramework := false
	for _, targetArch := range targetArchs {
		buildDir, err := buildSingleFramework(jirix, xcodeTarget, targetArch)
		if err != nil {
			return err
		}
		builtFrameworkPath := filepath.Join(buildDir, frameworkName)
		if !didCopyFramework {
			verbose(jirix, "Copying framework from %v to %v\n", builtFrameworkPath, flagOutDirSwift)
			sh.Cmd("cp", "-r", builtFrameworkPath, flagOutDirSwift).Run()
			didCopyFramework = true
			continue
		}
		// Copy this architecture's swift modules
		builtModulesPath := filepath.Join(builtFrameworkPath, "Modules", frameworkBinaryName+".swiftmodule")
		targetModulesPath := filepath.Join(flagOutDirSwift, frameworkName, "Modules", frameworkBinaryName+".swiftmodule")
		verbose(jirix, "Copying built modules from %v to %v\n", filepath.Join(builtModulesPath, "*"), targetModulesPath)
		modules, err := filepath.Glob(filepath.Join(builtModulesPath, "*"))
		if err != nil {
			return err
		}
		for _, module := range modules {
			sh.Cmd("cp", "-f", module, targetModulesPath).Run()
		}
		// Inject the architecture binary
		sh.Cmd("lipo", fatBinaryPath, filepath.Join(builtFrameworkPath, frameworkBinaryName), "-create", "-output", fatBinaryPath).Run()
	}
	return nil
}

func buildSingleFramework(jirix *jiri.X, xcodeTarget string, targetArch string) (string, error) {
	buildDir := sh.MakeTempDir()
	appleArch, err := appleArchFromGoArch(targetArch)
	if err != nil {
		return "", err
	}
	configuration := "Release"
	if !flagReleaseMode {
		configuration = "Debug"
	}
	sdk := "iphoneos"
	if targetArch == targetArchAmd64 {
		sdk = "iphonesimulator"
	}
	if err := os.RemoveAll("build"); err != nil {
		return "", nil
	}
	verbose(jirix, "Building target %v for architecture %v in %v configuration\n", xcodeTarget, appleArch, configuration)
	cmd := sh.Cmd("xcodebuild", "-target", xcodeTarget, "-arch", appleArch, "-configuration", configuration, "-sdk", sdk, "CONFIGURATION_BUILD_DIR="+buildDir, "VALID_ARCHS="+appleArch)
	cmd.PropagateOutput = true
	cmd.Run()
	if err := os.RemoveAll("build"); err != nil {
		return "", nil
	}
	return buildDir, nil
}
