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

var profileName = "base"
var profileVersion = "1"

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
	pkgs := []string{}
	if target.OS == "linux" {
		pkgs = []string{"libssl-dev"}

	}
	if err := profiles.InstallPackages(ctx, pkgs); err != nil {
		return err
	}

	// Install profiles.
	for _, profile := range []string{"go", "syncbase"} {
		if !profiles.HasTarget(profile, target) {
			syncbaseMgr := profiles.LookupManager(profile)
			if syncbaseMgr == nil {
				return fmt.Errorf("syncbase profile is not available")
			}
			syncbaseMgr.SetRoot(m.root)
			if err := syncbaseMgr.Install(ctx, target); err != nil {
				return err
			}
		}
	}
	goTarget := profiles.LookupProfileTarget("go", target)
	syncbaseTarget := profiles.LookupProfileTarget("syncbase", target)
	target.Version = profileVersion
	// Merge the environments for the base target, go and syncbase.
	target.Env.Vars = envvar.MergeSlices(target.Env.Vars, goTarget.Env.Vars, syncbaseTarget.Env.Vars)
	return profiles.AddProfileTarget(profileName, target)
}

func (m *Manager) Uninstall(ctx *tool.Context, target profiles.Target) error {
	profiles.RemoveProfileTarget(profileName, target)
	return nil
}

func (m *Manager) Update(ctx *tool.Context, target profiles.Target) error {
	if !profiles.ProfileTargetNeedsUpdate(profileName, target, profileVersion) {
		return nil
	}
	return profiles.ErrNoIncrementalUpdate
}
