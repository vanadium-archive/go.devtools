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
	root, naclRoot          profiles.RelativePath
	naclSrcDir, naclInstDir profiles.RelativePath
	versionInfo             *profiles.VersionInfo
	spec                    versionSpec
}

func (Manager) Name() string {
	return profileName
}

func (m Manager) String() string {
	return fmt.Sprintf("%s[%s]", profileName, m.versionInfo.Default())
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

func (m *Manager) initForTarget(ctx *tool.Context, action string, root profiles.RelativePath, target *profiles.Target) error {
	m.root = root
	m.naclRoot = root.Join("nacl")

	if !target.IsSet() {
		def := *target
		target.Set("amd64p32-nacl")
		fmt.Fprintf(ctx.Stdout(), "Default target %v reinterpreted as: %v\n", def, target)
	} else {
		if target.Arch() != "amd64p32" && target.OS() != "nacl" {
			return fmt.Errorf("this profile can only be %v as amd64p32-nacl and not as %v", action, target)
		}
	}
	if err := m.versionInfo.Lookup(target.Version(), &m.spec); err != nil {
		return err
	}
	m.naclSrcDir = m.naclRoot.Join(m.spec.gitRevision)
	m.naclInstDir = m.naclRoot.Join(target.TargetSpecificDirname(), m.spec.gitRevision)
	return nil
}

func relPath(rp profiles.RelativePath) string {
	if profiles.SchemaVersion() >= 4 {
		return rp.String()
	}
	return rp.Expand()
}

func (m *Manager) Install(ctx *tool.Context, root profiles.RelativePath, target profiles.Target) error {
	if err := m.initForTarget(ctx, "installed", root, &target); err != nil {
		return err
	}

	if err := m.installNacl(ctx, target, m.spec); err != nil {
		return err
	}
	target.Env.Vars = envvar.MergeSlices(target.Env.Vars, []string{
		"GOARCH=amd64p32",
		"GOOS=nacl",
		"GOROOT=" + relPath(m.naclInstDir),
	})
	if profiles.SchemaVersion() >= 4 {
		target.InstallationDir = m.naclInstDir.String()
		profiles.InstallProfile(profileName, m.naclRoot.String())
	} else {
		target.InstallationDir = m.naclInstDir.Expand()
		profiles.InstallProfile(profileName, m.naclRoot.Expand())

	}
	return profiles.AddProfileTarget(profileName, target)
}

func (m *Manager) Uninstall(ctx *tool.Context, root profiles.RelativePath, target profiles.Target) error {
	// ignore errors to allow for older installs to be removed.
	m.initForTarget(ctx, "uninstalled", root, &target)

	if err := ctx.Run().RemoveAll(m.naclInstDir.Expand()); err != nil {
		return err
	}
	if err := ctx.Run().RemoveAll(m.naclSrcDir.Expand()); err != nil {
		return err
	}
	if profiles.RemoveProfileTarget(profileName, target) {
		return ctx.Run().RemoveAll(m.naclRoot.Expand())
	}
	return nil
}

// installNacl installs the nacl profile.
func (m *Manager) installNacl(ctx *tool.Context, target profiles.Target, spec versionSpec) error {
	switch runtime.GOOS {
	case "darwin":
	case "linux":
		if err := profiles.InstallPackages(ctx, []string{"g++", "libc6-i386", "zip"}); err != nil {
			return err
		}
	}
	naclSrcDir := m.naclSrcDir.Expand()
	naclInstDir := m.naclInstDir.Expand()
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

		if err := ctx.Run().MkdirAll(m.naclRoot.Expand(), profiles.DefaultDirPerm); err != nil {
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
