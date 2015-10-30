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
	profileName = "java"
)

type versionSpec struct {
	jdkVersion, jdkPackage string
}

func init() {
	m := &Manager{
		versionInfo: profiles.NewVersionInfo(profileName, map[string]interface{}{
			"1.7+": &versionSpec{"1.7+", "openjdk-7-jdk"},
		}, "1.7+"),
	}
	profiles.Register(profileName, m)
}

type Manager struct {
	root        string
	javaRoot    string
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
	m.javaRoot = filepath.Join(m.root, "profiles", "java")
}

func (m Manager) Info() string {
	return `
The java profile provides support for Java and in particular installs java related
tools such as gradle. It does not install a jre, but rather attempts to locate one
on the current system and prompts the user to install it if not present. It also
installs the android profile since android is the primary use of Java. It only
supports a single target of 'arm-android' and assumes it as the default.`
}

func (m Manager) VersionInfo() *profiles.VersionInfo {
	return m.versionInfo
}

func (m *Manager) AddFlags(flags *flag.FlagSet, action profiles.Action) {
}

func (m *Manager) Install(ctx *tool.Context, target profiles.Target) error {
	javaHome, err := m.install(ctx, target)
	if err != nil {
		return err
	}
	baseTarget := target
	baseTarget.SetVersion("")
	if err := profiles.EnsureProfileTargetIsInstalled(ctx, "base", baseTarget, m.root); err != nil {
		return err
	}
	// NOTE(spetrovic): For now, we install android profile along with Java,
	// as the two are bundled up for ease of development.
	androidTarget, err := profiles.NewTarget("arm-android")
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
		fmt.Sprintf("-I%s", filepath.Join(javaHome, "include", target.OS())),
	}
	env.SetTokens("CGO_CFLAGS", append(cgoflags, javaflags...), " ")
	env.Set("JAVA_HOME", javaHome)

	// Merge the base environment variables and store them in the java profile
	merged, err := profiles.MergeEnvFromProfiles(profiles.CommonConcatVariables(), profiles.CommonIgnoreVariables(), env, baseTarget, "base")
	if err != nil {
		return err
	}
	target.Env.Vars = merged
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

func (m *Manager) install(ctx *tool.Context, target profiles.Target) (string, error) {
	if err := m.versionInfo.Lookup(target.Version(), &m.spec); err != nil {
		return "", err
	}
	switch target.OS() {
	case "darwin":
		profiles.InstallPackages(ctx, []string{"gradle"})
		if javaHome, err := getJDKDarwin(ctx, m.spec); err == nil {
			return javaHome, nil
		}
		// Prompt the user to install JDK 1.7+, if not already installed.
		// (Note that JDK cannot be installed via Homebrew.)
		javaHomeBin := "/usr/libexec/java_home"
		if err := profiles.RunCommand(ctx, nil, javaHomeBin, "-t", "CommandLine", "-v", m.spec.jdkVersion); err != nil {
			fmt.Fprintf(ctx.Stderr(), "Couldn't find a valid JDK installation under JAVA_HOME (%s): installing a new JDK.\n", os.Getenv("JAVA_HOME"))
			profiles.RunCommand(ctx, nil, javaHomeBin, "-t", "CommandLine", "--request")
			// Wait for JDK to be installed.
			fmt.Println("Please follow the OS X prompt instructions to install JDK 1.7+.")
			for true {
				time.Sleep(5 * time.Second)
				if err = profiles.RunCommand(ctx, nil, javaHomeBin, "-t", "CommandLine", "-v", m.spec.jdkVersion); err == nil {
					break
				}
			}
		}
		return getJDKDarwin(ctx, m.spec)
	case "linux":
		pkgs := []string{"gradle"}
		if _, err := getJDKLinux(ctx, m.spec); err != nil {
			pkgs = append(pkgs, m.spec.jdkPackage)
		}
		if err := profiles.InstallPackages(ctx, pkgs); err != nil {
			return "", err
		}
		return getJDKLinux(ctx, m.spec)
	default:
		return "", fmt.Errorf("%q is not supported", target.OS)
	}
}

func checkInstall(ctx *tool.Context, home string) error {
	_, err := ctx.Run().Stat(filepath.Join(home, "include", "jni.h"))
	return err
}

func getJDKLinux(ctx *tool.Context, spec versionSpec) (string, error) {
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

func getJDKDarwin(ctx *tool.Context, spec versionSpec) (string, error) {
	if javaHome := os.Getenv("JAVA_HOME"); len(javaHome) > 0 {
		return javaHome, checkInstall(ctx, javaHome)
	}
	javaHomeBin := "/usr/libexec/java_home"
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	ctx.Run().CommandWithOpts(opts, javaHomeBin, "-t", "CommandLine", "-v", spec.jdkVersion)
	if out.Len() == 0 {
		return "", errors.New("Couldn't find a valid Java installation: did you run \"jiri profile install java\"?")
	}
	jdkLoc, _, err := bufio.NewReader(strings.NewReader(out.String())).ReadLine()
	if err != nil {
		return "", fmt.Errorf("Couldn't find a valid Java installation: %v", err)
	}
	return string(jdkLoc), checkInstall(ctx, string(jdkLoc))
}
