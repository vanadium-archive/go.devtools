// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package android

import (
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"

	"v.io/jiri/collect"
	"v.io/jiri/profiles"
	"v.io/jiri/runutil"
	"v.io/jiri/tool"
)

const (
	profileName        = "android"
	ndkDownloadBaseURL = "https://dl.google.com/android/ndk"
)

type versionSpec struct {
	ndkDownloadURL string
	// seq's chain may be already in progress.
	ndkExtract  func(seq *runutil.Sequence, src, dst string)
	ndkAPILevel int
}

func ndkArch() (string, error) {
	switch runtime.GOARCH {
	case "386":
		return "x86", nil
	case "amd64":
		return "x86_64", nil
	default:
		return "", fmt.Errorf("NDK unsupported for GOARCH %s", runtime.GOARCH)
	}
}

func tarExtract(seq *runutil.Sequence, src, dst string) {
	seq.Run("tar", "-C", dst, "-xjf", src)
}

func selfExtract(seq *runutil.Sequence, src, dst string) {
	seq.Chmod(src, profiles.DefaultDirPerm).Run(src, "-y", "-o"+dst)
}

func init() {
	arch, err := ndkArch()
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: android profile not supported: %v\n", err)
		return
	}
	m := &Manager{
		versionInfo: profiles.NewVersionInfo(profileName, map[string]interface{}{
			"3": &versionSpec{
				ndkDownloadURL: fmt.Sprintf("%s/android-ndk-r9d-%s-%s.tar.bz2", ndkDownloadBaseURL, runtime.GOOS, arch),
				ndkExtract:     tarExtract,
				ndkAPILevel:    9,
			},
			"4": &versionSpec{
				ndkDownloadURL: fmt.Sprintf("%s/android-ndk-r10e-%s-%s.bin", ndkDownloadBaseURL, runtime.GOOS, arch),
				ndkExtract:     selfExtract,
				ndkAPILevel:    16,
			},
		}, "3"),
	}
	profiles.Register(profileName, m)
}

type Manager struct {
	root        string
	androidRoot string
	versionInfo *profiles.VersionInfo
}

func (Manager) Name() string {
	return profileName
}

func (m Manager) String() string {
	return fmt.Sprintf("%s[%s]", profileName, m.versionInfo.Default())
}

func (m Manager) Root() string {
	return m.root
}

func (m *Manager) SetRoot(root string) {
	m.root = root
	m.androidRoot = filepath.Join(m.root, "profiles", "android")
}

func (m Manager) Info() string {
	return `
The android profile provides support for cross-compiling from linux or darwin
to android. It only supports one target 'arm-android' and will assume that
as the default value if one is not supplied. It installs the android NDK
and a go compiler configured to use it.`
}

func (m Manager) VersionInfo() *profiles.VersionInfo {
	return m.versionInfo
}

func (m *Manager) AddFlags(flags *flag.FlagSet, action profiles.Action) {
}

func (m *Manager) defaultTarget(ctx *tool.Context, action string, target *profiles.Target) error {
	if !target.IsSet() {
		def := *target
		target.Set("arm-android")
		fmt.Fprintf(ctx.Stdout(), "Default target %v reinterpreted as: %v\n", def, target)
	} else {
		if target.Arch() != "arm" && target.OS() != "android" {
			return fmt.Errorf("this profile can only be %v as arm-android and not as %v", action, target)
		}
	}
	return nil
}

func (m *Manager) Install(ctx *tool.Context, target profiles.Target) error {
	if err := m.defaultTarget(ctx, "installed", &target); err != nil {
		return err
	}
	var spec versionSpec
	if err := m.versionInfo.Lookup(target.Version(), &spec); err != nil {
		return err
	}
	ndkRoot, err := m.installAndroidNDK(ctx, spec)
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
	baseTarget := target
	baseTarget.SetVersion("2")
	if err := m.installAndroidBaseTarget(ctx, baseTarget); err != nil {
		return err
	}

	// Use the same variables as the go target.
	goTarget := profiles.LookupProfileTarget("go", target)
	if goTarget == nil {
		return fmt.Errorf("failed to lookup go --target=%v", target)
	}
	target.Env.Vars = goTarget.Env.Vars
	target.InstallationDir = ndkRoot
	profiles.InstallProfile(profileName, m.androidRoot)
	return profiles.UpdateProfileTarget(profileName, target)
}

func (m *Manager) Uninstall(ctx *tool.Context, target profiles.Target) error {
	if err := m.defaultTarget(ctx, "uninstalled", &target); err != nil {
		return err
	}
	target.Env.Vars = append(target.Env.Vars, "GOARM=7")
	if err := profiles.EnsureProfileTargetIsUninstalled(ctx, "base", target, m.root); err != nil {
		return err
	}
	if err := ctx.Seq().RemoveAll(m.androidRoot).Done(); err != nil {
		return err
	}
	profiles.RemoveProfileTarget(profileName, target)
	return nil
}

// installAndroidNDK installs the android NDK toolchain.
func (m *Manager) installAndroidNDK(ctx *tool.Context, spec versionSpec) (ndkRoot string, e error) {
	// Install dependencies.
	var pkgs []string
	switch runtime.GOOS {
	case "linux":
		pkgs = []string{"ant", "autoconf", "bzip2", "default-jdk", "gawk", "lib32z1", "lib32stdc++6"}
	case "darwin":
		pkgs = []string{"ant", "autoconf", "gawk"}
	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	if err := profiles.InstallPackages(ctx, pkgs); err != nil {
		return "", err
	}
	// Download Android NDK.
	ndkRoot = filepath.Join(m.androidRoot, "ndk-toolchain")
	installNdkFn := func() error {
		tmpDir, err := ctx.Seq().TempDir("", "")
		if err != nil {
			return fmt.Errorf("TempDir() failed: %v", err)
		}
		defer collect.Error(func() error { return ctx.Seq().RemoveAll(tmpDir).Done() }, &e)
		extractDir, err := ctx.Seq().TempDir(tmpDir, "extract")
		if err != nil {
			return err
		}
		local := filepath.Join(tmpDir, path.Base(spec.ndkDownloadURL))
		ctx.Seq().Run("curl", "-Lo", local, spec.ndkDownloadURL)
		spec.ndkExtract(ctx.Seq(), local, extractDir)
		files, err := ctx.Seq().ReadDir(extractDir)
		if err != nil {
			return err
		}
		if len(files) != 1 {
			return fmt.Errorf("expected one directory under %s, got: %v", extractDir, files)
		}
		ndkBin := filepath.Join(extractDir, files[0].Name(), "build", "tools", "make-standalone-toolchain.sh")
		ndkArgs := []string{ndkBin, fmt.Sprintf("--platform=android-%d", spec.ndkAPILevel), "--arch=arm", "--install-dir=" + ndkRoot}
		return ctx.Seq().Run("bash", ndkArgs...).Done()
	}
	return ndkRoot, profiles.AtomicAction(ctx, installNdkFn, ndkRoot, "Download Android NDK")
}

// installAndroidTargets installs android targets for other profiles, such
// as go, java, syncbase etc.
func (m *Manager) installAndroidBaseTarget(ctx *tool.Context, target profiles.Target) (e error) {
	env := fmt.Sprintf("ANDROID_NDK_DIR=%s,GOARM=7", filepath.Join(m.androidRoot, "ndk-toolchain"))
	androidTarget, err := profiles.NewTargetWithEnv(target.String(), env)
	if err != nil {
		return err
	}
	return profiles.EnsureProfileTargetIsInstalled(ctx, "base", androidTarget, m.root)
}
