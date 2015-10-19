// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nacl

import (
	"flag"
	"fmt"
	"path/filepath"
	"runtime"

	"v.io/jiri/profiles"
	"v.io/jiri/tool"
	"v.io/x/lib/envvar"
)

const (
	profileName    = "nacl"
	profileVersion = "2"
	gitRemote      = "https://vanadium.googlesource.com/release.go.ppapi"
	gitRevision    = "5e967194049bd1a6f097854f09fcbbbaa21afc05"
)

func init() {
	profiles.Register(profileName, &Manager{})
}

type Manager struct {
	root                    string
	naclRoot                string
	naclSrcDir, naclInstDir string
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
	m.naclRoot = filepath.Join(m.root, "profiles", "nacl")
	m.naclSrcDir = filepath.Join(m.naclRoot, gitRevision)
	// Nacl builds are always amd64p32-nacl
	m.naclInstDir = filepath.Join(m.naclRoot, "amd64p32-nacl")
}

func (m *Manager) AddFlags(flags *flag.FlagSet, action profiles.Action) {
}

func (m *Manager) defaultTarget(ctx *tool.Context, action string, target *profiles.Target) error {
	if !target.IsSet() {
		def := *target
		target.Set("nacl=amd64p32-nacl")
		fmt.Fprintf(ctx.Stdout(), "Default target %v reinterpreted as: %v\n", def, target)
	} else {
		if target.Arch != "amd64p32" && target.OS != "nacl" {
			return fmt.Errorf("this profile can only be %v as amd64p32-nacl and not as %v", action, target)
		}
	}
	return nil
}

func (m *Manager) Install(ctx *tool.Context, target profiles.Target) error {
	if err := m.defaultTarget(ctx, "installed", &target); err != nil {
		return err
	}
	target.Version = profileVersion
	if err := m.installNacl(ctx, target); err != nil {
		return err
	}
	target.Env.Vars = envvar.MergeSlices(target.Env.Vars, []string{
		"GOARCH=amd64p32",
		"GOOS=nacl",
		"GOROOT=" + m.naclInstDir,
	})
	target.InstallationDir = m.naclInstDir
	profiles.InstallProfile(profileName, m.naclRoot)
	return profiles.AddProfileTarget(profileName, target)
}

func (m *Manager) Uninstall(ctx *tool.Context, target profiles.Target) error {
	//if err := m.defaultTarget(ctx, "uninstalled", &target); err != nil {
	//	return err
	//}
	// ignore errors for now.
	//m.defaultTarget(ctx, "uninstalled", &target)
	if err := ctx.Run().RemoveAll(m.naclInstDir); err != nil {
		return err
	}
	if err := ctx.Run().RemoveAll(m.naclSrcDir); err != nil {
		return err
	}
	if profiles.RemoveProfileTarget(profileName, target) {
		return ctx.Run().RemoveAll(m.naclRoot)
	}
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

// installNacl installs the nacl profile.
func (m *Manager) installNacl(ctx *tool.Context, target profiles.Target) error {
	switch runtime.GOOS {
	case "darwin":
	case "linux":
		if err := profiles.InstallPackages(ctx, []string{"g++", "libc6-i386", "zip"}); err != nil {
			return err
		}
	}

	cloneGoPpapiFn := func() error {
		tmpDir, err := ctx.Run().TempDir("", "")
		if err != nil {
			return err
		}
		defer ctx.Run().RemoveAll(tmpDir)

		if err := profiles.GitCloneRepo(ctx, gitRemote, gitRevision, tmpDir, profiles.DefaultDirPerm); err != nil {
			return err
		}
		if err := ctx.Run().Chdir(tmpDir); err != nil {
			return err
		}

		if err := ctx.Run().MkdirAll(m.naclRoot, profiles.DefaultDirPerm); err != nil {
			return err
		}

		if ctx.Run().DirectoryExists(m.naclSrcDir) {
			ctx.Run().RemoveAll(m.naclSrcDir)
		}
		if err := ctx.Run().Rename(tmpDir, m.naclSrcDir); err != nil {
			return err
		}
		return nil
	}
	// Cloning is slow so we handle it as an atomic action and then create
	// a copy for the actual build.
	if err := profiles.AtomicAction(ctx, cloneGoPpapiFn, m.naclSrcDir, "Clone Go Ppapi repository"); err != nil {
		return err
	}

	// Compile the Go Ppapi compiler.
	compileGoPpapiFn := func() error {
		if err := profiles.RunCommand(ctx, nil, "cp", "-r", m.naclSrcDir, m.naclInstDir); err != nil {
			return err
		}
		goPpapiCompileScript := filepath.Join(m.naclInstDir, "src", "make-nacl-amd64p32.sh")
		if err := profiles.RunCommand(ctx, nil, goPpapiCompileScript); err != nil {
			return err
		}
		return nil
	}
	return profiles.AtomicAction(ctx, compileGoPpapiFn, m.naclInstDir, "Compile Go Ppapi compiler")
}
