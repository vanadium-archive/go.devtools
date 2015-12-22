// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodejs

import (
	"flag"
	"fmt"
	"runtime"

	"v.io/jiri/jiri"
	"v.io/jiri/profiles"
	"v.io/jiri/profiles/manager"
)

const (
	profileName = "nodejs"
)

type versionSpec struct {
	nodeVersion string
}

func init() {
	m := &Manager{
		versionInfo: profiles.NewVersionInfo(profileName, map[string]interface{}{
			"10.24": &versionSpec{"node-v0.10.24"},
		}, "10.24"),
	}
	manager.Register(profileName, m)
}

type Manager struct {
	root, nodeRoot          jiri.RelPath
	nodeSrcDir, nodeInstDir jiri.RelPath
	versionInfo             *profiles.VersionInfo
	spec                    versionSpec
}

func (Manager) Name() string {
	return profileName
}

func (m Manager) String() string {
	return fmt.Sprintf("%s[%s]", profileName, m.versionInfo.Default())
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
		return fmt.Errorf("the %q profile does not support cross compilation to %v", profileName, target)
	}
	if err := m.installNode(jirix, target); err != nil {
		return err
	}
	target.InstallationDir = string(m.nodeInstDir)
	pdb.InstallProfile(profileName, string(m.nodeRoot))
	return pdb.AddProfileTarget(profileName, target)
}

func (m *Manager) Uninstall(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	if err := m.initForTarget(root, target); err != nil {
		return err
	}
	if err := jirix.NewSeq().RemoveAll(m.nodeInstDir.Abs(jirix)).Done(); err != nil {
		return err
	}
	pdb.RemoveProfileTarget(profileName, target)
	return nil
}

func (m *Manager) installNode(jirix *jiri.X, target profiles.Target) error {
	switch target.OS() {
	case "darwin":
	case "linux":
		if err := profiles.InstallPackages(jirix, []string{"g++"}); err != nil {
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
	return profiles.AtomicAction(jirix, installNodeFn, m.nodeInstDir.Abs(jirix), "Build and install node.js")
}
