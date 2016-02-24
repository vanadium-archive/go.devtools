// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"strings"
	"text/template"

	"v.io/jiri/jiri"
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
		compileCgo(jirix, targetArch)
		installCgoBinary(jirix, targetArch)
	}

	copyCommonHeaders(jirix)

	// Grab either the main arch we're building for or just the first -- we just need to make sure
	// it's one that will have headers generated for it.
	return generateSingleHeader(jirix, targetArchs[0])
}

func compileCgo(jirix *jiri.X, targetArch string) {
	targetFlag := targetArch + "-ios"
	verbose(jirix, "Building for target %v with build mode %v in dir %v\n", targetFlag, flagBuildMode, flagBuildDirCgo)
	sh.Cmd("jiri", "go", "-target", targetFlag, "build", "-buildmode="+flagBuildMode, "-tags", "ios", "v.io/x/swift/main").Run()
	sh.Cmd("jiri", "go", "-target", targetFlag, "install", "-buildmode="+flagBuildMode, "-tags", "ios", "v.io/x/swift/main").Run()
}

func installCgoBinary(jirix *jiri.X, targetArch string) {
	// Install it to the Swift target directory
	swiftTargetDir := getSwiftTargetDir(jirix)
	sh.Cmd("mkdir", "-p", swiftTargetDir).Run()

	var destLibPath string
	switch flagBuildMode {
	case buildModeArchive:
		a := fmt.Sprintf("v23_%v.a", targetArch)
		destLibPath = path.Join(swiftTargetDir, a)
		sh.Cmd("mv", "main.a", destLibPath).Run()
	case buildModeShared:
		dylib := fmt.Sprintf("v23_%v.dylib", targetArch)
		destLibPath = path.Join(swiftTargetDir, dylib)
		sh.Cmd("mv", "main", dylib).Run()
		sh.Cmd("install_name_tool", "-id", "@loader_path/"+dylib, dylib).Run()
		sh.Cmd("mv", dylib, destLibPath).Run()
	}
	verbose(jirix, "Installed binary at %v\n", destLibPath)
	verifyCgoBinaryArchOrPanic(destLibPath, targetArch)
}

func copyCommonHeaders(jirix *jiri.X) {
	verbose(jirix, "Copying common shared headers between Swift and Go\n")
	// Take types.h and make it into go_types.h
	sh.Cmd("cp", path.Join(jirix.Root, "release/go/src/v.io/x/swift/types.h"), path.Join(getSwiftTargetDir(jirix), "go_types.h")).Run()
}

func generateSingleHeader(jirix *jiri.X, targetArch string) error {
	verbose(jirix, "Generating header for Swift\n")
	// Load and parse all the headers
	generatedHeadersDir := fmt.Sprintf("%v/release/go/pkg/darwin_%v/v.io/x", jirix.Root, targetArch)
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
