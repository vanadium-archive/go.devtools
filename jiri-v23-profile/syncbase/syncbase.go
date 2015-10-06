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

	"v.io/jiri/profiles"
	"v.io/jiri/tool"
	"v.io/x/lib/envvar"
)

const (
	profileName    = "syncbase"
	profileVersion = "1"
)

func init() {
	profiles.Register(profileName, &Manager{})
}

type Manager struct {
	root                           string
	syncbaseRoot, syncbaseInstRoot string
	snappySrcDir, leveldbSrcDir    string
	snappyInstDir, leveldbInstDir  string
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
	m.syncbaseRoot = filepath.Join(m.root, "profiles", "cout")
	m.snappySrcDir = filepath.Join(m.root, "third_party", "csrc", "snappy-1.1.2")
	m.leveldbSrcDir = filepath.Join(m.root, "third_party", "csrc", "leveldb")

}

func (m *Manager) AddFlags(flags *flag.FlagSet, action profiles.Action) {
}

func (m *Manager) initForTarget(target profiles.Target) {
	targetDir := profiles.TargetSpecificDirname(target, true)
	m.syncbaseInstRoot = filepath.Join(m.root, "profiles", "cout", targetDir)
	m.snappyInstDir = filepath.Join(m.syncbaseRoot, targetDir, "snappy")
	m.leveldbInstDir = filepath.Join(m.syncbaseRoot, targetDir, "leveldb")
}

// setSyncbaseEnv adds the LevelDB third-party C++ libraries Vanadium
// Go code depends on to the CGO_CFLAGS and CGO_LDFLAGS variables.
func (m *Manager) setSyncbaseEnv(ctx *tool.Context, env *envvar.Vars, target profiles.Target) error {
	for _, dir := range []string{
		m.leveldbInstDir,
		m.snappyInstDir,
	} {
		cflags := env.GetTokens("CGO_CFLAGS", " ")
		cxxflags := env.GetTokens("CGO_CXXFLAGS", " ")
		ldflags := env.GetTokens("CGO_LDFLAGS", " ")
		if _, err := ctx.Run().Stat(dir); err != nil {
			if !os.IsNotExist(err) {
				return err
			}
			continue
		}
		cflags = append(cflags, filepath.Join("-I"+dir, "include"))
		cxxflags = append(cxxflags, filepath.Join("-I"+dir, "include"))
		ldflags = append(ldflags, filepath.Join("-L"+dir, "lib"))
		if target.Arch == "linux" {
			ldflags = append(ldflags, "-Wl,-rpath", filepath.Join(dir, "lib"))
		}
		env.SetTokens("CGO_CFLAGS", cflags, " ")
		env.SetTokens("CGO_CXXFLAGS", cxxflags, " ")
		env.SetTokens("CGO_LDFLAGS", ldflags, " ")
	}
	return nil
}

func (m *Manager) Install(ctx *tool.Context, target profiles.Target) error {
	m.initForTarget(target)
	if err := m.installDependencies(ctx, target.Arch, target.OS); err != nil {
		return err
	}
	if err := m.installCommon(ctx, target); err != nil {
		return err
	}
	target.InstallationDir = m.syncbaseInstRoot
	env := envvar.VarsFromSlice(target.Env.Vars)
	if err := m.setSyncbaseEnv(ctx, env, target); err != nil {
		return err
	}
	target.Env.Vars = env.ToSlice()
	profiles.InstallProfile(profileName, m.syncbaseRoot)
	if len(target.Version) == 0 {
		target.Version = profileVersion
	}
	return profiles.AddProfileTarget(profileName, target)
}

func (m *Manager) Uninstall(ctx *tool.Context, target profiles.Target) error {
	m.initForTarget(target)
	if err := ctx.Run().RemoveAll(m.snappyInstDir); err != nil {
		return err
	}
	if err := ctx.Run().RemoveAll(m.leveldbInstDir); err != nil {
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

func (m *Manager) installDependencies(ctx *tool.Context, arch, OS string) error {
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
	return profiles.InstallPackages(ctx, pkgs)
}

func getAndroidRoot() (string, error) {
	androidProfile := profiles.LookupProfile("android")
	if androidProfile == nil {
		return "", fmt.Errorf("android profile is not installed")
	}
	return androidProfile.Root, nil
}

// installSyncbaseCommon installs the syncbase profile.
func (m *Manager) installCommon(ctx *tool.Context, target profiles.Target) (e error) {
	// Build and install Snappy.
	installSnappyFn := func() error {
		if err := ctx.Run().Chdir(m.snappySrcDir); err != nil {
			return err
		}
		if err := profiles.RunCommand(ctx, nil, "autoreconf", "--install", "--force", "--verbose"); err != nil {
			return err
		}
		args := []string{
			fmt.Sprintf("--prefix=%v", m.snappyInstDir),
			"--enable-shared=false",
		}
		env := map[string]string{
			// NOTE(nlacasse): The -fPIC flag is needed to compile Syncbase Mojo service.
			"CXXFLAGS": " -fPIC",
		}
		if target.Arch == "386" {
			env["CC"] = "gcc -m32"
			env["CXX"] = "g++ -m32"
		} else if target.Arch == "arm" && target.OS == "android" {
			androidRoot, err := getAndroidRoot()
			if err != nil {
				return err
			}
			args = append(args,
				"--host=arm-linux-androidabi",
				"--target=arm-linux-androidabi",
			)
			ndkRoot := filepath.Join(androidRoot, "ndk-toolchain")
			env["CC"] = filepath.Join(ndkRoot, "bin", "arm-linux-androideabi-gcc")
			env["CXX"] = filepath.Join(ndkRoot, "bin", "arm-linux-androideabi-g++")
			env["AR"] = filepath.Join(ndkRoot, "arm-linux-androideabi", "bin", "ar")
			env["RANLIB"] = filepath.Join(ndkRoot, "arm-linux-androideabi", "bin", "ranlib")
		} else if target.Arch == "arm" && runtime.GOOS == "darwin" && target.OS == "linux" {
			return fmt.Errorf("darwin -> arm-linux cross compilation not yet supported.")
			/*
			   export CC=/Volumes/code2/llvm/bin/cc-arm-raspian
			   export CXX=/Volumes/code2/llvm/bin/cxx-arm-raspian
			   export LDFLAGS=-lm
			   export AR=/Volumes/code2/llvm/install/binutils/bin/ar
			   export RANLIB=/Volumes/code2/llvm/install/binutils/bin/ranlib
			   ./configure --prefix=$(pwd)/../../cout/linux_arm/snappy --enable-shared=false \
			           --host=arm-linux-gnueabi
			*/
		}
		if err := profiles.RunCommand(ctx, env, "./configure", args...); err != nil {
			return err
		}
		if err := profiles.RunCommand(ctx, nil, "make", "clean"); err != nil {
			return err
		}
		if err := profiles.RunCommand(ctx, nil, "make", fmt.Sprintf("-j%d", runtime.NumCPU())); err != nil {
			return err
		}
		if err := profiles.RunCommand(ctx, nil, "make", "install"); err != nil {
			return err
		}
		if err := profiles.RunCommand(ctx, nil, "make", "distclean"); err != nil {
			return err
		}
		return nil
	}
	if err := profiles.AtomicAction(ctx, installSnappyFn, m.snappyInstDir, "Build and install Snappy"); err != nil {
		return err
	}

	// Build and install LevelDB.
	installLeveldbFn := func() error {
		if err := ctx.Run().Chdir(m.leveldbSrcDir); err != nil {
			return err
		}
		if err := profiles.RunCommand(ctx, nil, "mkdir", "-p", m.leveldbInstDir); err != nil {
			return err
		}
		leveldbIncludeDir := filepath.Join(m.leveldbInstDir, "include")
		if err := profiles.RunCommand(ctx, nil, "cp", "-R", "include", leveldbIncludeDir); err != nil {
			return err
		}
		leveldbLibDir := filepath.Join(m.leveldbInstDir, "lib")
		if err := profiles.RunCommand(ctx, nil, "mkdir", leveldbLibDir); err != nil {
			return err
		}
		env := map[string]string{
			"PREFIX": leveldbLibDir,
			// NOTE(nlacasse): The -fPIC flag is needed to compile Syncbase Mojo service.
			"CXXFLAGS": "-I" + filepath.Join(m.snappyInstDir, "include") + " -fPIC",
			"LDFLAGS":  "-L" + filepath.Join(m.snappyInstDir, "lib"),
		}
		if target.Arch == "386" {
			env["CC"] = "gcc -m32"
			env["CXX"] = "g++ -m32"
		} else if target.Arch == "arm" && target.OS == "android" {
			androidRoot, err := getAndroidRoot()
			if err != nil {
				return err
			}
			ndkRoot := filepath.Join(androidRoot, "ndk-toolchain")
			env["CC"] = filepath.Join(ndkRoot, "bin", "arm-linux-androideabi-gcc")
			env["CXX"] = filepath.Join(ndkRoot, "bin", "arm-linux-androideabi-g++")
			env["TARGET_OS"] = "OS_ANDROID_CROSSCOMPILE"
			env["AR"] = filepath.Join(ndkRoot, "arm-linux-androideabi", "bin", "ar")
			env["RANLIB"] = filepath.Join(ndkRoot, "arm-linux-androideabi", "bin", "ranlib")
		} else if target.Arch == "arm" && runtime.GOOS == "darwin" && target.OS == "linux" {
			return fmt.Errorf("darwin -> arm-linux cross compilation not yet supported.")
			/*
				export CC=/Volumes/code2/llvm/bin/cc-arm-raspian
				export CXX=/Volumes/code2/llvm/bin/cxx-arm-raspian
				export TARGET_OS=Linux
				export AR=/Volumes/code2/llvm/install/binutils/bin/ar
				export RANLIB=/Volumes/code2/llvm/install/binutils/bin/ranlib
				INST_DIR=../../cout/linux_arm/leveldb
				mkdir -p $INST_DIR
				mkdir -p $INST_DIR/lib
				mkdir -p $INST_DIR/include
				export PREFIX=../../cout/linux_arm/leveldb/lib
				make static
				cp -r ./include/leveldb ../../cout/linux_arm/leveldb/include
			*/
		}
		if err := profiles.RunCommand(ctx, env, "make", "clean"); err != nil {
			return err
		}
		if err := profiles.RunCommand(ctx, env, "make", "static"); err != nil {
			return err
		}
		return nil
	}
	if err := profiles.AtomicAction(ctx, installLeveldbFn, m.leveldbInstDir, "Build and install LevelDB"); err != nil {
		return err
	}
	return nil
}
