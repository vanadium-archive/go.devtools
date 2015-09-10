// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package android

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"v.io/jiri/collect"
	"v.io/jiri/profiles"
	"v.io/jiri/tool"
)

const (
	profileName    = "android"
	profileVersion = "2"
)

func init() {
	profiles.Register(profileName, &Manager{})
}

type Manager struct {
	root        string
	androidRoot string
}

func (Manager) Name() string {
	return profileName
}

func (m Manager) String() string {
	return fmt.Sprintf("%s version:%s root:%s", profileName, profileVersion, m.root)
}

func (m Manager) Root() string {
	return m.root
}

func (m *Manager) SetRoot(root string) {
	m.root = root
	m.androidRoot = filepath.Join(m.root, "profiles", "android")
}

func (m *Manager) AddFlags(flags *flag.FlagSet, action profiles.Action) {
}

func (m *Manager) Install(ctx *tool.Context, target profiles.Target) error {
	target.Version = profileVersion
	if target.CrossCompiling() {
		return fmt.Errorf("the %q profile does not support cross compilation to %v", profileName, target)
	}
	if target.OS != "linux" && target.OS != "darwin" {
		return fmt.Errorf("this profile can only be installed on linux and darwin")
	}
	if err := m.installCommon(ctx, m.root, target.OS); err != nil {
		return err
	}
	profiles.InstallProfile(profileName, m.androidRoot)
	target.InstallationDir = m.androidRoot
	return profiles.AddProfileTarget(profileName, target)
}

func (m *Manager) Uninstall(ctx *tool.Context, target profiles.Target) error {
	if err := ctx.Run().RemoveAll(m.androidRoot); err != nil {
		return err
	}
	profiles.RemoveProfileTarget(profileName, target)
	return nil
}

func (m *Manager) Update(ctx *tool.Context, target profiles.Target) error {
	return profiles.ErrNoIncrementalUpdate
}

// installCommon prepares the shared cross-platform parts of the android setup.
func (m *Manager) installCommon(ctx *tool.Context, root, OS string) (e error) {
	// Install dependencies.
	var pkgs []string
	switch OS {
	case "linux":
		pkgs = []string{"ant", "autoconf", "bzip2", "default-jdk", "gawk", "lib32z1", "lib32stdc++6"}
	case "darwin":
		pkgs = []string{"ant", "autoconf", "gawk"}
	default:
		return fmt.Errorf("unsupported OS: %s", OS)
	}
	if err := profiles.InstallPackages(ctx, pkgs); err != nil {
		return err
	}

	var sdkRoot string
	switch OS {
	case "linux":
		sdkRoot = filepath.Join(m.androidRoot, "android-sdk-linux")
	case "darwin":
		sdkRoot = filepath.Join(m.androidRoot, "android-sdk-macosx")
	default:
		return fmt.Errorf("unsupported OS: %s", OS)
	}

	// Download Android SDK.
	installSdkFn := func() error {
		if err := ctx.Run().MkdirAll(m.androidRoot, profiles.DefaultDirPerm); err != nil {
			return err
		}
		tmpDir, err := ctx.Run().TempDir("", "")
		if err != nil {
			return fmt.Errorf("TempDir() failed: %v", err)
		}
		defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpDir) }, &e)
		var filename string
		switch OS {
		case "linux":
			filename = "android-sdk_r23-linux.tgz"
		case "darwin":
			filename = "android-sdk_r23-macosx.zip"
		default:
			return fmt.Errorf("unsupported OS: %s", OS)
		}
		remote, local := "https://dl.google.com/android/"+filename, filepath.Join(tmpDir, filename)
		if err := profiles.RunCommand(ctx, "curl", []string{"-Lo", local, remote}, nil); err != nil {
			return err
		}
		switch OS {
		case "linux":
			if err := profiles.RunCommand(ctx, "tar", []string{"-C", m.androidRoot, "-xzf", local}, nil); err != nil {
				return err
			}
		case "darwin":
			if err := profiles.RunCommand(ctx, "unzip", []string{"-d", m.androidRoot, local}, nil); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported OS: %s", OS)
		}
		return nil
	}
	if err := profiles.AtomicAction(ctx, installSdkFn, sdkRoot, "Download Android SDK"); err != nil {
		return err
	}

	// Install Android SDK packagess.
	androidPkgs := []androidPkg{
		androidPkg{"Android SDK Platform-tools", filepath.Join(sdkRoot, "platform-tools")},
		androidPkg{"SDK Platform Android 4.4.2, API 19, revision 4", filepath.Join(sdkRoot, "platforms", "android-19")},
		androidPkg{"Android SDK Build-tools, revision 21.0.2", filepath.Join(sdkRoot, "build-tools")},
		androidPkg{"ARM EABI v7a System Image, Android API 19, revision 3", filepath.Join(sdkRoot, "system-images", "android-19")},
	}
	for _, pkg := range androidPkgs {
		if err := installAndroidPkg(ctx, sdkRoot, pkg); err != nil {
			return err
		}
	}

	// Update Android SDK tools.
	toolPkg := androidPkg{"Android SDK Tools", ""}
	if err := installAndroidPkg(ctx, sdkRoot, toolPkg); err != nil {
		return err
	}

	// Download Android NDK.
	ndkRoot := filepath.Join(m.androidRoot, "ndk-toolchain")
	installNdkFn := func() error {
		tmpDir, err := ctx.Run().TempDir("", "")
		if err != nil {
			return fmt.Errorf("TempDir() failed: %v", err)
		}
		defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpDir) }, &e)
		filename := "android-ndk-r9d-" + OS + "-x86_64.tar.bz2"
		remote := "https://dl.google.com/android/ndk/" + filename
		local := filepath.Join(tmpDir, filename)
		if err := profiles.RunCommand(ctx, "curl", []string{"-Lo", local, remote}, nil); err != nil {
			return err
		}
		if err := profiles.RunCommand(ctx, "tar", []string{"-C", tmpDir, "-xjf", local}, nil); err != nil {
			return err
		}
		ndkBin := filepath.Join(tmpDir, "android-ndk-r9d", "build", "tools", "make-standalone-toolchain.sh")
		ndkArgs := []string{ndkBin, "--platform=android-9", "--install-dir=" + ndkRoot}
		if err := profiles.RunCommand(ctx, "bash", ndkArgs, nil); err != nil {
			return err
		}
		return nil
	}
	if err := profiles.AtomicAction(ctx, installNdkFn, ndkRoot, "Download Android NDK"); err != nil {
		return err
	}

	// Install Android Go.
	goProfileMgr := profiles.LookupManager("go")
	if goProfileMgr == nil {
		return fmt.Errorf("no profile available to install go")
	}
	goProfileMgr.SetRoot(root)
	goTarget := profiles.Target{
		Tag:  "android",
		Arch: "arm",
		OS:   "android",
	}
	// Equivalent to:
	// install --target=android=arm-android -env GOARM=7,CGO_ENABLED_7,CC_FOR_TARGET=<ndkRoot>/bin/arm-linux-androidabi-gcc
	ccForTarget := "CC_FOR_TARGET=" + filepath.Join(ndkRoot, "bin", "arm-linux-androideabi-gcc")
	goTarget.Env.Vars = []string{"GOARM=7", "CGO_ENABLED=1", ccForTarget}
	if ctx.Run().Opts().Verbose || ctx.Run().Opts().DryRun {
		fmt.Fprintf(ctx.Stdout(), "install --target=%s -env=%s go\n", "android=arm-android", strings.Join(goTarget.Env.Vars, ","))
	}
	if err := goProfileMgr.Install(ctx, goTarget); err != nil {
		return err
	}
	goBinDir := filepath.Join(m.androidRoot, "go", "bin")
	if !profiles.DirectoryExists(ctx, goBinDir) {
		if err := ctx.Run().MkdirAll(goBinDir, profiles.DefaultDirPerm); err != nil {
			return err
		}
	}
	profile := profiles.LookupProfileTarget("go", profiles.Target{Tag: "android"})
	gocmd := filepath.Join(goBinDir, "go")
	ctx.Run().Remove(gocmd)
	return ctx.Run().Symlink(filepath.Join(profile.InstallationDir, "bin", "go"), gocmd)
}

type androidPkg struct {
	name      string
	directory string
}

func installAndroidPkg(ctx *tool.Context, sdkRoot string, pkg androidPkg) error {
	installPkgFn := func() error {
		// Identify all indexes that match the given package.
		var out bytes.Buffer
		androidBin := filepath.Join(sdkRoot, "tools", "android")
		androidArgs := []string{"list", "sdk", "--all", "--no-https"}
		opts := ctx.Run().Opts()
		opts.Stdout = &out
		opts.Stderr = &out
		if err := ctx.Run().CommandWithOpts(opts, androidBin, androidArgs...); err != nil {
			return err
		}
		scanner, indexes := bufio.NewScanner(&out), []int{}
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Index(line, pkg.name) != -1 {
				// The output of "android list sdk --all" looks as follows:
				//
				// Packages available for installation or update: 118
				//    1- Android SDK Tools, revision 23.0.5
				//    2- Android SDK Platform-tools, revision 21
				//    3- Android SDK Build-tools, revision 21.1
				// ...
				//
				// The following logic gets the package index.
				index, err := strconv.Atoi(strings.TrimSpace(line[:4]))
				if err != nil {
					return fmt.Errorf("%v", err)
				}
				indexes = append(indexes, index)
			}
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("Scan() failed: %v", err)
		}
		switch {
		case len(indexes) == 0:
			return fmt.Errorf("no package matching %q found", pkg.name)
		case len(indexes) > 1:
			return fmt.Errorf("multiple packages matching %q found", pkg.name)
		}

		// Install the target package.
		androidArgs = []string{"update", "sdk", "--no-ui", "--all", "--no-https", "--filter", fmt.Sprintf("%d", indexes[0])}
		var stdin, stdout bytes.Buffer
		stdin.WriteString("y") // pasing "y" to accept Android's license agreement
		opts = ctx.Run().Opts()
		opts.Stdin = &stdin
		opts.Stdout = &stdout
		opts.Stderr = &stdout
		err := ctx.Run().CommandWithOpts(opts, androidBin, androidArgs...)
		if err != nil || tool.VerboseFlag {
			fmt.Fprintf(ctx.Stdout(), out.String())
		}
		return err
	}
	return profiles.AtomicAction(ctx, installPkgFn, pkg.directory, fmt.Sprintf("Install %s", pkg.name))
}
