// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package base_profile

import (
	"flag"
	"fmt"

	"v.io/jiri"
	"v.io/jiri/profiles"
	"v.io/jiri/profiles/profilesmanager"
	"v.io/jiri/profiles/profilesreader"
	"v.io/x/lib/envvar"
)

type versionSpec struct {
	dependencies []struct{ name, version string }
}

func Register(installer, profile string) {
	m := &Manager{
		profileInstaller: installer,
		profileName:      profile,
		qualifiedName:    profiles.QualifiedProfileName(installer, profile),
		versionInfo: profiles.NewVersionInfo(profile,
			map[string]interface{}{
				"1": &versionSpec{[]struct{ name, version string }{
					{"go", ""},
					{"syncbase", ""}},
				},
				"2": &versionSpec{[]struct{ name, version string }{
					{"go", "1.5.1.1:2738c5e0"},
					{"syncbase", ""}},
				},
				"3": &versionSpec{[]struct{ name, version string }{
					{"go", "1.5.2"},
					{"syncbase", ""}},
				},
				"4": &versionSpec{[]struct{ name, version string }{
					{"go", "1.5.2.1:56093743"},
					{"syncbase", ""}},
				},
				"5": &versionSpec{[]struct{ name, version string }{
					{"go", "1.6"},
					{"syncbase", ""}},
				},
				"6": &versionSpec{[]struct{ name, version string }{
					{"go", "1.8.3"},
					{"syncbase", ""}},
				},
			}, "6"),
	}
	profilesmanager.Register(m)
}

type Manager struct {
	profileInstaller, profileName, qualifiedName string
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

func (m Manager) Info() string {
	return `
The base profile is a convenient shorthand for installing the profiles that all
vanadium projects need, this is currently go and syncbase.`
}

func (m Manager) VersionInfo() *profiles.VersionInfo {
	return m.versionInfo
}

func (m *Manager) AddFlags(flags *flag.FlagSet, action profiles.Action) {
}

func (m *Manager) OSPackages(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) ([]string, error) {
	if err := m.versionInfo.Lookup(target.Version(), &m.spec); err != nil {
		return nil, err
	}
	var packages []string
	if !target.CrossCompiling() && target.OS() == "linux" && (target.Version() == "1" || target.Version() == "2" || target.Version() == "3" || target.Version() == "4") {
		// Version 5 onwards uses go 1.6+, where there is no need for "libssl-dev".
		packages = []string{"libssl-dev"}
	}
	// Get packages from dependent profiles.
	// TODO(nlacasse): Consider making the notion of "dependent profiles"
	// something that jiri understands, and move this logic (and the similar
	// logic in Install) into jiri.
	for _, profile := range m.spec.dependencies {
		qname := profiles.QualifiedProfileName(m.profileInstaller, profile.name)
		depManager := profilesmanager.LookupManager(qname)
		if depManager == nil {
			return nil, fmt.Errorf("no manager found for dependent profile %v", profile.name)
		}
		depPackages, err := depManager.OSPackages(jirix, pdb, root, target)
		if err != nil {
			return nil, err
		}
		packages = append(packages, depPackages...)
	}
	return packages, nil
}

func (m *Manager) Install(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	if err := m.versionInfo.Lookup(target.Version(), &m.spec); err != nil {
		return err
	}
	// Install profiles.
	profileEnvs := [][]string{}
	for _, profile := range m.spec.dependencies {
		dependency := target
		dependency.SetVersion(profile.version)
		if err := profilesmanager.EnsureProfileTargetIsInstalled(jirix, pdb, m.profileInstaller, profile.name, root, dependency); err != nil {
			return err
		}
		installed := pdb.LookupProfileTarget(m.profileInstaller, profile.name, dependency)
		if installed == nil {
			return fmt.Errorf("%s %s should have been installed, but apparently is not", profile.name, dependency)
		}
		profileEnvs = append(profileEnvs, installed.Env.Vars)
	}
	// Merge the environments for go and syncbase and store it in the base profile.
	base := envvar.VarsFromSlice(target.Env.Vars)
	base.Set("GOARCH", target.Arch())
	// iOS specifically uses Darwin as its GOOS. Using "ios" aka target.OS() will make go cry.
	os := target.OS()
	if target.OS() == "ios" {
		os = "darwin"
	}
	base.Set("GOOS", os)
	// Slight modifications to ProfileMergePolicies: Want the values from
	// the "go" profile we depend on to prevail.
	mp := profilesreader.ProfileMergePolicies()
	mp["GOROOT"] = profilesreader.UseLast
	mp["GOROOT_BOOTSTRAP"] = profilesreader.IgnoreBaseUseLast
	mp["CGO_ENABLED"] = profilesreader.IgnoreBaseUseLast
	profilesreader.MergeEnv(mp, base, profileEnvs...)
	target.Env.Vars = base.ToSlice()
	pdb.InstallProfile(m.profileInstaller, m.profileName, string(root))
	return pdb.AddProfileTarget(m.profileInstaller, m.profileName, target)
}

func (m *Manager) Uninstall(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	if err := m.versionInfo.Lookup(target.Version(), &m.spec); err != nil {
		return err
	}
	for _, profile := range m.spec.dependencies {
		dependency := target
		dependency.SetVersion(profile.version)
		if err := profilesmanager.EnsureProfileTargetIsUninstalled(jirix, pdb, m.profileInstaller, profile.name, root, dependency); err != nil {
			return err
		}
	}
	pdb.RemoveProfileTarget(m.profileInstaller, m.profileName, target)
	return nil
}
