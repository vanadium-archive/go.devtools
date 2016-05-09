// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	"v.io/jiri"
	"v.io/x/lib/gosh"
)

var memoizedRegex map[string]*regexp.Regexp

func init() {
	memoizedRegex = make(map[string]*regexp.Regexp)
}

func appleArchFromGoArch(goArch string) (appleArch string, err error) {
	switch goArch {
	case targetArchArm:
		appleArch = "armv7"
	case targetArchArm64:
		appleArch = "arm64"
	case targetArchAmd64:
		appleArch = "x86_64"
	default:
		return "", fmt.Errorf("Unsupported architecture: %v", goArch)
	}
	return appleArch, nil
}

// extractFromRegex returns the first matched capture group in line for a given regex.
func extractFromRegex(line string, regex string) string {
	if re, ok := memoizedRegex[regex]; ok {
		// Grab the
		return re.FindStringSubmatch(line)[1]
	}
	memoizedRegex[regex] = regexp.MustCompile(regex)
	return extractFromRegex(line, regex)
}

func getSwiftTargetDir(jirix *jiri.X) string {
	return path.Join(jirix.Root, "release", "swift", selectedProject.directoryName, "Generated")
}

func findHeadersUnderPath(path string) []string {
	output := sh.Cmd("find", path, "-name", "*.h", "-type", "f").Stdout()
	paths := strings.Split(strings.TrimSpace(output), "\n")
	sort.Strings(paths) // Maintain consistent order and do package roots first
	return paths
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func newShell() *gosh.Shell {
	newSh := gosh.NewShell(nil)
	// Remove any inherited JIRI env flags as they'll conflict potentially with jiri calls for our given
	// target down the line.
	goEnvs := []string{
		"CGO_CFLAGS",
		"CGO_CXXFLAGS",
		"CGO_LDFLAGS",
		"GOARCH",
		"GOOS",
		"GOROOT",
		"GOROOT_BOOTSTRAP",
		"GOPATH",
	}
	for _, goEnv := range goEnvs {
		delete(newSh.Vars, goEnv)
	}
	return newSh
}

func sanityCheckDir(dir string) {
	// Sanity check before rm -r
	if dir == "" || (strings.HasPrefix(dir, "/") && len(dir) < len("/tmp")) {
		panic(fmt.Sprintf("Aborting because %v may be malformed", dir))
	}
}

func verbose(jirix *jiri.X, format string, args ...interface{}) {
	if jirix.Verbose() {
		fmt.Fprintf(jirix.Stdout(), format, args...)
	}
}

func verifyCgoBinaryArchOrPanic(binaryPath string, goArch string) {
	appleArch, err := appleArchFromGoArch(goArch)
	if err != nil {
		panic(fmt.Sprintf("%v", err))
	}
	// Will exit 1 if the binary does not contain the arch, and GOSH will panic
	sh.Cmd("lipo", binaryPath, "-verify_arch", appleArch).Run()
}
