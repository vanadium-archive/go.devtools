// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nacl

import (
	"flag"
	"fmt"
	"path/filepath"
	"runtime"

	"v.io/jiri/jiri"
	"v.io/jiri/profiles"
	"v.io/jiri/profiles/manager"
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
	manager.Register(profileName, m)
}

type Manager struct {
	root, naclRoot          jiri.RelPath
	naclSrcDir, naclInstDir jiri.RelPath
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

func (m *Manager) initForTarget(jirix *jiri.X, action string, root jiri.RelPath, target *profiles.Target) error {
	m.root = root
	m.naclRoot = root.Join("nacl")
	if !target.IsSet() {
		def := *target
		target.Set("amd64p32-nacl")
		fmt.Fprintf(jirix.Stdout(), "Default target %v reinterpreted as: %v\n", def, target)
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

func (m *Manager) Install(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	if err := m.initForTarget(jirix, "installed", root, &target); err != nil {
		return err
	}
	if p := pdb.LookupProfileTarget(profileName, target); p != nil {
		fmt.Fprintf(jirix.Stdout(), "%v %v is already installed as %v\n", profileName, target, p)
		return nil
	}
	if err := m.installNacl(jirix, target, m.spec); err != nil {
		return err
	}
	target.Env.Vars = envvar.MergeSlices(target.Env.Vars, []string{
		"GOARCH=amd64p32",
		"GOOS=nacl",
		"GOROOT=" + m.naclInstDir.Symbolic(),
	})
	target.InstallationDir = string(m.naclInstDir)
	pdb.InstallProfile(profileName, string(m.naclRoot))
	return pdb.AddProfileTarget(profileName, target)
}

func (m *Manager) Uninstall(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	// ignore errors to allow for older installs to be removed.
	m.initForTarget(jirix, "uninstalled", root, &target)

	s := jirix.NewSeq()
	if err := s.RemoveAll(m.naclInstDir.Abs(jirix)).
		RemoveAll(m.naclSrcDir.Abs(jirix)).Done(); err != nil {
		return err
	}
	if pdb.RemoveProfileTarget(profileName, target) {
		return s.RemoveAll(m.naclRoot.Abs(jirix)).Done()
	}
	return nil
}

// installNacl installs the nacl profile.
func (m *Manager) installNacl(jirix *jiri.X, target profiles.Target, spec versionSpec) error {
	switch runtime.GOOS {
	case "darwin":
	case "linux":
		if err := profiles.InstallPackages(jirix, []string{"g++", "libc6-i386", "zip"}); err != nil {
			return err
		}
	}
	naclSrcDir := m.naclSrcDir.Abs(jirix)
	naclInstDir := m.naclInstDir.Abs(jirix)
	cloneGoPpapiFn := func() error {
		s := jirix.NewSeq()
		tmpDir, err := s.TempDir("", "")
		if err != nil {
			return err
		}
		defer jirix.NewSeq().RemoveAll(tmpDir)
		return s.Pushd(tmpDir).
			Call(func() error { return jirix.Git().Clone(gitRemote, tmpDir) }, "").
			Call(func() error { return jirix.Git().Reset(m.spec.gitRevision) }, "").
			Popd().
			MkdirAll(m.naclRoot.Abs(jirix), profiles.DefaultDirPerm).
			RemoveAll(naclSrcDir).
			Rename(tmpDir, naclSrcDir).Done()
	}
	// Cloning is slow so we handle it as an atomic action and then create
	// a copy for the actual build.
	if err := profiles.AtomicAction(jirix, cloneGoPpapiFn, naclSrcDir, "Clone Go Ppapi repository"); err != nil {
		return err
	}

	// Compile the Go Ppapi compiler.
	compileGoPpapiFn := func() error {
		dir := filepath.Dir(naclInstDir)
		goPpapiCompileScript := filepath.Join(naclInstDir, "src", "make-nacl-amd64p32.sh")
		return jirix.NewSeq().MkdirAll(dir, profiles.DefaultDirPerm).
			Run("cp", "-r", naclSrcDir, naclInstDir).
			Last(goPpapiCompileScript)
	}
	return profiles.AtomicAction(jirix, compileGoPpapiFn, naclInstDir, "Compile Go Ppapi compiler")
}
