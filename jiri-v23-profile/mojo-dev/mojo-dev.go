// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// package mojo_dev_profile implements the mojo_dev profile.
// Users must pass the "--mojo-dev.dir" flag when installing the profile,
// pointing it to a checkout of the mojo repo.  It is the user's responsibility
// to sync and build the mojo checkout.
package mojo_dev_profile

import (
	"flag"
	"fmt"
	"path/filepath"

	"v.io/jiri/jiri"
	"v.io/jiri/profiles"
	"v.io/jiri/profiles/profilesmanager"
	"v.io/x/lib/envvar"
)

const (
	profileName     = "mojo-dev"
	mojoDirFlagName = profileName + ".dir"
)

var mojoDir = ""

func init() {
	m := &Manager{
		versionInfo: profiles.NewVersionInfo(profileName, map[string]interface{}{
			"0": nil,
		}, "0"),
	}
	profilesmanager.Register(profileName, m)
}

type Manager struct {
	versionInfo *profiles.VersionInfo
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
	return `Sets up a mojo compilation environment based on a mojo checkout specified in the --mojo-dev.dir flag.`
}

func (m *Manager) AddFlags(flags *flag.FlagSet, action profiles.Action) {
	if action == profiles.Install {
		flags.StringVar(&mojoDir, mojoDirFlagName, "", "Path of mojo repo checkout.")
	}
}

func (m *Manager) Install(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	if mojoDir == "" {
		return fmt.Errorf("flag %q must be set", mojoDirFlagName)
	}
	if !filepath.IsAbs(mojoDir) {
		return fmt.Errorf("flag %q must be absolute path: %s", mojoDirFlagName, mojoDir)
	}

	mojoBuildDir := filepath.Join(mojoDir, "src", "out", "Debug")
	if target.OS() == "android" {
		mojoBuildDir = filepath.Join(mojoDir, "src", "out", "android_Debug")
	}

	target.Env.Vars = envvar.MergeSlices(target.Env.Vars, []string{
		"CGO_CFLAGS=-I" + filepath.Join(mojoDir, "src"),
		"CGO_CXXFLAGS=-I" + filepath.Join(mojoDir, "src"),
		"CGO_LDFLAGS=-L" + filepath.Join(mojoBuildDir, "obj", "mojo") + " -lsystem_thunks",
		"GOPATH=" + mojoDir + ":" + filepath.Join(mojoBuildDir, "gen", "go"),
		"MOJO_DEVTOOLS=" + filepath.Join(mojoDir, "src", "mojo", "devtools", "common"),
		"MOJO_SDK=" + filepath.Join(mojoDir),
		"MOJO_SHELL=" + filepath.Join(mojoBuildDir, "mojo_shell"),
		"MOJO_SERVICES=" + mojoBuildDir,
		"MOJO_SYSTEM_THUNKS=" + filepath.Join(mojoBuildDir, "obj", "mojo", "libsystem_thunks.a"),
	})

	if target.OS() == "android" {
		target.Env.Vars = envvar.MergeSlices(target.Env.Vars, []string{
			"ANDROID_PLATFORM_TOOLS=" + filepath.Join(mojoDir, "src", "third_party", "android_tools", "sdk", "platform-tools"),
			"MOJO_SHELL=" + filepath.Join(mojoBuildDir, "apks", "MojoShell.apk"),
		})
	}

	pdb.InstallProfile(profileName, "mojo-dev") // Needed to confirm installation, but nothing will be inside.
	return pdb.AddProfileTarget(profileName, target)
}

func (m *Manager) Uninstall(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	pdb.RemoveProfileTarget(profileName, target)
	return nil
}
