// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodejs_profile

import (
	"flag"
	"fmt"
	"path/filepath"
	"runtime"

	"v.io/jiri"
	"v.io/jiri/profiles"
	"v.io/jiri/profiles/profilesmanager"
	"v.io/jiri/profiles/profilesutil"
	"v.io/x/lib/envvar"
)

type versionSpec struct {
	nodeVersion string
}

func Register(installer, profile string) {
	m := &Manager{
		profileInstaller: installer,
		profileName:      profile,
		qualifiedName:    profiles.QualifiedProfileName(installer, profile),
		versionInfo: profiles.NewVersionInfo(profile, map[string]interface{}{
			"0.10.24": &versionSpec{"node-v0.10.24"},
			"4.4.1":   &versionSpec{"node-v4.4.1"},
		}, "4.4.1"),
	}
	profilesmanager.Register(m)
}

type Manager struct {
	profileInstaller, profileName, qualifiedName string
	root, nodeRoot                               jiri.RelPath
	nodeSrcDir, nodeInstDir                      jiri.RelPath
	versionInfo                                  *profiles.VersionInfo
	spec                                         versionSpec
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

func (m Manager) VersionInfo() *profiles.VersionInfo {
	return m.versionInfo
}

func (m Manager) Info() string {
	return `
The nodejs profile provides support for node. It installs and builds particular,
tested, versions of node.`
}

func (m *Manager) AddFlags(flags *flag.FlagSet, action profiles.Action) {}

func (m *Manager) initForTarget(root jiri.RelPath, target profiles.Target) error {
	if err := m.versionInfo.Lookup(target.Version(), &m.spec); err != nil {
		return err
	}
	m.root = root
	m.nodeRoot = m.root.Join("nodejs", m.spec.nodeVersion)
	m.nodeInstDir = m.nodeRoot.Join(target.TargetSpecificDirname())
	return nil
}

func (m *Manager) OSPackages(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) ([]string, error) {
	switch target.OS() {
	case "darwin":
	case "linux":
		if target.Version() == "0.10.24" {
			return []string{"g++"}, nil
		}
	default:
		return nil, fmt.Errorf("%q is not supported", target.OS)
	}
	return nil, nil
}

func (m *Manager) Install(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	if err := m.initForTarget(root, target); err != nil {
		return err
	}
	if target.Version() == "0.10.24" {
		// Install from source only for version 0.10.24.
		m.nodeRoot = m.root.Join("cout", m.spec.nodeVersion)
		m.nodeInstDir = m.nodeRoot.Join(target.TargetSpecificDirname())
		m.nodeSrcDir = jiri.NewRelPath("third_party", "csrc", m.spec.nodeVersion)
		if err := m.installNodeFromSource(jirix, target); err != nil {
			return err
		}
	} else {
		// Install from binary tarballs.
		if err := m.installNodeBinaries(jirix, target, m.nodeInstDir.Abs(jirix)); err != nil {
			return err
		}
	}

	target.Env.Vars = envvar.MergeSlices(target.Env.Vars, []string{
		"NODE_BIN=" + m.nodeInstDir.Join("bin").Symbolic(),
		"PATH=" + m.nodeInstDir.Join("bin").Symbolic(),
	})
	target.InstallationDir = string(m.nodeInstDir)
	pdb.InstallProfile(m.profileInstaller, m.profileName, string(m.nodeRoot))
	return pdb.AddProfileTarget(m.profileInstaller, m.profileName, target)
}

func (m *Manager) Uninstall(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	if err := m.initForTarget(root, target); err != nil {
		return err
	}
	if err := jirix.NewSeq().RemoveAll(m.nodeInstDir.Abs(jirix)).Done(); err != nil {
		return err
	}
	pdb.RemoveProfileTarget(m.profileInstaller, m.profileName, target)
	return nil
}

func (m *Manager) installNodeBinaries(jirix *jiri.X, target profiles.Target, outDir string) error {
	if target.Arch() != "amd64" {
		return fmt.Errorf("%q is not supported", target.Arch())
	}

	var arch string
	switch target.Arch() {
	case "amd64":
		arch = "x64"
	case "386":
		arch = "x86"
	default:
		return fmt.Errorf("arch %q is not supported", target.Arch())
	}

	switch target.OS() {
	case "darwin":
		if target.Arch() != "amd64" {
			return fmt.Errorf("arch %q is not supported on darwin", target.Arch())
		}
	case "linux":
	default:
		return fmt.Errorf("os %q is not supported", target.OS())
	}

	tarball := fmt.Sprintf("node-v%s-%s-%s.tar.gz", target.Version(), target.OS(), arch)
	url := fmt.Sprintf("https://nodejs.org/dist/v%s/%s", target.Version(), tarball)
	dirname := fmt.Sprintf("node-v%s-%s-%s", target.Version(), target.OS(), arch)

	tmpDir, err := jirix.NewSeq().TempDir("", "")
	if err != nil {
		return err
	}
	defer jirix.NewSeq().RemoveAll(tmpDir)

	fn := func() error {
		return jirix.NewSeq().
			Pushd(tmpDir).
			Call(func() error {
				return profilesutil.Fetch(jirix, tarball, url)
			}, "fetch nodejs tarball").
			Call(func() error {
				return profilesutil.Untar(jirix, tarball, tmpDir)
			}, "untar nodejs tarball").
			MkdirAll(filepath.Dir(outDir), profilesutil.DefaultDirPerm).
			Rename(filepath.Join(tmpDir, dirname), outDir).
			Done()
	}
	return profilesutil.AtomicAction(jirix, fn, outDir, "Install NodeJS")
}

func (m *Manager) installNodeFromSource(jirix *jiri.X, target profiles.Target) error {
	// Build and install NodeJS.
	installNodeFn := func() error {
		return jirix.NewSeq().Pushd(m.nodeSrcDir.Abs(jirix)).
			Run("./configure", fmt.Sprintf("--prefix=%v", m.nodeInstDir.Abs(jirix))).
			Run("make", "clean").
			Run("make", fmt.Sprintf("-j%d", runtime.NumCPU())).
			Last("make", "install")
	}
	return profilesutil.AtomicAction(jirix, installNodeFn, m.nodeInstDir.Abs(jirix), "Build and install node.js")
}
