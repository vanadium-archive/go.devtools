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
	profileName    = "base"
	profileVersion = "1"
)

// The base profile is just a shorthand for go+syncbase.
var baseProfiles = []string{"go", "syncbase"}

func init() {
	profiles.Register(profileName, &Manager{})
}

type Manager struct {
	root string
}

func (Manager) Name() string {
	return profileName
}

func (m Manager) String() string {
	return fmt.Sprintf("%s version:%s root:%s", profileName, profileVersion, m.root)
}

func (m Manager) Root() string {
	return m.root
}

func (m *Manager) SetRoot(root string) {
	m.root = root
}

func (m *Manager) AddFlags(flags *flag.FlagSet, action profiles.Action) {
}

func (m *Manager) Install(ctx *tool.Context, target profiles.Target) error {
	// Install packages
	if target.OS == "linux" {
		if err := profiles.InstallPackages(ctx, []string{"libssl-dev"}); err != nil {
			return err
		}
	}
	// Install profiles.
	for _, profile := range baseProfiles {
		if err := profiles.EnsureProfileTargetIsInstalled(ctx, profile, target, m.root); err != nil {
			return err
		}
	}
	// Merge the environments for go and syncbase and store it in the base profile.
	merged, err := profiles.MergeEnvFromProfiles(profiles.CommonConcatVariables(), profiles.CommonIgnoreVariables(), envvar.VarsFromSlice(target.Env.Vars), target, "syncbase", "go")
	if err != nil {
		return err
	}
	target.Env.Vars = merged
	target.Version = profileVersion
	return profiles.AddProfileTarget(profileName, target)
}

func (m *Manager) Uninstall(ctx *tool.Context, target profiles.Target) error {
	for _, profile := range baseProfiles {
		if err := profiles.EnsureProfileTargetIsUninstalled(ctx, profile, target, m.root); err != nil {
			return err
		}
	}
	profiles.RemoveProfileTarget(profileName, target)
	return nil
}

func (m *Manager) Update(ctx *tool.Context, target profiles.Target) error {
	update, err := profiles.ProfileTargetNeedsUpdate(profileName, target, profileVersion)
	if err != nil {
		return err
	}
	if !update {
		return nil
	}
	return profiles.ErrNoIncrementalUpdate
}
