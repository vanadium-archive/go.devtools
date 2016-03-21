// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package android_profile

import (
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"

	"v.io/jiri"
	"v.io/jiri/collect"
	"v.io/jiri/profiles"
	"v.io/jiri/profiles/profilesmanager"
	"v.io/jiri/profiles/profilesreader"
	"v.io/jiri/profiles/profilesutil"
	"v.io/jiri/runutil"
	"v.io/x/lib/envvar"
)

const (
	ndkDownloadBaseURL   = "https://dl.google.com/android/ndk"
	platformToolsBaseURL = "http://tools.android.com/download"
)

type versionSpec struct {
	ndkDownloadURL string
	// seq's chain may be already in progress.
	ndkExtract           func(seq runutil.Sequence, src, dst string) runutil.Sequence
	ndkAPILevel          int
	platformToolsVersion map[string]string
	baseVersion          string // Version of the base profile that this requires
}

func ndkArch(goArch string) (string, error) {
	switch goArch {
	case "386":
		return "x86", nil
	case "amd64":
		return "x86_64", nil
	case "arm":
		return "arm", nil
	default:
		return "", fmt.Errorf("NDK unsupported for GOARCH %s", goArch)
	}
}

func tarExtract(seq runutil.Sequence, src, dst string) runutil.Sequence {
	return seq.Run("tar", "-C", dst, "-xjf", src)
}

func selfExtract(seq runutil.Sequence, src, dst string) runutil.Sequence {
	return seq.Chmod(src, profilesutil.DefaultDirPerm).Run(src, "-y", "-o"+dst)
}

func Register(installer, profile string) {
	arch, err := ndkArch(runtime.GOARCH)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: android profile not supported: %v\n", err)
		return
	}
	m := &Manager{
		profileInstaller: installer,
		profileName:      profile,
		qualifiedName:    profiles.QualifiedProfileName(installer, profile),
		versionInfo: profiles.NewVersionInfo(profile, map[string]interface{}{
			"3": &versionSpec{
				ndkDownloadURL: fmt.Sprintf("%s/android-ndk-r9d-%s-%s.tar.bz2", ndkDownloadBaseURL, runtime.GOOS, arch),
				ndkExtract:     tarExtract,
				ndkAPILevel:    9,
				baseVersion:    "4",
			},
			"4": &versionSpec{
				ndkDownloadURL: fmt.Sprintf("%s/android-ndk-r10e-%s-%s.bin", ndkDownloadBaseURL, runtime.GOOS, arch),
				ndkExtract:     selfExtract,
				ndkAPILevel:    16,
				baseVersion:    "4",
			},
			"5": &versionSpec{
				ndkDownloadURL: fmt.Sprintf("%s/android-ndk-r10e-%s-%s.bin", ndkDownloadBaseURL, runtime.GOOS, arch),
				ndkExtract:     selfExtract,
				ndkAPILevel:    21,
				baseVersion:    "4",
			},
			"7": &versionSpec{
				ndkDownloadURL: fmt.Sprintf("%s/android-ndk-r10e-%s-%s.bin", ndkDownloadBaseURL, runtime.GOOS, arch),
				ndkExtract:     selfExtract,
				ndkAPILevel:    21,
				baseVersion:    "4",
			},
			"8": &versionSpec{
				ndkDownloadURL: fmt.Sprintf("%s/android-ndk-r10e-%s-%s.bin", ndkDownloadBaseURL, runtime.GOOS, arch),
				ndkExtract:     selfExtract,
				ndkAPILevel:    21,
				platformToolsVersion: map[string]string{
					"darwin": "sdk-repo-darwin-platform-tools-2219242",
					"linux":  "sdk-repo-linux-platform-tools-2219198",
				},
				baseVersion: "4",
			},
			"9": &versionSpec{
				ndkDownloadURL: fmt.Sprintf("%s/android-ndk-r10e-%s-%s.bin", ndkDownloadBaseURL, runtime.GOOS, arch),
				ndkExtract:     selfExtract,
				ndkAPILevel:    21,
				platformToolsVersion: map[string]string{
					"darwin": "sdk-repo-darwin-platform-tools-2219242",
					"linux":  "sdk-repo-linux-platform-tools-2219198",
				},
				baseVersion: "5",
			},
		}, "9"),
	}
	profilesmanager.Register(m)
}

type Manager struct {
	profileInstaller string
	profileName      string
	qualifiedName    string
	root             jiri.RelPath
	androidRoot      jiri.RelPath
	ndkRoot          jiri.RelPath
	platformRoot     jiri.RelPath
	versionInfo      *profiles.VersionInfo
	spec             versionSpec
}

func (m Manager) Name() string {
	return m.profileName
}

func (m Manager) Installer() string {
	return m.profileInstaller
}

func (m Manager) String() string {
	return fmt.Sprintf("%s[%s]", m.qualifiedName, m.versionInfo.Default())
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
		if (target.Arch() != "amd64" && target.Arch() != "arm") || target.OS() != "android" {
			return fmt.Errorf("this profile can only be %v as arm-android or amd64-android and not as %v", action, target)
		}
	}
	if err := m.versionInfo.Lookup(target.Version(), &m.spec); err != nil {
		return err
	}
	m.root = root
	m.androidRoot = root.Join("android")
	archName, err := ndkArch(target.Arch())
	if err != nil {
		return err
	}
	m.ndkRoot = m.androidRoot.Join("ndk-toolchain", fmt.Sprintf("%s-%d", archName, m.spec.ndkAPILevel))
	m.platformRoot = m.androidRoot.Join("platform-tools", m.spec.platformToolsVersion[runtime.GOOS])
	return nil
}

func (m *Manager) Install(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	var err error
	var baseEnv []string
	if err = m.initForTarget(jirix, "installed", root, &target); err != nil {
		return err
	}
	if p := pdb.LookupProfileTarget(m.profileInstaller, m.profileName, target); p != nil {
		fmt.Fprintf(jirix.Stdout(), "%v %v is already installed as %v\n", m.profileName, target, p)
		return nil
	}
	if err = m.installAndroidNDK(jirix, target); err != nil {
		return err
	}
	// Note that the ordering is important here.  Installing base depends upon the NDK
	// being installed.  Essentially there is a circular dependency between base and android.
	if baseEnv, err = m.installBase(jirix, pdb, root, target); err != nil {
		return err
	}
	if target, err = m.installAndroidPlatformTools(jirix, target); err != nil {
		return err
	}

	// Merge the target and baseProfile environments.
	env := &envvar.Vars{}
	if target.Arch() == "amd64" {
		ldflags := env.GetTokens("CGO_LDFLAGS", " ")
		ldflags = append(ldflags, m.ndkRoot.Join("x86_64-linux-android", "lib64", "libstdc++.a").Symbolic())
		env.SetTokens("CGO_LDFLAGS", ldflags, " ")
	}
	profilesreader.MergeEnv(profilesreader.ProfileMergePolicies(), env, target.Env.Vars, baseEnv)
	target.Env.Vars = env.ToSlice()
	target.InstallationDir = string(m.androidRoot)
	pdb.InstallProfile(m.profileInstaller, m.profileName, string(m.androidRoot))
	return pdb.AddProfileTarget(m.profileInstaller, m.profileName, target)
}

func (m *Manager) Uninstall(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	pdb.RemoveProfileTarget(m.profileInstaller, m.profileName, target)
	return nil
}

func (m *Manager) installBase(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) ([]string, error) {
	// It turns out that you can't install base for *-android without setting
	// at least one of these variables.
	env := fmt.Sprintf("ANDROID_NDK_DIR=%s,GOARM=7", m.ndkRoot.Symbolic())
	// target.String() only preserves the arch, opsys and version fields.
	// So this is a good way to copy the arch/opsys and we just have to set
	// the version.
	baseTarget, err := profiles.NewTarget(target.String(), env)
	baseTarget.SetVersion(m.spec.baseVersion)
	if err != nil {
		return nil, err
	}
	if err := profilesmanager.EnsureProfileTargetIsInstalled(jirix, pdb, m.profileInstaller, "base", root, baseTarget); err != nil {
		return nil, err
	}
	return pdb.EnvFromProfile(m.profileInstaller, "base", baseTarget), nil
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
	if err := profilesutil.InstallPackages(jirix, pkgs); err != nil {
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
	return profilesutil.AtomicAction(jirix, installNdkFn, m.ndkRoot.Abs(jirix), "Download Android NDK")
}

// installAndroidPlatformTools installs the android platform tools.
func (m *Manager) installAndroidPlatformTools(jirix *jiri.X, target profiles.Target) (profiles.Target, error) {
	suffix := m.spec.platformToolsVersion[runtime.GOOS]
	if suffix == "" {
		return target, nil
	}

	tmpDir, err := jirix.NewSeq().TempDir("", "")
	if err != nil {
		return target, err
	}
	defer jirix.NewSeq().RemoveAll(tmpDir)

	outDir := m.platformRoot.Abs(jirix)
	target.Env.Set("PATH=" + m.platformRoot.Symbolic())
	fn := func() error {
		androidPlatformToolsZipFile := filepath.Join(tmpDir, "platform-tools.zip")
		return jirix.NewSeq().
			Call(func() error {
				url := platformToolsBaseURL + "/" + suffix + ".zip"
				return profilesutil.Fetch(jirix, androidPlatformToolsZipFile, url)
			}, "fetch android platform tools").
			Call(func() error {
				return profilesutil.Unzip(jirix, androidPlatformToolsZipFile, tmpDir)
			}, "unzip android platform tools").
			MkdirAll(filepath.Dir(outDir), profilesutil.DefaultDirPerm).
			Rename(filepath.Join(tmpDir, "platform-tools"), outDir).
			Done()
	}
	return target, profilesutil.AtomicAction(jirix, fn, outDir, "Install Android Platform Tools")
}
