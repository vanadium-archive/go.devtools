// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nacl

import (
	"flag"
	"fmt"
	"path/filepath"

	"v.io/jiri/lib/profiles"
	"v.io/jiri/lib/tool"
)

const (
	profileName    = "nacl"
	profileVersion = "1"
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
	// Nacl builds are always native
	m.naclInstDir = filepath.Join(m.naclRoot, "native")
}

func (m *Manager) AddFlags(flags *flag.FlagSet, action profiles.Action) {
}

func (m *Manager) Install(ctx *tool.Context, target profiles.Target) error {
	target.Version = profileVersion
	if target.CrossCompiling() {
		return fmt.Errorf("the %q profile does not support cross compilation to %v", profileName, target)
	}
	if err := m.installNacl(ctx, target); err != nil {
		return err
	}
	target.InstallationDir = m.naclInstDir
	profiles.InstallProfile(profileName, m.naclRoot)
	return profiles.AddProfileTarget(profileName, target)
}

func (m *Manager) Uninstall(ctx *tool.Context, target profiles.Target) error {
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
	return profiles.ErrNoIncrementalUpdate
}

// installNacl installs the nacl profile.
func (m *Manager) installNacl(ctx *tool.Context, target profiles.Target) error {
	switch target.OS {
	case "darwin":
	case "linux":
		if err := profiles.InstallPackages(ctx, []string{"g++", "libc6-i386", "zip"}); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%q is not supported", target.OS)
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

		if profiles.DirectoryExists(ctx, m.naclSrcDir) {
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
		if err := profiles.RunCommand(ctx, "cp", []string{"-r", m.naclSrcDir, m.naclInstDir}, nil); err != nil {
			return err
		}
		goPpapiCompileScript := filepath.Join(m.naclInstDir, "src", "make-nacl-amd64p32.sh")
		if err := profiles.RunCommand(ctx, goPpapiCompileScript, []string{}, nil); err != nil {
			return err
		}
		return nil
	}
	return profiles.AtomicAction(ctx, compileGoPpapiFn, m.naclInstDir, "Compile Go Ppapi compiler")
}
