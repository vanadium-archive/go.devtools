// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package java

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"v.io/jiri/profiles"
	"v.io/jiri/tool"
	"v.io/x/lib/envvar"
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
	javaHome, err := m.install(ctx, target)
	if err != nil {
		return err
	}
	if err := profiles.EnsureProfileTargetIsInstalled(ctx, "base", target, m.root); err != nil {
		return err
	}
	// NOTE(spetrovic): For now, we install android profile along with Java, as the two are bundled
	// up for ease of development.
	androidTarget, err := profiles.NewTarget("android=arm-android")
	if err != nil {
		return err
	}
	if err := profiles.EnsureProfileTargetIsInstalled(ctx, "android", androidTarget, m.root); err != nil {
		return err
	}

	target.InstallationDir = javaHome
	env := envvar.VarsFromSlice(target.Env.Vars)
	cgoflags := env.GetTokens("CGO_CFLAGS", " ")
	javaflags := []string{
		fmt.Sprintf("-I%s", filepath.Join(javaHome, "include")),
		fmt.Sprintf("-I%s", filepath.Join(javaHome, "include", target.OS)),
	}
	env.SetTokens("CGO_CFLAGS", append(cgoflags, javaflags...), " ")
	env.Set("JAVA_HOME", javaHome)

	// Merge the base environment variables and store them in the java profile
	merged, err := profiles.MergeEnvFromProfiles(profiles.CommonConcatVariables(), profiles.CommonIgnoreVariables(), env, target, "base")
	if err != nil {
		return err
	}
	target.Env.Vars = merged
	target.Version = profileVersion
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
	update, err := profiles.ProfileTargetNeedsUpdate(profileName, target, profileVersion)
	if err != nil {
		return err
	}
	if !update {
		return nil
	}
	return profiles.ErrNoIncrementalUpdate
}

func (m *Manager) install(ctx *tool.Context, target profiles.Target) (string, error) {
	switch target.OS {
	case "darwin":
		profiles.InstallPackages(ctx, []string{"gradle"})
		if javaHome, err := getJDKDarwin(ctx); err == nil {
			return javaHome, nil
		}
		// Prompt the user to install JDK 1.7+, if not already installed.
		// (Note that JDK cannot be installed via Homebrew.)
		javaHomeBin := "/usr/libexec/java_home"
		if err := profiles.RunCommand(ctx, nil, javaHomeBin, "-t", "CommandLine", "-v", "1.7+"); err != nil {
			fmt.Fprintf(ctx.Stderr(), "Couldn't find a valid JDK installation under JAVA_HOME (%s): installing a new JDK.\n", os.Getenv("JAVA_HOME"))
			profiles.RunCommand(ctx, nil, javaHomeBin, "-t", "CommandLine", "--request")
			// Wait for JDK to be installed.
			fmt.Println("Please follow the OS X prompt instructions to install JDK 1.7+.")
			for true {
				time.Sleep(5 * time.Second)
				if err = profiles.RunCommand(ctx, nil, javaHomeBin, "-t", "CommandLine", "-v", "1.7+"); err == nil {
					break
				}
			}
		}
		return getJDKDarwin(ctx)
	case "linux":
		pkgs := []string{"gradle"}
		if _, err := getJDKLinux(ctx); err != nil {
			pkgs = append(pkgs, jdkPackage)
		}
		if err := profiles.InstallPackages(ctx, pkgs); err != nil {
			return "", err
		}
		return getJDKLinux(ctx)
	default:
		return "", fmt.Errorf("%q is not supported", target.OS)
	}
}

func checkInstall(ctx *tool.Context, home string) error {
	_, err := ctx.Run().Stat(filepath.Join(home, "include", "jni.h"))
	return err
}

func getJDKLinux(ctx *tool.Context) (string, error) {
	if javaHome := os.Getenv("JAVA_HOME"); len(javaHome) > 0 {
		return javaHome, checkInstall(ctx, javaHome)
	}
	javacBin := "/usr/bin/javac"
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	ctx.Run().CommandWithOpts(opts, "readlink", "-f", javacBin)
	if out.Len() == 0 {
		return "", errors.New("Couldn't find a valid Java installation: did you run \"jiri profile install java\"?")
	}

	// Strip "/bin/javac" from the returned path.
	javaHome := strings.TrimSuffix(out.String(), "/bin/javac\n")
	return javaHome, checkInstall(ctx, javaHome)
}

func getJDKDarwin(ctx *tool.Context) (string, error) {
	if javaHome := os.Getenv("JAVA_HOME"); len(javaHome) > 0 {
		return javaHome, checkInstall(ctx, javaHome)
	}
	javaHomeBin := "/usr/libexec/java_home"
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	ctx.Run().CommandWithOpts(opts, javaHomeBin, "-t", "CommandLine", "-v", "1.7+")
	if out.Len() == 0 {
		return "", errors.New("Couldn't find a valid Java installation: did you run \"jiri profile install java\"?")
	}
	jdkLoc, _, err := bufio.NewReader(strings.NewReader(out.String())).ReadLine()
	if err != nil {
		return "", fmt.Errorf("Couldn't find a valid Java installation: %v", err)
	}
	return string(jdkLoc), checkInstall(ctx, string(jdkLoc))
}
