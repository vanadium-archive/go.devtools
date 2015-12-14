// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syncbase

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"v.io/jiri/jiri"
	"v.io/jiri/profiles"
	"v.io/jiri/runutil"
	"v.io/x/lib/envvar"
)

const (
	profileName    = "syncbase"
	profileVersion = "1"
)

func init() {
	m := &Manager{
		versionInfo: profiles.NewVersionInfo(profileName, map[string]interface{}{
			"1": "1",
		}, "1"),
	}
	profiles.Register(profileName, m)
}

type Manager struct {
	syncbaseRoot, syncbaseInstRoot jiri.RelPath
	snappySrcDir, leveldbSrcDir    jiri.RelPath
	snappyInstDir, leveldbInstDir  jiri.RelPath
	versionInfo                    *profiles.VersionInfo
}

func (Manager) Name() string {
	return profileName
}

func (m Manager) String() string {
	return fmt.Sprintf("%s[%s]", profileName, m.versionInfo.Default())
}

func (m Manager) Info() string {
	return `
The syncbase profile provides support for syncbase, in particular the snappy and
leveldb libraries.`
}

func (m Manager) VersionInfo() *profiles.VersionInfo {
	return m.versionInfo
}

func (m *Manager) AddFlags(flags *flag.FlagSet, action profiles.Action) {
}

func (m *Manager) initForTarget(jirix *jiri.X, root jiri.RelPath, target profiles.Target) {
	m.syncbaseRoot = root.Join("cout")
	m.snappySrcDir = jiri.NewRelPath("third_party", "csrc", "snappy-1.1.2")
	m.leveldbSrcDir = jiri.NewRelPath("third_party", "csrc", "leveldb")

	targetDir := target.TargetSpecificDirname()
	m.syncbaseInstRoot = m.syncbaseRoot.Join(targetDir)
	m.snappyInstDir = m.syncbaseInstRoot.Join("snappy")
	m.leveldbInstDir = m.syncbaseInstRoot.Join("leveldb")

	if jirix.Verbose() {
		fmt.Fprintf(jirix.Stdout(), "Installation Directories for: %s\n", target)
		fmt.Fprintf(jirix.Stdout(), "Syncbase installation dir: %s\n", m.syncbaseInstRoot)
		fmt.Fprintf(jirix.Stdout(), "Snappy: %s\n", m.snappyInstDir)
		fmt.Fprintf(jirix.Stdout(), "Leveldb: %s\n", m.leveldbInstDir)
	}
}

// setSyncbaseEnv adds the LevelDB third-party C++ libraries Vanadium
// Go code depends on to the CGO_CFLAGS and CGO_LDFLAGS variables.
func (m *Manager) syncbaseEnv(jirix *jiri.X, target profiles.Target) ([]string, error) {
	env := envvar.VarsFromSlice([]string{})
	for _, dir := range []jiri.RelPath{
		m.leveldbInstDir,
		m.snappyInstDir,
	} {
		cflags := env.GetTokens("CGO_CFLAGS", " ")
		cxxflags := env.GetTokens("CGO_CXXFLAGS", " ")
		ldflags := env.GetTokens("CGO_LDFLAGS", " ")
		if _, err := jirix.NewSeq().Stat(dir.Abs(jirix)); err != nil {
			if !runutil.IsNotExist(err) {
				return nil, err
			}
			continue
		}
		cflags = append(cflags, filepath.Join("-I"+dir.Symbolic(), "include"))
		cxxflags = append(cxxflags, filepath.Join("-I"+dir.Symbolic(), "include"))
		ldflags = append(ldflags, filepath.Join("-L"+dir.Symbolic(), "lib"))
		if target.Arch() == "linux" {
			ldflags = append(ldflags, "-Wl,-rpath", filepath.Join(dir.Symbolic(), "lib"))
		}
		env.SetTokens("CGO_CFLAGS", cflags, " ")
		env.SetTokens("CGO_CXXFLAGS", cxxflags, " ")
		env.SetTokens("CGO_LDFLAGS", ldflags, " ")
	}
	return env.ToSlice(), nil
}

func (m *Manager) Install(jirix *jiri.X, root jiri.RelPath, target profiles.Target) error {
	m.initForTarget(jirix, root, target)
	if err := m.installDependencies(jirix, target.Arch(), target.OS()); err != nil {
		return err
	}
	if err := m.installCommon(jirix, root, target); err != nil {
		return err
	}
	env := envvar.VarsFromSlice(target.Env.Vars)
	syncbaseEnv, err := m.syncbaseEnv(jirix, target)
	if err != nil {
		return err
	}
	profiles.MergeEnv(profiles.ProfileMergePolicies(), env, syncbaseEnv)
	target.Env.Vars = env.ToSlice()
	target.InstallationDir = string(m.syncbaseInstRoot)
	profiles.InstallProfile(profileName, string(m.syncbaseRoot))
	return profiles.AddProfileTarget(profileName, target)
}

func (m *Manager) Uninstall(jirix *jiri.X, root jiri.RelPath, target profiles.Target) error {
	m.initForTarget(jirix, root, target)
	if err := jirix.NewSeq().
		RemoveAll(m.snappyInstDir.Abs(jirix)).
		RemoveAll(m.leveldbInstDir.Abs(jirix)).Done(); err != nil {
		return err
	}
	profiles.RemoveProfileTarget(profileName, target)
	return nil
}

func (m *Manager) installDependencies(jirix *jiri.X, arch, OS string) error {
	var pkgs []string
	switch runtime.GOOS {
	case "darwin":
		pkgs = []string{
			"autoconf", "automake", "libtool", "pkg-config",
		}
	case "linux":
		pkgs = []string{
			"autoconf", "automake", "g++", "g++-multilib", "gcc-multilib", "libtool", "pkg-config",
		}
	default:
		return fmt.Errorf("%q is not supported", runtime.GOOS)
	}
	return profiles.InstallPackages(jirix, pkgs)
}

func getAndroidRoot(root jiri.RelPath) (jiri.RelPath, error) {
	rp := jiri.NewRelPath()
	androidProfile := profiles.LookupProfile("android")
	if androidProfile == nil {
		return rp, fmt.Errorf("android profile is not installed")
	}
	return rp.Join(androidProfile.Root), nil
}

func initClangEnv(jirix *jiri.X, target profiles.Target) (map[string]string, error) {
	target.SetVersion("")
	goProfile := profiles.LookupProfileTarget("go", target)
	if goProfile == nil {
		return nil, fmt.Errorf("go profile is not installed for %s", target)
	}
	goEnv := envvar.VarsFromSlice(goProfile.Env.Vars)
	jiri.ExpandEnv(jirix, goEnv)
	path := envvar.SplitTokens(jirix.Env()["PATH"], ":")
	path = append([]string{goEnv.Get("BINUTILS_BIN")}, path...)
	env := map[string]string{
		"CC":      goEnv.Get("CLANG"),
		"CXX":     goEnv.Get("CLANG++"),
		"LDFLAGS": goEnv.Get("LDFLAGS"),
		"AR":      goEnv.Get("AR"),
		"RANLIB":  goEnv.Get("RANLIB"),
		"PATH":    envvar.JoinTokens(path, ":"),
		"TARGET":  goEnv.Get("TARGET"),
	}
	for k, v := range env {
		if len(v) == 0 {
			return nil, fmt.Errorf("variable %q is not set", k)
		}
	}
	return env, nil
}

// installSyncbaseCommon installs the syncbase profile.
func (m *Manager) installCommon(jirix *jiri.X, root jiri.RelPath, target profiles.Target) (e error) {
	// Build and install Snappy.
	installSnappyFn := func() error {
		s := jirix.NewSeq()
		snappySrcDir := m.snappySrcDir.Abs(jirix)
		// ignore errors from make distclean.
		s.Pushd(snappySrcDir).Last("make", "distclean")
		if err := s.Pushd(snappySrcDir).
			Last("autoreconf", "--install", "--force", "--verbose"); err != nil {
			return err
		}
		args := []string{
			fmt.Sprintf("--prefix=%v", m.snappyInstDir.Abs(jirix)),
			"--enable-shared=false",
		}
		env := map[string]string{
			// NOTE(nlacasse): The -fPIC flag is needed to compile
			// Syncbase Mojo service. This is set here since we don't
			// currently have a specific target. Targets that don't
			// require it should override it.
			"CXXFLAGS": " -fPIC",
		}
		switch {
		case target.Arch() == "386":
			env["CC"] = "gcc -m32"
			env["CXX"] = "g++ -m32"
		case target.OS() == "android":
			androidRoot, err := getAndroidRoot(root)
			if err != nil {
				return err
			}
			ndkRoot := androidRoot.Join(fmt.Sprintf("ndk-toolchain-%s", runtime.GOARCH))
			abi, err := androidABI(target.Arch())
			if err != nil {
				return err
			}
			args = append(args,
				fmt.Sprintf("--host=%s", abi),
				fmt.Sprintf("--target=%s", abi),
			)
			env["CC"] = ndkRoot.Join("bin", fmt.Sprintf("%s-gcc", abi)).Abs(jirix)
			env["CXX"] = ndkRoot.Join("bin", fmt.Sprintf("%s-g++", abi)).Abs(jirix)
			env["AR"] = ndkRoot.Join(abi, "bin", "ar").Abs(jirix)
			env["RANLIB"] = ndkRoot.Join(abi, "bin", "ranlib").Abs(jirix)
		case target.Arch() == "amd64" && runtime.GOOS == "linux" && target.OS() == "fnl":
			fnlRoot := os.Getenv("FNL_JIRI_ROOT")
			if len(fnlRoot) == 0 {
				return fmt.Errorf("FNL_JIRI_ROOT not specified in the command line environment")
			}
			muslBin := filepath.Join(fnlRoot, "out/root/tools/x86_64-fuchsia-linux-musl/bin")
			env["CC"] = filepath.Join(muslBin, "x86_64-fuchsia-linux-musl-gcc")
			env["CXX"] = filepath.Join(muslBin, "x86_64-fuchsia-linux-musl-g++")
			args = append(args, "--host=amd64-linux")
		case target.Arch() == "arm" && runtime.GOOS == "darwin" && target.OS() == "linux":
			clangEnv, err := initClangEnv(jirix, target)
			if err != nil {
				return err
			}
			env = clangEnv
			args = append(args,
				"--host="+clangEnv["TARGET"],
				"--target="+clangEnv["TARGET"],
			)
		}
		if jirix.Verbose() {
			fmt.Fprintf(jirix.Stdout(), "Environment: %s\n", strings.Join(envvar.MapToSlice(env), " "))
		}
		return s.Pushd(snappySrcDir).
			Env(env).Run("./configure", args...).
			Run("make", "clean").
			Env(env).Run("make", fmt.Sprintf("-j%d", runtime.NumCPU())).
			Env(env).Run("make", "install").
			Last("make", "distclean")
	}
	if err := profiles.AtomicAction(jirix, installSnappyFn, m.snappyInstDir.Abs(jirix), "Build and install Snappy"); err != nil {
		return err
	}

	// Build and install LevelDB.
	installLeveldbFn := func() error {
		leveldbIncludeDir := m.leveldbInstDir.Join("include").Abs(jirix)
		leveldbLibDir := m.leveldbInstDir.Join("lib").Abs(jirix)

		s := jirix.NewSeq()
		err := s.Chdir(m.leveldbSrcDir.Abs(jirix)).
			Run("mkdir", "-p", m.leveldbInstDir.Abs(jirix)).
			Run("cp", "-R", "include", leveldbIncludeDir).
			Last("mkdir", leveldbLibDir)
		if err != nil {
			return err
		}

		env := map[string]string{
			"PREFIX": leveldbLibDir,
			// NOTE(nlacasse): The -fPIC flag is needed to compile Syncbase Mojo service.
			"CXXFLAGS": "-I" + filepath.Join(m.snappyInstDir.Abs(jirix), "include") + " -fPIC",
			"LDFLAGS":  "-L" + filepath.Join(m.snappyInstDir.Abs(jirix), "lib"),
		}
		if target.Arch() == "386" {
			env["CC"] = "gcc -m32"
			env["CXX"] = "g++ -m32"
		} else if target.OS() == "android" {
			androidRoot, err := getAndroidRoot(root)
			if err != nil {
				return err
			}
			ndkRoot := androidRoot.Join(fmt.Sprintf("ndk-toolchain-%s", runtime.GOARCH))
			abi, err := androidABI(target.Arch())
			if err != nil {
				return err
			}
			env["CC"] = ndkRoot.Join("bin", fmt.Sprintf("%s-gcc", abi)).Abs(jirix)
			env["CXX"] = ndkRoot.Join("bin", fmt.Sprintf("%s-g++", abi)).Abs(jirix)
			env["TARGET_OS"] = "OS_ANDROID_CROSSCOMPILE"
			env["AR"] = ndkRoot.Join(abi, "bin", "ar").Abs(jirix)
			env["RANLIB"] = ndkRoot.Join(abi, "bin", "ranlib").Abs(jirix)
		} else if target.Arch() == "amd64" && runtime.GOOS == "linux" && target.OS() == "fnl" {
			fnlRoot := os.Getenv("FNL_JIRI_ROOT")
			if len(fnlRoot) == 0 {
				return fmt.Errorf("FNL_JIRI_ROOT not specified in the command line environment")
			}
			muslBin := filepath.Join(fnlRoot, "out/root/tools/x86_64-fuchsia-linux-musl/bin")
			env["CC"] = filepath.Join(muslBin, "x86_64-fuchsia-linux-musl-gcc")
			env["CXX"] = filepath.Join(muslBin, "x86_64-fuchsia-linux-musl-g++")
			env["AR"] = filepath.Join(muslBin, "x86_64-fuchsia-linux-musl-ar")
		} else if target.Arch() == "arm" && runtime.GOOS == "darwin" && target.OS() == "linux" {
			clangEnv, err := initClangEnv(jirix, target)
			if err != nil {
				return err
			}
			env["CXXFLAGS"] = "-I" + filepath.Join(m.snappyInstDir.Abs(jirix), "include")
			env["TARGET_OS"] = "Linux"
			env = envvar.MergeMaps(env, clangEnv)
		}
		if jirix.Verbose() {
			fmt.Fprintf(jirix.Stdout(), "Environment: %s\n", strings.Join(envvar.MapToSlice(env), " "))
		}
		return s.Run("make", "clean").
			Env(env).Last("make", "static")
	}
	if err := profiles.AtomicAction(jirix, installLeveldbFn, m.leveldbInstDir.Abs(jirix), "Build and install LevelDB"); err != nil {
		return err
	}
	return nil
}

func androidABI(targetArch string) (string, error) {
	switch targetArch {
	case "amd64":
		return "x86_64-linux-android", nil
	case "arm":
		return "arm-linux-androideabi", nil
	default:
		return "", fmt.Errorf("could not locate android abi for target arch %s", targetArch)
	}
}
