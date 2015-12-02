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

	"v.io/jiri/jiri"
	"v.io/jiri/profiles"
	"v.io/x/lib/envvar"
)

const (
	profileName = "java"
)

type versionSpec struct {
	jdkVersion string
}

func init() {
	m := &Manager{
		versionInfo: profiles.NewVersionInfo(profileName, map[string]interface{}{
			"1.7+": versionSpec{"1.7+"},
			"1.8+": versionSpec{"1.8+"},
		}, "1.8+"),
	}
	profiles.Register(profileName, m)
}

type Manager struct {
	root, javaRoot jiri.RelPath
	versionInfo    *profiles.VersionInfo
	spec           versionSpec
}

func (Manager) Name() string {
	return profileName
}

func (m Manager) String() string {
	return fmt.Sprintf("%s[%s]", profileName, m.versionInfo.Default())
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

func (m *Manager) initForTarget(root jiri.RelPath, target profiles.Target) error {
	m.root = root
	m.javaRoot = root.Join("java")
	if err := m.versionInfo.Lookup(target.Version(), &m.spec); err != nil {
		return err
	}
	return nil
}

func (m *Manager) Install(jirix *jiri.X, root jiri.RelPath, target profiles.Target) error {
	if err := m.initForTarget(root, target); err != nil {
		return err
	}

	javaHome, err := m.install(jirix, target)
	if err != nil {
		return err
	}
	baseTarget := target
	baseTarget.SetVersion("")
	if err := profiles.EnsureProfileTargetIsInstalled(jirix, "base", root, baseTarget); err != nil {
		return err
	}
	// NOTE(spetrovic): For now, we install android profile along with Java,
	// as the two are bundled up for ease of development.
	androidTarget, err := profiles.NewTarget("arm-android")
	if err != nil {
		return err
	}
	if err := profiles.EnsureProfileTargetIsInstalled(jirix, "android", root, androidTarget); err != nil {
		return err
	}

	// Merge the environments using those in the target as the base
	// with those from the base profile and then the java ones
	// we want to set here.
	env := envvar.VarsFromSlice(target.Env.Vars)
	javaProfileEnv := []string{
		fmt.Sprintf("CGO_CFLAGS=-I%s -I%s", filepath.Join(javaHome, "include"),
			filepath.Join(javaHome, "include", target.OS())),
		"JAVA_HOME=" + javaHome,
	}

	baseProfileEnv := profiles.EnvFromProfile(baseTarget, "base")
	profiles.MergeEnv(profiles.ProfileMergePolicies(), env, baseProfileEnv, javaProfileEnv)
	target.Env.Vars = env.ToSlice()
	target.InstallationDir = javaHome
	profiles.InstallProfile(profileName, string(m.javaRoot))
	return profiles.AddProfileTarget(profileName, target)
}

func (m *Manager) Uninstall(jirix *jiri.X, root jiri.RelPath, target profiles.Target) error {
	if err := m.initForTarget(root, target); err != nil {
		return err
	}
	if err := jirix.NewSeq().RemoveAll(m.javaRoot.Abs(jirix)).Done(); err != nil {
		return err
	}
	profiles.RemoveProfileTarget(profileName, target)
	return nil
}

func (m *Manager) install(jirix *jiri.X, target profiles.Target) (string, error) {
	switch target.OS() {
	case "darwin":
		if err := profiles.InstallPackages(jirix, []string{"gradle"}); err != nil {
			return "", err
		}
		javaHome, err := getJDKDarwin(jirix, m.spec)
		if err == nil {
			return javaHome, nil
		}
		fmt.Fprintf(os.Stderr, "Couldn't find an existing Java installation: %v", err)

		// Prompt the user to install JDK.
		// (Note that JDK cannot be installed via Homebrew.)
		javaHomeBin := "/usr/libexec/java_home"
		jirix.NewSeq().Last(javaHomeBin, "-t", "CommandLine", "--request")
		return "", fmt.Errorf("Please follow the OS X prompt instructions to install JDK, then set JAVA_HOME and re-run the profile installation command.")
	case "linux":
		if err := profiles.InstallPackages(jirix, []string{"gradle"}); err != nil {
			return "", err
		}
		javaHome, err := getJDKLinux(jirix, m.spec)
		if err == nil {
			return javaHome, nil
		}
		fmt.Fprintf(os.Stderr, "Couldn't find an existing Java installation: %v", err)

		// Prompt the user to install JDK.
		// (Note that Oracle JDKs cannot be installed via apt-get.)
		dlURL := "http://www.oracle.com/technetwork/java/javase/downloads/index.html"
		jirix.NewSeq().Last("xdg-open", dlURL)
		return "", fmt.Errorf("Please follow the instructions in the browser to install JDK, then set JAVA_HOME and re-run the profile installation command")
	default:
		return "", fmt.Errorf("OS %q is not supported", target.OS)
	}
}

func checkInstall(jirix *jiri.X, home, version string) error {
	s := jirix.NewSeq()
	if _, err := s.Stat(filepath.Join(home, "include", "jni.h")); err != nil {
		return err
	}
	var out bytes.Buffer
	javacPath := filepath.Join(home, "bin", "javac")
	if err := s.Capture(&out, &out).Last(javacPath, "-version"); err != nil {
		return err
	}
	if out.Len() == 0 {
		return errors.New("couldn't find a valid javac at: " + javacPath)
	}
	javacVersion := strings.TrimPrefix(out.String(), "javac ")
	if !strings.HasPrefix(javacVersion, strings.TrimSuffix(version, "+")) {
		return fmt.Errorf("want javac version %v, got %v from output %v.", version, javacVersion, out.String())
	}
	return nil
}

func getJDKLinux(jirix *jiri.X, spec versionSpec) (string, error) {
	if javaHome := os.Getenv("JAVA_HOME"); len(javaHome) > 0 {
		err := checkInstall(jirix, javaHome, spec.jdkVersion)
		if err == nil {
			return javaHome, nil
		}
		fmt.Fprintf(os.Stderr, "JAVA_HOME (%s) is incompatible with required profile version: %v; trying to find a compatible system installation.", javaHome, err)
	}
	// JAVA_HOME doesn't point to the right version: check the system installation.
	javacBin := "/usr/bin/javac"
	var out bytes.Buffer
	if err := jirix.NewSeq().Capture(&out, &out).Last("readlink", "-f", javacBin); err != nil {
		return "", err
	}
	if out.Len() == 0 {
		return "", errors.New("No Java installed under /usr/bin/javac")
	}
	// Strip "/bin/javac" from the returned path.
	javaHome := strings.TrimSuffix(out.String(), "/bin/javac\n")
	if err := checkInstall(jirix, javaHome, spec.jdkVersion); err != nil {
		return "", errors.New("Java installed in /usr/bin/javac is incompatible with profile version: " + spec.jdkVersion)
	}
	return javaHome, nil
}

func getJDKDarwin(jirix *jiri.X, spec versionSpec) (string, error) {
	if javaHome := os.Getenv("JAVA_HOME"); len(javaHome) > 0 {
		err := checkInstall(jirix, javaHome, spec.jdkVersion)
		if err == nil {
			return javaHome, nil
		}
		fmt.Fprintf(os.Stderr, "JAVA_HOME (%s) is incompatible with required profile version %v; trying to find a compatible system installation", javaHome, err)
	}
	// JAVA_HOME doesn't point to the right version: check the system installation.
	javaHomeBin := "/usr/libexec/java_home"
	var out bytes.Buffer
	if err := jirix.NewSeq().Capture(&out, &out).Last(javaHomeBin, "-t", "CommandLine", "-v", spec.jdkVersion); err != nil {
		return "", err
	}
	if out.Len() == 0 {
		return "", errors.New("Couldn't find a valid Java system installation.")
	}
	jdkLoc, _, err := bufio.NewReader(strings.NewReader(out.String())).ReadLine()
	if err != nil {
		return "", fmt.Errorf("Couldn't find a valid Java system installation: %v", err)
	}
	if err := checkInstall(jirix, string(jdkLoc), spec.jdkVersion); err != nil {
		return "", fmt.Errorf("Java system installation is incompatible with profile version %s: %v", spec.jdkVersion, err)
	}
	return string(jdkLoc), nil
}
