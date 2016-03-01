// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package terraform_profile

import (
	"flag"
	"fmt"
	"path/filepath"

	"v.io/jiri"
	"v.io/jiri/profiles"
	"v.io/jiri/profiles/profilesmanager"
	"v.io/jiri/profiles/profilesutil"
	"v.io/x/lib/envvar"
)

func Register(installer, profile string) {
	m := &Manager{
		profileInstaller: installer,
		profileName:      profile,
		qualifiedName:    profiles.QualifiedProfileName(installer, profile),
		versionInfo: profiles.NewVersionInfo(profile, map[string]interface{}{
			"0.6.11": "0.6.11",
		}, "0.6.11"),
	}
	profilesmanager.Register(m)
}

type Manager struct {
	profileInstaller, profileName, qualifiedName string
	installDir                                   jiri.RelPath
	versionInfo                                  *profiles.VersionInfo
	version                                      string
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
The terraform profile provides support for Hashicorp's Terraform tool.`
}

func (m *Manager) AddFlags(flags *flag.FlagSet, action profiles.Action) {}

func (m *Manager) initForTarget(root jiri.RelPath, target profiles.Target) error {
	if err := m.versionInfo.Lookup(target.Version(), &m.version); err != nil {
		return err
	}
	m.installDir = root.Join("terraform")
	return nil
}

func (m *Manager) Install(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	if err := m.initForTarget(root, target); err != nil {
		return err
	}

	if target.CrossCompiling() {
		return fmt.Errorf("the %q profile does not support cross compilation to %v", m.qualifiedName, target)
	}

	if err := m.installTerraform(jirix, target); err != nil {
		return err
	}

	target.Env.Vars = envvar.MergeSlices(target.Env.Vars, []string{
		"PATH=" + m.installDir.Symbolic(),
	})

	target.InstallationDir = string(m.installDir)
	pdb.InstallProfile(m.profileInstaller, m.profileName, string(m.installDir))
	return pdb.AddProfileTarget(m.profileInstaller, m.profileName, target)
}

func (m *Manager) Uninstall(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	if err := m.initForTarget(root, target); err != nil {
		return err
	}
	if err := jirix.NewSeq().RemoveAll(m.installDir.Abs(jirix)).Done(); err != nil {
		return err
	}
	pdb.RemoveProfileTarget(m.profileInstaller, m.profileName, target)
	return nil
}

func (m *Manager) installTerraform(jirix *jiri.X, target profiles.Target) error {
	tmpDir, err := jirix.NewSeq().TempDir("", "")
	if err != nil {
		return err
	}
	defer jirix.NewSeq().RemoveAll(tmpDir)

	installFn := func() error {
		url := fmt.Sprintf("https://releases.hashicorp.com/terraform/%s/terraform_%s_%s_%s.zip", m.version, m.version, target.OS(), target.Arch())
		zipFile := filepath.Join(tmpDir, "terraform.zip")
		return jirix.NewSeq().
			Call(func() error { return profilesutil.Fetch(jirix, zipFile, url) }, "fetch %s", url).
			Call(func() error { return profilesutil.Unzip(jirix, zipFile, m.installDir.Abs(jirix)) }, "unzip %s", zipFile).
			Done()
	}
	return profilesutil.AtomicAction(jirix, installFn, m.installDir.Abs(jirix), "Install Terraform")
}
