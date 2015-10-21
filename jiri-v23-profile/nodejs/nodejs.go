// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodejs

import (
	"flag"
	"fmt"
	"path/filepath"
	"runtime"

	"v.io/jiri/profiles"
	"v.io/jiri/tool"
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
	profiles.Register(profileName, m)
}

type Manager struct {
	root                              string
	nodeRoot, nodeSrcDir, nodeInstDir string
	versionInfo                       *profiles.VersionInfo
	spec                              versionSpec
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

}

func (m Manager) VersionInfo() *profiles.VersionInfo {
	return m.versionInfo
}

func (m *Manager) AddFlags(flags *flag.FlagSet, action profiles.Action) {}

func (m *Manager) initForTarget(target profiles.Target) error {
	if err := m.versionInfo.Lookup(target.Version(), &m.spec); err != nil {
		return err
	}
	m.nodeRoot = filepath.Join(m.root, "profiles", "cout", m.spec.nodeVersion)
	m.nodeInstDir = filepath.Join(m.nodeRoot, target.TargetSpecificDirname())
	m.nodeSrcDir = filepath.Join(m.root, "third_party", "csrc", m.spec.nodeVersion)
	return nil
}

func (m *Manager) Install(ctx *tool.Context, target profiles.Target) error {
	if err := m.initForTarget(target); err != nil {
		return err
	}
	if target.CrossCompiling() {
		return fmt.Errorf("the %q profile does not support cross compilation to %v", profileName, target)
	}
	if err := m.installNode(ctx, target); err != nil {
		return err
	}
	target.InstallationDir = m.nodeInstDir
	profiles.InstallProfile(profileName, m.nodeRoot)
	return profiles.AddProfileTarget(profileName, target)
}

func (m *Manager) Uninstall(ctx *tool.Context, target profiles.Target) error {
	if err := m.initForTarget(target); err != nil {
		return err
	}
	if err := ctx.Run().RemoveAll(m.nodeInstDir); err != nil {
		return err
	}
	profiles.RemoveProfileTarget(profileName, target)
	return nil
}

func (m *Manager) installNode(ctx *tool.Context, target profiles.Target) error {
	switch target.OS() {
	case "darwin":
	case "linux":
		if err := profiles.InstallPackages(ctx, []string{"g++"}); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%q is not supported", target.OS)
	}
	// Build and install NodeJS.
	installNodeFn := func() error {
		if err := ctx.Run().Chdir(m.nodeSrcDir); err != nil {
			return err
		}
		if err := profiles.RunCommand(ctx, nil, "./configure", fmt.Sprintf("--prefix=%v", m.nodeInstDir)); err != nil {
			return err
		}
		if err := profiles.RunCommand(ctx, nil, "make", "clean"); err != nil {
			return err
		}
		if err := profiles.RunCommand(ctx, nil, "make", fmt.Sprintf("-j%d", runtime.NumCPU())); err != nil {
			return err
		}
		if err := profiles.RunCommand(ctx, nil, "make", "install"); err != nil {
			return err
		}
		return nil
	}
	return profiles.AtomicAction(ctx, installNodeFn, m.nodeInstDir, "Build and install node.js")
}
