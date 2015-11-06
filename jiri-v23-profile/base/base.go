// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package base

import (
	"flag"
	"fmt"

	"v.io/jiri/profiles"
	"v.io/jiri/tool"
	"v.io/x/lib/envvar"
)

const (
	profileName = "base"
)

type versionSpec struct {
	dependencies []struct{ name, version string }
}

func init() {
	m := &Manager{
		versionInfo: profiles.NewVersionInfo(profileName,
			map[string]interface{}{
				"1": &versionSpec{[]struct{ name, version string }{
					{"go", ""},
					{"syncbase", ""}},
				},
				"2": &versionSpec{[]struct{ name, version string }{
					{"go", "master"},
					{"syncbase", ""}},
				},
			}, "1"),
	}
	profiles.Register(profileName, m)
}

type Manager struct {
	root        string
	versionInfo *profiles.VersionInfo
	spec        versionSpec
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

func (m *Manager) Install(ctx *tool.Context, target profiles.Target) error {
	// Install packages
	if target.OS() == "linux" {
		if err := profiles.InstallPackages(ctx, []string{"libssl-dev"}); err != nil {
			return err
		}
	}
	if err := m.versionInfo.Lookup(target.Version(), &m.spec); err != nil {
		return err
	}
	// Install profiles.
	profileEnvs := [][]string{}
	for _, profile := range m.spec.dependencies {
		dependency := target
		dependency.SetVersion(profile.version)
		if err := profiles.EnsureProfileTargetIsInstalled(ctx, profile.name, dependency, m.root); err != nil {
			return err
		}
		installed := profiles.LookupProfileTarget(profile.name, dependency)
		if installed == nil {
			return fmt.Errorf("%s %s should have been installed, but apparently is not", profile.name, dependency)
		}
		profileEnvs = append(profileEnvs, installed.Env.Vars)
	}
	// Merge the environments for go and syncbase and store it in the base profile.
	base := envvar.VarsFromSlice(target.Env.Vars)
	base.Set("GOARCH", target.Arch())
	base.Set("GOOS", target.OS())
	profiles.MergeEnv(profiles.ProfileMergePolicies(), base, profileEnvs...)
	target.Env.Vars = base.ToSlice()
	return profiles.AddProfileTarget(profileName, target)
}

func (m *Manager) Uninstall(ctx *tool.Context, target profiles.Target) error {
	if err := m.versionInfo.Lookup(target.Version(), &m.spec); err != nil {
		return err
	}
	for _, profile := range m.spec.dependencies {
		dependency := target
		dependency.SetVersion(profile.version)
		if err := profiles.EnsureProfileTargetIsUninstalled(ctx, profile.name, target, m.root); err != nil {
			return err
		}
	}
	profiles.RemoveProfileTarget(profileName, target)
	return nil
}
