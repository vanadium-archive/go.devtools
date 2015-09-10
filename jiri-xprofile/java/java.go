// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package java

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"v.io/jiri/profiles"
	"v.io/jiri/tool"
)

const (
	profileName    = "java"
	profileVersion = "1.7+"
	jdkPackage     = "openjdk-7-jdk"
)

func init() {
	profiles.Register(profileName, &Manager{})
}

type Manager struct {
	root     string
	javaRoot string
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
	m.javaRoot = filepath.Join(m.root, "profiles", "java")
}

func (m *Manager) AddFlags(flags *flag.FlagSet, action profiles.Action) {
}

func (m *Manager) Install(ctx *tool.Context, target profiles.Target) error {
	target.Version = profileVersion
	if err := m.install(ctx, target); err != nil {
		return err
	}
	if err := m.installGo(ctx, target); err != nil {
		return err
	}
	profiles.InstallProfile(profileName, m.javaRoot)
	return profiles.AddProfileTarget(profileName, target)
}

func (m *Manager) Uninstall(ctx *tool.Context, target profiles.Target) error {
	if err := ctx.Run().RemoveAll(m.javaRoot); err != nil {
		return err
	}
	profiles.RemoveProfileTarget(profileName, target)
	return nil
}

func (m *Manager) Update(ctx *tool.Context, target profiles.Target) error {
	return profiles.ErrNoIncrementalUpdate
}

// hasJDK returns true iff the JDK already exists on the machine and
// is correctly set up.
func hasJDK(ctx *tool.Context) bool {
	javaHome := os.Getenv("JAVA_HOME")
	if javaHome == "" {
		return false
	}
	_, err := ctx.Run().Stat(filepath.Join(javaHome, "include", "jni.h"))
	return err == nil
}

func (m *Manager) installGo(ctx *tool.Context, target profiles.Target) error {
	goProfileMgr := profiles.LookupManager("go")
	if goProfileMgr == nil {
		return fmt.Errorf("no profile available to install go")
	}
	goProfileMgr.SetRoot(m.root)
	native := profiles.NativeTarget()
	return goProfileMgr.Install(ctx, native)
}

func (m *Manager) install(ctx *tool.Context, target profiles.Target) error {
	switch target.OS {
	case "darwin":
		profiles.InstallPackages(ctx, []string{"gradle"})
		if hasJDK(ctx) {
			return nil
		}
		// Prompt the user to install JDK 1.7+, if not already installed.
		// (Note that JDK cannot be installed via Homebrew.)
		javaHomeBin := "/usr/libexec/java_home"
		if err := profiles.RunCommand(ctx, javaHomeBin, []string{"-t", "CommandLine", "-v", "1.7+"}, nil); err != nil {
			fmt.Printf("Couldn't find a valid JDK installation under JAVA_HOME (%s): installing a new JDK.\n", os.Getenv("JAVA_HOME"))
			profiles.RunCommand(ctx, javaHomeBin, []string{"-t", "CommandLine", "--request"}, nil)
			// Wait for JDK to be installed.
			fmt.Println("Please follow the OS X prompt instructions to install JDK 1.7+.")
			for true {
				time.Sleep(5 * time.Second)
				if err = profiles.RunCommand(ctx, javaHomeBin, []string{"-t", "CommandLine", "-v", "1.7+"}, nil); err == nil {
					break
				}
			}
		}
	case "linux":
		pkgs := []string{"gradle"}
		if !hasJDK(ctx) {
			pkgs = append(pkgs, jdkPackage)
		}
		return profiles.InstallPackages(ctx, pkgs)
	default:
		return fmt.Errorf("%q is not supported", target.OS)
	}
	return nil
}
