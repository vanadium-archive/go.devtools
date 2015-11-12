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
	"v.io/x/lib/envvar"
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
	root, androidRoot, ndkRoot profiles.RelativePath
	versionInfo                *profiles.VersionInfo
	spec                       versionSpec
}

func (Manager) Name() string {
	return profileName
}

func (m Manager) String() string {
	return fmt.Sprintf("%s[%s]", profileName, m.versionInfo.Default())
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

func (m *Manager) initForTarget(ctx *tool.Context, action string, root profiles.RelativePath, target *profiles.Target) error {
	if !target.IsSet() {
		def := *target
		target.Set("arm-android")
		fmt.Fprintf(ctx.Stdout(), "Default target %v reinterpreted as: %v\n", def, target)
	} else {
		if target.Arch() != "arm" && target.OS() != "android" {
			return fmt.Errorf("this profile can only be %v as arm-android and not as %v", action, target)
		}
	}
	if err := m.versionInfo.Lookup(target.Version(), &m.spec); err != nil {
		return err
	}
	m.root = root
	m.androidRoot = root.Join("android")
	m.ndkRoot = m.androidRoot.Join("ndk-toolchain")
	return nil
}

func (m *Manager) Install(ctx *tool.Context, root profiles.RelativePath, target profiles.Target) error {
	if err := m.initForTarget(ctx, "installed", root, &target); err != nil {
		return err
	}
	if err := m.installAndroidNDK(ctx); err != nil {
		return err
	}
	if profiles.SchemaVersion() >= 4 {
		profiles.InstallProfile(profileName, m.androidRoot.RelativePath())
	} else {
		profiles.InstallProfile(profileName, m.androidRoot.Expand())
	}
	if err := profiles.AddProfileTarget(profileName, target); err != nil {
		return err
	}

	// Install android targets for other profiles.
	dependency := target
	dependency.SetVersion("2")
	if err := m.installAndroidBaseTargets(ctx, dependency); err != nil {
		return err
	}

	// Merge the target and baseProfile environments.
	env := envvar.VarsFromSlice(target.Env.Vars)
	baseProfileEnv := profiles.EnvFromProfile(target, "base")
	profiles.MergeEnv(profiles.ProfileMergePolicies(), env, baseProfileEnv)
	target.Env.Vars = env.ToSlice()
	if profiles.SchemaVersion() >= 4 {
		target.InstallationDir = m.ndkRoot.RelativePath()
		profiles.InstallProfile(profileName, m.androidRoot.RelativePath())
	} else {
		target.InstallationDir = m.ndkRoot.Expand()
		profiles.InstallProfile(profileName, m.androidRoot.Expand())
	}
	return profiles.UpdateProfileTarget(profileName, target)
}

func (m *Manager) Uninstall(ctx *tool.Context, root profiles.RelativePath, target profiles.Target) error {
	if err := m.initForTarget(ctx, "uninstalled", root, &target); err != nil {
		return err
	}
	target.Env.Vars = append(target.Env.Vars, "GOARM=7")
	if err := profiles.EnsureProfileTargetIsUninstalled(ctx, "base", root, target); err != nil {
		return err
	}
	if err := ctx.Seq().RemoveAll(m.androidRoot.Expand()).Done(); err != nil {
		return err
	}
	profiles.RemoveProfileTarget(profileName, target)
	return nil
}

func relPath(rp profiles.RelativePath) string {
	if profiles.SchemaVersion() >= 4 {
		return rp.String()
	}
	return rp.Expand()
}

// installAndroidNDK installs the android NDK toolchain.
func (m *Manager) installAndroidNDK(ctx *tool.Context) (e error) {
	// Install dependencies.
	var pkgs []string
	switch runtime.GOOS {
	case "linux":
		pkgs = []string{"ant", "autoconf", "bzip2", "default-jdk", "gawk", "lib32z1", "lib32stdc++6"}
	case "darwin":
		pkgs = []string{"ant", "autoconf", "gawk"}
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	if err := profiles.InstallPackages(ctx, pkgs); err != nil {
		return err
	}
	// Download Android NDK.
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
		local := filepath.Join(tmpDir, path.Base(m.spec.ndkDownloadURL))
		ctx.Seq().Run("curl", "-Lo", local, m.spec.ndkDownloadURL)
		m.spec.ndkExtract(ctx.Seq(), local, extractDir)
		files, err := ctx.Seq().ReadDir(extractDir)
		if err != nil {
			return err
		}
		if len(files) != 1 {
			return fmt.Errorf("expected one directory under %s, got: %v", extractDir, files)
		}
		ndkBin := filepath.Join(extractDir, files[0].Name(), "build", "tools", "make-standalone-toolchain.sh")
		ndkArgs := []string{ndkBin, fmt.Sprintf("--platform=android-%d", m.spec.ndkAPILevel), "--arch=arm", "--install-dir=" + m.ndkRoot.Expand()}
		return ctx.Seq().Run("bash", ndkArgs...).Done()
	}
	return profiles.AtomicAction(ctx, installNdkFn, m.ndkRoot.Expand(), "Download Android NDK")
}

// installAndroidTargets installs android targets for other profiles, currently
// just the base profile (i.e. go and syncbase.)
func (m *Manager) installAndroidBaseTargets(ctx *tool.Context, target profiles.Target) (e error) {
	env := fmt.Sprintf("ANDROID_NDK_DIR=%s,GOARM=7", relPath(m.ndkRoot))
	androidTarget, err := profiles.NewTargetWithEnv(target.String(), env)
	if err != nil {
		return err
	}
	return profiles.EnsureProfileTargetIsInstalled(ctx, "base", m.root, androidTarget)
}
