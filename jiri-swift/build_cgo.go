// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"v.io/jiri"
)

const singleHeaderTmpl = `/* Created by jiri-swift - DO NOT EDIT. */

/* Start of preamble from import "C" comments.  */

{{.Includes}}
#import "go_types.h"

// These sizes (including C struct memory alignment/padding) isn't available from Go, so we make that available via CGo.
{{.Typedefs}}

/* End of preamble from import "C" comments.  */

/* Start of boilerplate cgo prologue.  */

{{.Prologue}}

/* End of boilerplate cgo prologue.  */

#ifdef __cplusplus
extern "C"
#endif

{{.Exports}}

#ifdef __cplusplus
}
#endif
`

func runBuildCgo(jirix *jiri.X) error {
	if flagBuildDirCgo == "" {
		flagBuildDirCgo = sh.MakeTempDir()
	}
	sh.Pushd(flagBuildDirCgo)

	for _, targetArch := range targetArchs {
		cleanOldCompiledFiles(jirix, targetArch)
		compileCgo(jirix, targetArch)
		installCgoBinary(jirix, targetArch)
	}

	copyCommonHeaders(jirix)

	// Grab either the main arch we're building for or just the first -- we just need to make sure
	// it's one that will have headers generated for it.
	return generateSingleHeader(jirix, targetArchs[0])
}

func cleanOldCompiledFiles(jirix *jiri.X, targetArch string) {
	d := filepath.Join(jirix.Root, "release/go/pkg", "darwin_"+targetArch, "v.io")
	if !pathExists(d) {
		verbose(jirix, "Previously built go binaries & headers directory doesn't exist, nothing to remove: %v\n", d)
		return
	}
	sanityCheckDir(d)
	verbose(jirix, "Removing compiled go files and headers in path %v\n", d)
	if err := os.RemoveAll(d); err != nil {
		panic(fmt.Sprint("Unable to remove old compiled files:", err))
	}
}

func compileCgo(jirix *jiri.X, targetArch string) {
	targetFlag := targetArch + "-ios"
	verbose(jirix, "Building for project %v target %v with build mode %v in dir %v\n", selectedProject.name, targetFlag, flagBuildMode, flagBuildDirCgo)
	// Create the binary
	bp := buildBinaryPath(targetArch)
	verbose(jirix, "Running jiri go -target %v build -buildmode=%v -tags ios -o %v %v\n", targetFlag, flagBuildMode, bp, selectedProject.mainPackage)
	sh.Cmd("jiri", "go", "-target", targetFlag, "build", "-buildmode="+flagBuildMode, "-tags", "ios", "-o", bp, selectedProject.mainPackage).Run()
	// If the package is simple enough it'll also generate a header -- we'll use the installed
	// headers instead (as its more universal), so we can delete this generated header now if
	// it exists.
	b := strings.TrimSuffix(bp, filepath.Ext(bp))
	os.RemoveAll(b + ".h")
	// Now make sure the headers are created/generated in our go/pkg directory for a later step.
	verbose(jirix, "Running jiri go -target %v install -buildmode=%v -tags ios %v\n", targetFlag, flagBuildMode, selectedProject.mainPackage)
	sh.Cmd("jiri", "go", "-target", targetFlag, "install", "-buildmode="+flagBuildMode, "-tags", "ios", selectedProject.mainPackage).Run()
}

func buildBinaryPath(targetArch string) string {
	bn := path.Join(flagBuildDirCgo, selectedProject.libraryBinaryName+"_"+targetArch)
	switch flagBuildMode {
	case buildModeArchive:
		return bn + ".a"
	case buildModeShared:
		return bn + ".dylib"
	default:
		panic("Unknown build mode")
	}
}

func installCgoBinary(jirix *jiri.X, targetArch string) {
	// Install it to the Swift target directory
	swiftTargetDir := getSwiftTargetDir(jirix)
	sh.Cmd("mkdir", "-p", swiftTargetDir).Run()

	var destLibPath string
	switch flagBuildMode {
	case buildModeArchive:
		a := fmt.Sprintf("%v_%v.a", selectedProject.libraryBinaryName, targetArch)
		destLibPath = path.Join(swiftTargetDir, a)
		sh.Cmd("mv", buildBinaryPath(targetArch), destLibPath).Run()
	case buildModeShared:
		dylib := fmt.Sprintf("%v_%v.dylib", selectedProject.libraryBinaryName, targetArch)
		destLibPath = path.Join(swiftTargetDir, dylib)
		sh.Cmd("mv", buildBinaryPath(targetArch), destLibPath).Run()
		sh.Cmd("install_name_tool", "-id", "@loader_path/"+dylib, destLibPath).Run()
	}
	verbose(jirix, "Installed binary at %v\n", destLibPath)
	verifyCgoBinaryArchOrPanic(destLibPath, targetArch)
}

func copyCommonHeaders(jirix *jiri.X) {
	verbose(jirix, "Copying common shared headers between Swift and Go\n")
	// Take types.h and make it into go_types.h
	sh.Cmd("cp", path.Join(jirix.Root, selectedProject.commonHeaderPath), path.Join(getSwiftTargetDir(jirix), "go_types.h")).Run()
}

func generateSingleHeader(jirix *jiri.X, targetArch string) error {
	verbose(jirix, "Generating header for Swift\n")
	// Load and parse all the headers
	generatedHeadersDir := fmt.Sprintf("%v/release/go/pkg/darwin_%v/%v", jirix.Root, targetArch, selectedProject.exportedHeadersPackageRoot)
	generatedHeadersPaths := findHeadersUnderPath(generatedHeadersDir)
	hdrs := cgoHeaders{}
	for _, file := range generatedHeadersPaths {
		hdr, err := newCgoHeader(jirix, file)
		if err != nil {
			return err
		}
		hdrs = append(hdrs, hdr)
	}
	// Generate the header
	data := struct {
		Includes string
		Typedefs string
		Prologue string
		Exports  string
	}{
		Includes: strings.Join(hdrs.includes(), "\n"),
		Typedefs: strings.Join(hdrs.typedefs(), "\n"),
		Prologue: strings.Join(hdrs[0].prologue, "\n"), // Grab the first -- it is sufficient and complete.
		Exports:  strings.Join(hdrs.exports(), "\n"),
	}
	tmpl := template.Must(template.New("singleCgoHeader").Parse(singleHeaderTmpl))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return err
	}
	// Write it to disk
	combinedHdrPath := path.Join(getSwiftTargetDir(jirix), "cgo_exports.h")
	verbose(jirix, "Writing generated merged header to %v\n", combinedHdrPath)
	// Remove the old file if it exists
	if err := os.RemoveAll(combinedHdrPath); err != nil {
		return err
	}
	f, err := os.Create(combinedHdrPath)
	if err != nil {
		return err
	}
	if _, err := buf.WriteTo(f); err != nil {
		return err
	}
	return nil
}
