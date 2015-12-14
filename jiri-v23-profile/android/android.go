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
	"v.io/jiri/jiri"
	"v.io/jiri/profiles"
	"v.io/jiri/runutil"
	"v.io/x/lib/envvar"
)

const (
	profileName        = "android"
	ndkDownloadBaseURL = "https://dl.google.com/android/ndk"
)

type versionSpec struct {
	ndkDownloadURL string
	// seq's chain may be already in progress.
	ndkExtract  func(seq *runutil.Sequence, src, dst string) *runutil.Sequence
	ndkAPILevel int
}

func ndkArch(goArch string) (string, error) {
	switch goArch {
	case "386":
		return "x86", nil
	case "amd64":
		return "x86_64", nil
	default:
		return "", fmt.Errorf("NDK unsupported for GOARCH %s", goArch)
	}
}

func tarExtract(seq *runutil.Sequence, src, dst string) *runutil.Sequence {
	return seq.Run("tar", "-C", dst, "-xjf", src)
}

func selfExtract(seq *runutil.Sequence, src, dst string) *runutil.Sequence {
	return seq.Chmod(src, profiles.DefaultDirPerm).Run(src, "-y", "-o"+dst)
}

func init() {
	arch, err := ndkArch(runtime.GOARCH)
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
			"5": &versionSpec{
				ndkDownloadURL: fmt.Sprintf("%s/android-ndk-r10e-%s-%s.bin", ndkDownloadBaseURL, runtime.GOOS, arch),
				ndkExtract:     selfExtract,
				ndkAPILevel:    21,
			},
		}, "3"),
	}
	profiles.Register(profileName, m)
}

type Manager struct {
	root, androidRoot, ndkRoot jiri.RelPath
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

func (m *Manager) initForTarget(jirix *jiri.X, action string, root jiri.RelPath, target *profiles.Target) error {
	if !target.IsSet() {
		def := *target
		target.Set("arm-android")
		fmt.Fprintf(jirix.Stdout(), "Default target %v reinterpreted as: %v\n", def, target)
	} else {
		if target.Arch() != "amd64" && target.Arch() != "arm" && target.OS() != "android" {
			return fmt.Errorf("this profile can only be %v as arm-android or amd64-android and not as %v", action, target)
		}
	}
	if err := m.versionInfo.Lookup(target.Version(), &m.spec); err != nil {
		return err
	}
	m.root = root
	m.androidRoot = root.Join("android")
	m.ndkRoot = m.androidRoot.Join(fmt.Sprintf("ndk-toolchain-%s", target.Arch()))
	return nil
}

func (m *Manager) Install(jirix *jiri.X, root jiri.RelPath, target profiles.Target) error {
	if err := m.initForTarget(jirix, "installed", root, &target); err != nil {
		return err
	}
	if p := profiles.LookupProfileTarget(profileName, target); p != nil {
		fmt.Fprintf(jirix.Stdout(), "%v %v is already installed as %v\n", profileName, target, p)
		return nil
	}
	if err := m.installAndroidNDK(jirix, target); err != nil {
		return err
	}
	profiles.InstallProfile(profileName, string(m.androidRoot))
	if err := profiles.AddProfileTarget(profileName, target); err != nil {
		return err
	}

	// Install android targets for other profiles.
	dependency := target
	dependency.SetVersion("4")
	if err := m.installAndroidBaseTargets(jirix, dependency); err != nil {
		return err
	}

	// Merge the target and baseProfile environments.
	env := envvar.VarsFromSlice(target.Env.Vars)

	if target.Arch() == "amd64" {
		ldflags := env.GetTokens("CGO_LDFLAGS", " ")
		ldflags = append(ldflags, m.ndkRoot.Join("x86_64-linux-android", "lib64", "libstdc++.a").Symbolic())
		env.SetTokens("CGO_LDFLAGS", ldflags, " ")
	}

	baseProfileEnv := profiles.EnvFromProfile(dependency, "base")
	profiles.MergeEnv(profiles.ProfileMergePolicies(), env, baseProfileEnv)
	target.Env.Vars = env.ToSlice()
	target.InstallationDir = string(m.ndkRoot)
	profiles.InstallProfile(profileName, string(m.androidRoot))
	return profiles.UpdateProfileTarget(profileName, target)
}

func (m *Manager) Uninstall(jirix *jiri.X, root jiri.RelPath, target profiles.Target) error {
	if err := m.initForTarget(jirix, "uninstalled", root, &target); err != nil {
		return err
	}
	target.Env.Vars = append(target.Env.Vars, "GOARM=7")
	if err := profiles.EnsureProfileTargetIsUninstalled(jirix, "base", root, target); err != nil {
		return err
	}
	if err := jirix.NewSeq().RemoveAll(m.androidRoot.Abs(jirix)).Done(); err != nil {
		return err
	}
	profiles.RemoveProfileTarget(profileName, target)
	return nil
}

// installAndroidNDK installs the android NDK toolchain.
func (m *Manager) installAndroidNDK(jirix *jiri.X, target profiles.Target) (e error) {
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
	if err := profiles.InstallPackages(jirix, pkgs); err != nil {
		return err
	}
	// Download Android NDK.
	installNdkFn := func() error {
		s := jirix.NewSeq()
		tmpDir, err := s.TempDir("", "")
		if err != nil {
			return fmt.Errorf("TempDir() failed: %v", err)
		}
		defer collect.Error(func() error { return jirix.NewSeq().RemoveAll(tmpDir).Done() }, &e)
		extractDir, err := s.TempDir(tmpDir, "extract")
		if err != nil {
			return err
		}
		local := filepath.Join(tmpDir, path.Base(m.spec.ndkDownloadURL))
		s.Run("curl", "-Lo", local, m.spec.ndkDownloadURL)
		files, err := m.spec.ndkExtract(s, local, extractDir).ReadDir(extractDir)
		if err != nil {
			return err
		}
		if len(files) != 1 {
			return fmt.Errorf("expected one directory under %s, got: %v", extractDir, files)
		}
		ndkBin := filepath.Join(extractDir, files[0].Name(), "build", "tools", "make-standalone-toolchain.sh")
		archOption, err := ndkArch(target.Arch())
		if err != nil {
			return err
		}
		ndkArgs := []string{ndkBin, fmt.Sprintf("--platform=android-%d", m.spec.ndkAPILevel), fmt.Sprintf("--arch=%s", archOption), "--install-dir=" + m.ndkRoot.Abs(jirix)}
		return s.Last("bash", ndkArgs...)
	}
	return profiles.AtomicAction(jirix, installNdkFn, m.ndkRoot.Abs(jirix), "Download Android NDK")
}

// installAndroidTargets installs android targets for other profiles, currently
// just the base profile (i.e. go and syncbase.)
func (m *Manager) installAndroidBaseTargets(jirix *jiri.X, target profiles.Target) (e error) {
	env := fmt.Sprintf("ANDROID_NDK_DIR=%s,GOARM=7", m.ndkRoot.Symbolic())
	androidTarget, err := profiles.NewTargetWithEnv(target.String(), env)
	if err != nil {
		return err
	}
	return profiles.EnsureProfileTargetIsInstalled(jirix, "base", m.root, androidTarget)
}
