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
	profileName = "nacl"
	gitRemote   = "https://vanadium.googlesource.com/release.go.ppapi"
)

type versionSpec struct {
	gitRevision string
}

func init() {
	m := &Manager{
		versionInfo: profiles.NewVersionInfo(profileName, map[string]interface{}{
			"1": &versionSpec{"5e967194049bd1a6f097854f09fcbbbaa21afc05"},
			"2": &versionSpec{"5e967194049bd1a6f097854f09fcbbbaa21afc05"},
		}, "2"),
	}
	profiles.Register(profileName, m)
}

type Manager struct {
	root        string
	naclRoot    string
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
	m.naclRoot = filepath.Join(m.root, "profiles", "nacl")
}

func (m Manager) VersionInfo() *profiles.VersionInfo {
	return m.versionInfo
}

func (m Manager) Info() string {
	return `
The nacl profile provides support for native client builds for chrome. It
clones and builds the go.ppapi git repository. It supports a single target of
amd64p32-nacl and assumes it as the default`
}

func (m *Manager) AddFlags(flags *flag.FlagSet, action profiles.Action) {
}

func (m *Manager) defaultTarget(ctx *tool.Context, action string, target *profiles.Target) error {
	if !target.IsSet() {
		def := *target
		target.Set("nacl=amd64p32-nacl")
		fmt.Fprintf(ctx.Stdout(), "Default target %v reinterpreted as: %v\n", def, target)
	} else {
		if target.Arch() != "amd64p32" && target.OS() != "nacl" {
			return fmt.Errorf("this profile can only be %v as amd64p32-nacl and not as %v", action, target)
		}
	}
	return nil
}

func (m *Manager) Install(ctx *tool.Context, target profiles.Target) error {
	if err := m.defaultTarget(ctx, "installed", &target); err != nil {
		return err
	}
	if err := m.versionInfo.Lookup(target.Version(), &m.spec); err != nil {
		return err
	}
	naclInstDir := filepath.Join(m.naclRoot, target.TargetSpecificDirname(), m.spec.gitRevision)

	if err := m.installNacl(ctx, target, m.spec, naclInstDir); err != nil {
		return err
	}
	target.InstallationDir = naclInstDir
	target.Env.Vars = envvar.MergeSlices(target.Env.Vars, []string{
		"GOARCH=amd64p32",
		"GOOS=nacl",
		"GOROOT=" + naclInstDir,
	})
	profiles.InstallProfile(profileName, m.naclRoot)
	return profiles.AddProfileTarget(profileName, target)
}

func (m *Manager) Uninstall(ctx *tool.Context, target profiles.Target) error {
	// ignore errors to allow for older installs to be removed.
	m.defaultTarget(ctx, "uninstalled", &target)
	if err := m.versionInfo.Lookup(target.Version(), &m.spec); err != nil {
		return err
	}
	naclSrcDir := filepath.Join(m.naclRoot, m.spec.gitRevision)
	naclInstDir := filepath.Join(m.naclRoot, target.TargetSpecificDirname(), m.spec.gitRevision)
	if err := ctx.Run().RemoveAll(naclInstDir); err != nil {
		return err
	}
	if err := ctx.Run().RemoveAll(naclSrcDir); err != nil {
		return err
	}
	if profiles.RemoveProfileTarget(profileName, target) {
		return ctx.Run().RemoveAll(m.naclRoot)
	}
	return nil
}

// installNacl installs the nacl profile.
func (m *Manager) installNacl(ctx *tool.Context, target profiles.Target, spec versionSpec, naclInstDir string) error {
	switch runtime.GOOS {
	case "darwin":
	case "linux":
		if err := profiles.InstallPackages(ctx, []string{"g++", "libc6-i386", "zip"}); err != nil {
			return err
		}
	}

	naclSrcDir := filepath.Join(m.naclRoot, spec.gitRevision)

	cloneGoPpapiFn := func() error {
		tmpDir, err := ctx.Run().TempDir("", "")
		if err != nil {
			return err
		}
		defer ctx.Run().RemoveAll(tmpDir)

		if err := profiles.GitCloneRepo(ctx, gitRemote, spec.gitRevision, tmpDir, profiles.DefaultDirPerm); err != nil {
			return err
		}
		if err := ctx.Run().Chdir(tmpDir); err != nil {
			return err
		}

		if err := ctx.Run().MkdirAll(m.naclRoot, profiles.DefaultDirPerm); err != nil {
			return err
		}

		if ctx.Run().DirectoryExists(naclSrcDir) {
			ctx.Run().RemoveAll(naclSrcDir)
		}
		if err := ctx.Run().Rename(tmpDir, naclSrcDir); err != nil {
			return err
		}
		return nil
	}
	// Cloning is slow so we handle it as an atomic action and then create
	// a copy for the actual build.
	if err := profiles.AtomicAction(ctx, cloneGoPpapiFn, naclSrcDir, "Clone Go Ppapi repository"); err != nil {
		return err
	}

	// Compile the Go Ppapi compiler.
	compileGoPpapiFn := func() error {
		dir := filepath.Dir(naclInstDir)
		if err := ctx.Run().MkdirAll(dir, profiles.DefaultDirPerm); err != nil {
			return err
		}
		if err := profiles.RunCommand(ctx, nil, "cp", "-r", naclSrcDir, naclInstDir); err != nil {
			return err
		}
		goPpapiCompileScript := filepath.Join(naclInstDir, "src", "make-nacl-amd64p32.sh")
		if err := profiles.RunCommand(ctx, nil, goPpapiCompileScript); err != nil {
			return err
		}
		return nil
	}
	return profiles.AtomicAction(ctx, compileGoPpapiFn, naclInstDir, "Compile Go Ppapi compiler")
}
