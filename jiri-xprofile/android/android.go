// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package android

import (
	"flag"
	"fmt"
	"path/filepath"
	"runtime"

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

func (m *Manager) defaultTarget(ctx *tool.Context, target *profiles.Target) error {
	if !target.IsSet() {
		def := *target
		target.Set("android=arm-android")
		fmt.Fprintf(ctx.Stdout(), "Default target %v reinterpreted as: %v\n", def, target)
	} else {
		if target.Arch != "arm" && target.OS != "android" {
			return fmt.Errorf("this profile can only be installed as arm-android")
		}
	}
	return nil
}

func (m *Manager) Install(ctx *tool.Context, target profiles.Target) error {
	target.Version = profileVersion
	if err := m.defaultTarget(ctx, &target); err != nil {
		return err
	}
	ndkRoot, err := m.installAndroidNDK(ctx, runtime.GOOS)
	if err != nil {
		return err
	}
	// Install the NDK profile so that subsequent profile installations can use it
	profiles.InstallProfile(profileName, m.androidRoot)
	target.InstallationDir = m.androidRoot
	if err := profiles.AddProfileTarget(profileName, target); err != nil {
		return err
	}

	// Install android targets for other profiles.
	if err := m.installAndroidTargets(ctx, target); err != nil {
		return err
	}

	// Use the same variables as the base target.
	baseTarget := profiles.LookupProfileTarget("base", target)
	if baseTarget == nil {
		return fmt.Errorf("failed to lookup go --target=%v", target)
	}
	target.Env.Vars = baseTarget.Env.Vars
	target.InstallationDir = ndkRoot
	profiles.InstallProfile(profileName, m.androidRoot)
	return profiles.UpdateProfileTarget(profileName, target)
}

func (m *Manager) Uninstall(ctx *tool.Context, target profiles.Target) error {
	if err := m.defaultTarget(ctx, &target); err != nil {
		return err
	}
	if err := ctx.Run().RemoveAll(m.androidRoot); err != nil {
		return err
	}
	profiles.RemoveProfileTarget(profileName, target)
	return nil
}

func (m *Manager) Update(ctx *tool.Context, target profiles.Target) error {
	if err := m.defaultTarget(ctx, &target); err != nil {
		return err
	}
	update, err := profiles.ProfileTargetNeedsUpdate(profileName, target, profileVersion)
	if err != nil {
		return err
	}
	if !update {
		return nil
	}
	return profiles.ErrNoIncrementalUpdate
}

// installAndroidNDK installs the android NDK toolchain.
func (m *Manager) installAndroidNDK(ctx *tool.Context, OS string) (ndkRoot string, e error) {
	// Install dependencies.
	var pkgs []string
	switch OS {
	case "linux":
		pkgs = []string{"ant", "autoconf", "bzip2", "default-jdk", "gawk", "lib32z1", "lib32stdc++6"}
	case "darwin":
		pkgs = []string{"ant", "autoconf", "gawk"}
	default:
		return "", fmt.Errorf("unsupported OS: %s", OS)
	}
	if err := profiles.InstallPackages(ctx, pkgs); err != nil {
		return "", err
	}

	// Download Android NDK.
	ndkRoot = filepath.Join(m.androidRoot, "ndk-toolchain")
	installNdkFn := func() error {
		tmpDir, err := ctx.Run().TempDir("", "")
		if err != nil {
			return fmt.Errorf("TempDir() failed: %v", err)
		}
		defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpDir) }, &e)
		filename := "android-ndk-r9d-" + OS + "-x86_64.tar.bz2"
		remote := "https://dl.google.com/android/ndk/" + filename
		local := filepath.Join(tmpDir, filename)
		if err := profiles.RunCommand(ctx, nil, "curl", "-Lo", local, remote); err != nil {
			return err
		}
		if err := profiles.RunCommand(ctx, nil, "tar", "-C", tmpDir, "-xjf", local); err != nil {
			return err
		}
		ndkBin := filepath.Join(tmpDir, "android-ndk-r9d", "build", "tools", "make-standalone-toolchain.sh")
		ndkArgs := []string{ndkBin, "--platform=android-9", "--install-dir=" + ndkRoot}
		if err := profiles.RunCommand(ctx, nil, "bash", ndkArgs...); err != nil {
			return err
		}
		return nil
	}
	return ndkRoot, profiles.AtomicAction(ctx, installNdkFn, ndkRoot, "Download Android NDK")
}

// installAndroidTargets installs android targets for other profiles, such
// as go, java, syncbase etc.
func (m *Manager) installAndroidTargets(ctx *tool.Context, target profiles.Target) (e error) {
	ndkRoot := filepath.Join(m.androidRoot, "ndk-toolchain")

	// Install Android Go target.
	ndkBin := filepath.Join(ndkRoot, "bin")
	ccForTarget := "CC_FOR_TARGET=" + filepath.Join(ndkBin, "arm-linux-androideabi-gcc")
	cxxForTarget := "CXX_FOR_TARGET=" + filepath.Join(ndkBin, "arm-linux-androideabi-g++")
	baseTarget := target
	baseTarget.Env.Vars = []string{"GOARM=7", "CGO_ENABLED=1", "NDK_BINDIR=" + ndkBin, ccForTarget, cxxForTarget}
	return profiles.EnsureProfileTargetIsInstalled(ctx, "base", baseTarget, m.root)
}
