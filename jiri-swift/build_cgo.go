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
	"time"

	"v.io/jiri"
	"v.io/jiri/profiles"
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

// installSuffix is passed to "go build" to keep different package files from
// concurrent builds from stomping on one another.
var installSuffix = fmt.Sprintf("swift_cgo_%d", time.Now().UnixNano())

func runBuildCgo(jirix *jiri.X) error {
	// Delete all artifacts after building, since the installSuffix prevents
	// them from being used as a cache anyways.
	defer func() {
		for _, targetArch := range targetArchs {
			cleanOldCompiledFiles(jirix, targetArch)
		}
	}()

	// Copy over dependent libraries.
	if flagBuildDirCgo == "" {
		flagBuildDirCgo = sh.MakeTempDir()
	}
	sh.Pushd(flagBuildDirCgo)

	for _, targetArch := range targetArchs {
		compileCgo(jirix, targetArch)
		installCgoBinary(jirix, targetArch)
		if err := copyLinkedLibraries(jirix, targetArch); err != nil {
			return err
		}
	}

	copyCommonHeaders(jirix)

	// Grab either the main arch we're building for or just the first -- we just need to make sure
	// it's one that will have headers generated for it.
	return generateSingleHeader(jirix, targetArchs[0])
}

func cleanOldCompiledFiles(jirix *jiri.X, targetArch string) {
	pattern := filepath.Join(jirix.Root, "release", "go", "pkg", fmt.Sprintf("darwin_%s_swift_cgo_*", targetArch))
	dirs, err := filepath.Glob(pattern)
	if err != nil {
		panic(fmt.Errorf("filepath.Glob(%s) failed: %v", pattern, err))
	}
	for _, d := range dirs {
		sanityCheckDir(d)
		verbose(jirix, "Removing compiled go files and headers in path %v\n", d)
		if err := os.RemoveAll(d); err != nil {
			panic(fmt.Sprint("Unable to remove old compiled files:", err))
		}
	}
}

func compileCgo(jirix *jiri.X, targetArch string) {
	targetFlag := targetArch + "-ios"
	verbose(jirix, "Building for project %v target %v with build mode %v in dir %v\n", selectedProject.name, targetFlag, flagBuildMode, flagBuildDirCgo)
	// Create the binary
	bp := buildBinaryPath(targetArch)
	binArgs := []string{"go", "-target", targetFlag, "build", "-installsuffix", installSuffix, "-buildmode", flagBuildMode, "-tags", "ios", "-o", bp, selectedProject.mainPackage}
	verbose(jirix, "Running jiri %s\n", strings.Join(binArgs, " "))
	sh.Cmd("jiri", binArgs...).Run()
	// If the package is simple enough it'll also generate a header -- we'll use the installed
	// headers instead (as its more universal), so we can delete this generated header now if
	// it exists.
	b := strings.TrimSuffix(bp, filepath.Ext(bp))
	os.RemoveAll(b + ".h")
	// Now make sure the headers are created/generated in our go/pkg directory for a later step.
	headerArgs := []string{"go", "-target", targetFlag, "install", "-installsuffix", installSuffix, "-buildmode", flagBuildMode, "-tags", "ios", selectedProject.mainPackage}
	verbose(jirix, "Running jiri %s\n", strings.Join(headerArgs, " "))
	sh.Cmd("jiri", headerArgs...).Run()
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

// copyLinkedLibraries will look at the project-specific profile requirements (like v23:syncbase) to find
// any static libraries in the profile that Go might have linked to via CGO_LDFLAGS, and then copy these
// static archives to the target directory. This allows Xcode to be able to directly link to a local copy
// of these files as CGO doesn't statically link the libraries. While it might seem like a bug, it's
// actually a feature: it allows us to distribute a version of a framework without potentially-conflicting
// dependencies like LevelDB should the end-user wish to provide their own copy (or already has another
// library that has statically-linked it).
func copyLinkedLibraries(jirix *jiri.X, targetArch string) error {
	if len(selectedProject.jiriProfiles) == 0 {
		// No files to copy over
		verbose(jirix, "No jiri profiles associated with project; not copying any linked static libs\n")
		return nil
	}
	// Load jiri profiles database
	db := profiles.NewDB()
	if err := db.Read(jirix, jirix.ProfilesDBDir()); err != nil {
		return fmt.Errorf("failed to read profiles db at path %v: %v", jirix.ProfilesDBDir(), err)
	}
	// Copy any profile's static libraries over
	for _, pn := range selectedProject.jiriProfiles {
		// Get profile
		splitPn := strings.Split(pn, ":")
		if len(splitPn) != 2 {
			return fmt.Errorf("did not understand jiri profile %v -- expected format is <installer>:<name>", pn)
		}
		p := db.LookupProfile(splitPn[0], splitPn[1])
		if p == nil {
			return fmt.Errorf("unable to find profile %v", pn)
		}
		// Find target for this architecture & os
		var target *profiles.Target
		for _, t := range p.Targets() {
			if t.Arch() != targetArch {
				continue
			}
			if t.OS() != "ios" {
				continue
			}
			target = t
		}
		if target == nil {
			return fmt.Errorf("couldn't find target arch %v in targets %v for profile %v", targetArch, p.Targets(), pn)
		}
		copyLinkedLibrariesForTarget(jirix, target)
	}
	return nil
}

// copyLinkedLibrariesForTarget copies any static libraries included on the
// CGO_LDFLAGS to our target directory to make it easy to link to (or
// distribute as its own library) in Xcode.
func copyLinkedLibrariesForTarget(jirix *jiri.X, target *profiles.Target) {
	libs := findStaticLibsInDirs(findLibDirsInTargetEnv(jirix, target))
	for _, l := range libs {
		// Convert path to dst/libname_arch.a
		bn := filepath.Base(l)
		bn = strings.Trim(bn, filepath.Ext(bn))
		dst := filepath.Join(getSwiftTargetDir(jirix), fmt.Sprintf("%v_%v.a", bn, target.Arch()))
		verbose(jirix, "Copying %v to %v\n", l, dst)
		sh.Cmd("cp", l, dst).Run()
	}
}

// findLibDirsInTargetEnv parses a profile target's CGO_LDFLAGS for any included
// library dirs, intersects them with the target's installation dir (to make sure
// we only get our own locally-built and not system-wide libraries), and returns
// these absolute paths. For example it will return the directories associated
// with target architecture's compiled static archives of LevelDB and Snappy
// when searching the v23:syncbase profile target.
func findLibDirsInTargetEnv(jirix *jiri.X, t *profiles.Target) []string {
	var dirs []string
	for _, v := range t.Env.Vars {
		if !strings.HasPrefix(v, "CGO_LDFLAGS") {
			continue
		}
		dirs = strings.Split(v, "-L")
		break
	}
	if len(dirs) == 0 {
		return dirs
	}
	var paths []string
	for _, d := range dirs {
		i := strings.Index(d, t.InstallationDir)
		if i == -1 {
			continue
		}
		d = filepath.Join(jirix.Root, strings.TrimSpace(d[i:]))
		paths = append(paths, d)
	}
	return paths
}

// findStaticLibsInDirs walks a slice of directories searching for static archives
// (files that end with .a), and returns those as a string slice.
func findStaticLibsInDirs(dirs []string) []string {
	var libs []string
	for _, d := range dirs {
		filepath.Walk(d, func(path string, f os.FileInfo, err error) error {
			if strings.HasSuffix(path, ".a") {
				libs = append(libs, path)
			}
			return nil
		})
	}
	return libs
}

func copyCommonHeaders(jirix *jiri.X) {
	verbose(jirix, "Copying common shared headers between Swift and Go\n")
	// Take types.h and make it into go_types.h
	sh.Cmd("cp", path.Join(jirix.Root, selectedProject.commonHeaderPath), path.Join(getSwiftTargetDir(jirix), "go_types.h")).Run()
}

func generateSingleHeader(jirix *jiri.X, targetArch string) error {
	verbose(jirix, "Generating header for Swift\n")
	// Load and parse all the headers
	generatedHeadersDir := fmt.Sprintf("%v/release/go/pkg/darwin_%v_%v/%v", jirix.Root, targetArch, installSuffix, selectedProject.exportedHeadersPackageRoot)
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
