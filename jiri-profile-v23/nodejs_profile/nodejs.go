// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodejs_profile

import (
	"flag"
	"fmt"
	"runtime"

	"v.io/jiri/jiri"
	"v.io/jiri/profiles"
	"v.io/jiri/profiles/profilesmanager"
	"v.io/jiri/profiles/profilesutil"
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
			"10.24": &versionSpec{"node-v0.10.24"},
		}, "10.24"),
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
	m.nodeRoot = m.root.Join("cout", m.spec.nodeVersion)
	m.nodeInstDir = m.nodeRoot.Join(target.TargetSpecificDirname())
	m.nodeSrcDir = jiri.NewRelPath("third_party", "csrc", m.spec.nodeVersion)
	return nil
}

func (m *Manager) Install(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	if err := m.initForTarget(root, target); err != nil {
		return err
	}
	if target.CrossCompiling() {
		return fmt.Errorf("the %q profile does not support cross compilation to %v", m.qualifiedName, target)
	}
	if err := m.installNode(jirix, target); err != nil {
		return err
	}
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

func (m *Manager) installNode(jirix *jiri.X, target profiles.Target) error {
	switch target.OS() {
	case "darwin":
	case "linux":
		if err := profilesutil.InstallPackages(jirix, []string{"g++"}); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%q is not supported", target.OS)
	}
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
