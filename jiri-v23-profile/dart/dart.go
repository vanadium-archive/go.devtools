// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// package dart_profile implements the dart profile.
package dart_profile

import (
	"flag"
	"fmt"
	"path/filepath"

	"v.io/jiri/jiri"
	"v.io/jiri/profiles"
	"v.io/x/lib/envvar"
)

const (
	profileName = "dart"
)

type versionSpec struct {
	// Version of Dart SDK to install.
	// See https://www.dartlang.org/downloads/archive/ and
	// https://www.dartlang.org/downloads/archive/#direct-download-urls
	dartSdkVersion string

	// Channel for the Dart SDK.  Must be "stable" or "dev".
	dartSdkChannel string
}

func init() {
	m := &Manager{
		versionInfo: profiles.NewVersionInfo(profileName, map[string]interface{}{
			"1.14.0-dev.1.0": &versionSpec{
				dartSdkChannel: "dev",
				dartSdkVersion: "1.14.0-dev.1.0",
			},
		}, "1.14.0-dev.1.0"),
	}
	profiles.Register(profileName, m)
}

type Manager struct {
	root, dartRoot, dartInstDir profiles.RelativePath
	versionInfo                 *profiles.VersionInfo
	spec                        versionSpec
}

func (m Manager) Name() string {
	return profileName
}

func (m Manager) String() string {
	return fmt.Sprintf("%s[%s]", profileName, m.versionInfo.Default())
}

func (m Manager) VersionInfo() *profiles.VersionInfo {
	return m.versionInfo
}

func (m Manager) Info() string {
	return `Installs the Dart SDK.`
}

func (m *Manager) AddFlags(flags *flag.FlagSet, action profiles.Action) {
}

func (m *Manager) initForTarget(jirix *jiri.X, root profiles.RelativePath, target *profiles.Target) error {
	if err := m.versionInfo.Lookup(target.Version(), &m.spec); err != nil {
		return err
	}
	m.dartRoot = root.Join("dart")
	m.dartInstDir = m.dartRoot.Join(target.OS(), m.spec.dartSdkChannel, m.spec.dartSdkVersion)
	return nil
}

func (m *Manager) Install(jirix *jiri.X, root profiles.RelativePath, target profiles.Target) error {
	if err := m.initForTarget(jirix, root, &target); err != nil {
		return err
	}

	if err := m.installDartSdk(jirix, target, m.dartInstDir.Expand()); err != nil {
		return err
	}

	target.Env.Vars = envvar.MergeSlices(target.Env.Vars, []string{
		"DART_SDK=" + m.dartInstDir.Expand(),
		"PATH=" + m.dartInstDir.Join("bin").Expand(),
	})

	if profiles.SchemaVersion() >= 4 {
		target.InstallationDir = m.dartInstDir.String()
		profiles.InstallProfile(profileName, m.dartRoot.String())
	} else {
		target.InstallationDir = m.dartInstDir.Expand()
		profiles.InstallProfile(profileName, m.dartRoot.Expand())
	}

	return profiles.AddProfileTarget(profileName, target)
}

func (m *Manager) installDartSdk(jirix *jiri.X, target profiles.Target, outDir string) error {
	tmpDir, err := jirix.NewSeq().TempDir("", "")
	if err != nil {
		return err
	}
	defer jirix.NewSeq().RemoveAll(tmpDir)

	fn := func() error {
		sdkUrl := dartSdkUrl(m.spec.dartSdkChannel, m.spec.dartSdkVersion, target.OS())
		sdkZipFile := filepath.Join(tmpDir, "dart-sdk.zip")
		return jirix.NewSeq().
			Call(func() error {
			return profiles.Fetch(jirix, sdkZipFile, sdkUrl)
		}, "fetch dart sdk").
			Call(func() error { return profiles.Unzip(jirix, sdkZipFile, tmpDir) }, "unzip dart sdk").
			MkdirAll(filepath.Join(outDir), profiles.DefaultDirPerm).
			Rename(filepath.Join(tmpDir, "dart-sdk"), outDir).
			Done()
	}
	return profiles.AtomicAction(jirix, fn, outDir, "Install Dart SDK")
}

func (m *Manager) Uninstall(jirix *jiri.X, root profiles.RelativePath, target profiles.Target) error {
	if err := m.initForTarget(jirix, root, &target); err != nil {
		return err
	}
	if err := jirix.NewSeq().RemoveAll(m.dartInstDir.Expand()).Done(); err != nil {
		return err
	}
	profiles.RemoveProfileTarget(profileName, target)
	return nil
}

// dartSdkUrl returns the url for the dart SDK with the given version and OS.
func dartSdkUrl(channel, version, os string) string {
	if os == "darwin" {
		os = "macos"
	}
	return fmt.Sprintf("https://storage.googleapis.com/dart-archive/channels/%s/release/%s/sdk/dartsdk-%s-x64-release.zip", channel, version, os)
}
