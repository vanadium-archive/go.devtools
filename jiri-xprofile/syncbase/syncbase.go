// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syncbase

import (
	"flag"
	"fmt"
	"path/filepath"
	"runtime"

	"v.io/jiri/lib/profiles"
	"v.io/jiri/lib/tool"
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
	m.snappySrcDir = filepath.Join(m.root, "profiles", "csrc", "snappy-1.1.2")
	m.leveldbSrcDir = filepath.Join(m.root, "profiles", "csrc", "leveldb")

}

func (m *Manager) AddFlags(flags *flag.FlagSet, action profiles.Action) {
}

func (m *Manager) initForTarget(target profiles.Target) {
	targetDir := profiles.TargetSpecificDirname(target, true)
	m.syncbaseInstRoot = filepath.Join(m.root, "profiles", "cout", targetDir)
	m.snappyInstDir = filepath.Join(m.syncbaseRoot, targetDir, "snappy")
	m.leveldbInstDir = filepath.Join(m.syncbaseRoot, targetDir, "leveldb")
}

func (m *Manager) Install(ctx *tool.Context, target profiles.Target) error {
	target.Version = profileVersion
	m.initForTarget(target)
	if err := m.installDependencies(ctx, target.Arch, target.OS); err != nil {
		return err
	}
	if err := m.installCommon(ctx, target); err != nil {
		return err
	}
	target.InstallationDir = m.syncbaseInstRoot
	profiles.InstallProfile(profileName, m.syncbaseRoot)
	return profiles.AddProfileTarget(profileName, target)
}

func (m *Manager) Uninstall(ctx *tool.Context, target profiles.Target) error {
	target.Version = profileVersion
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
	target.Version = profileVersion
	m.initForTarget(target)
	return profiles.ErrNoIncrementalUpdate
}

func (m *Manager) installDependencies(ctx *tool.Context, arch, OS string) error {
	var pkgs []string
	switch OS {
	case "android":
		if arch != "arm" {
			return fmt.Errorf("Architecture %q is not supported when compiling for Android.", arch)
		}
		androidProfileMgr := profiles.LookupManager("android")
		if androidProfileMgr == nil {
			return fmt.Errorf("no profile available to install android")
		}
		androidProfileMgr.SetRoot(m.root)
		// Default target is totally fine for installing the android profile.
		return androidProfileMgr.Install(ctx, profiles.Target{
			Tag: "native", Arch: runtime.GOARCH, OS: runtime.GOOS})
	case "darwin":
		pkgs = []string{
			"autoconf", "automake", "libtool", "pkg-config",
		}
	case "linux":
		pkgs = []string{
			// libssl-dev is technically not specific to syncbase, it is
			// required for all vanadium on linux/amd64. However, at the
			// time this was added here, "syncbase" was the only "required"
			// profile, so inserting it here to ensure that it is
			// installed.
			// TODO(ashankar): Figure this out!
			"autoconf", "automake", "g++", "g++-multilib", "gcc-multilib", "libtool", "libssl-dev", "pkg-config",
		}
	default:
		return fmt.Errorf("%q is not supported", OS)
	}
	return profiles.InstallPackages(ctx, pkgs)
}

// installSyncbaseCommon installs the syncbase profile.
func (m *Manager) installCommon(ctx *tool.Context, target profiles.Target) (e error) {

	androidProfile := profiles.LookupProfile("android")
	androidRoot := androidProfile.Root

	// Build and install Snappy.
	installSnappyFn := func() error {
		if err := ctx.Run().Chdir(m.snappySrcDir); err != nil {
			return err
		}
		if err := profiles.RunCommand(ctx, "autoreconf", []string{"--install", "--force", "--verbose"}, nil); err != nil {
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
			args = append(args,
				"--host=arm-linux-androidabi",
				"--target=arm-linux-androidabi",
			)

			ndkRoot := filepath.Join(androidRoot, "ndk-toolchain")
			env["CC"] = filepath.Join(ndkRoot, "bin", "arm-linux-androideabi-gcc")
			env["CXX"] = filepath.Join(ndkRoot, "bin", "arm-linux-androideabi-g++")
			env["AR"] = filepath.Join(ndkRoot, "bin", "arm-linux-androideabi-ar")
			env["RANLIB"] = filepath.Join(ndkRoot, "bin", "arm-linux-androideabi-ranlib")
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
		if err := profiles.RunCommand(ctx, "./configure", args, env); err != nil {
			return err
		}
		if err := profiles.RunCommand(ctx, "make", []string{fmt.Sprintf("-j%d", runtime.NumCPU())}, nil); err != nil {
			return err
		}
		if err := profiles.RunCommand(ctx, "make", []string{"install"}, nil); err != nil {
			return err
		}
		if err := profiles.RunCommand(ctx, "make", []string{"distclean"}, nil); err != nil {
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
		if err := profiles.RunCommand(ctx, "mkdir", []string{"-p", m.leveldbInstDir}, nil); err != nil {
			return err
		}
		leveldbIncludeDir := filepath.Join(m.leveldbInstDir, "include")
		if err := profiles.RunCommand(ctx, "cp", []string{"-R", "include", leveldbIncludeDir}, nil); err != nil {
			return err
		}
		leveldbLibDir := filepath.Join(m.leveldbInstDir, "lib")
		if err := profiles.RunCommand(ctx, "mkdir", []string{leveldbLibDir}, nil); err != nil {
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
			ndkRoot := filepath.Join(androidRoot, "ndk-toolchain")
			env["CC"] = filepath.Join(ndkRoot, "bin", "arm-linux-androideabi-gcc")
			env["CXX"] = filepath.Join(ndkRoot, "bin", "arm-linux-androideabi-g++")
			env["TARGET_OS"] = "OS_ANDROID_CROSSCOMPILE"
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
		if err := profiles.RunCommand(ctx, "make", []string{"clean"}, env); err != nil {
			return err
		}
		if err := profiles.RunCommand(ctx, "make", []string{"static"}, env); err != nil {
			return err
		}
		return nil
	}
	if err := profiles.AtomicAction(ctx, installLeveldbFn, m.leveldbInstDir, "Build and install LevelDB"); err != nil {
		return err
	}
	return nil
}
