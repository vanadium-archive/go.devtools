// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syncbase_profile

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"v.io/jiri"
	"v.io/jiri/profiles"
	"v.io/jiri/profiles/profilesmanager"
	"v.io/jiri/profiles/profilesreader"
	"v.io/jiri/profiles/profilesutil"
	"v.io/jiri/runutil"
	"v.io/x/lib/envvar"
)

const (
	profileVersion = "1"
)

func Register(installer, profile string) {
	m := &Manager{
		profileInstaller: installer,
		profileName:      profile,
		qualifiedName:    profiles.QualifiedProfileName(installer, profile),
		versionInfo: profiles.NewVersionInfo(profile, map[string]interface{}{
			"1": "1",
			"2": "2",
		}, "2"),
	}
	profilesmanager.Register(m)
}

type Manager struct {
	profileInstaller, profileName, qualifiedName string
	syncbaseRoot, syncbaseInstRoot               jiri.RelPath
	snappySrcDir, leveldbSrcDir                  jiri.RelPath
	snappyInstDir, leveldbInstDir                jiri.RelPath
	versionInfo                                  *profiles.VersionInfo
}

func (m Manager) Name() string {
	return m.profileName
}

func (m Manager) Installer() string {
	return m.profileInstaller
}

func (m Manager) String() string {
	return fmt.Sprintf("%s[%s]", m.qualifiedName, m.versionInfo.Default())
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

func (m *Manager) Install(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	m.initForTarget(jirix, root, target)
	if err := m.installDependencies(jirix, target.Arch(), target.OS()); err != nil {
		return err
	}
	if err := m.installCommon(jirix, pdb, root, target); err != nil {
		return err
	}
	env := envvar.VarsFromSlice(target.Env.Vars)
	syncbaseEnv, err := m.syncbaseEnv(jirix, target)
	if err != nil {
		return err
	}
	profilesreader.MergeEnv(profilesreader.ProfileMergePolicies(), env, syncbaseEnv)
	target.Env.Vars = env.ToSlice()
	target.InstallationDir = string(m.syncbaseInstRoot)
	pdb.InstallProfile(m.profileInstaller, m.profileName, string(m.syncbaseRoot))
	return pdb.AddProfileTarget(m.profileInstaller, m.profileName, target)
}

func (m *Manager) Uninstall(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	m.initForTarget(jirix, root, target)
	if err := jirix.NewSeq().
		RemoveAll(m.snappyInstDir.Abs(jirix)).
		RemoveAll(m.leveldbInstDir.Abs(jirix)).Done(); err != nil {
		return err
	}
	pdb.RemoveProfileTarget(m.profileInstaller, m.profileName, target)
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
	return profilesutil.InstallPackages(jirix, pkgs)
}

// initXCC sets the environment variables in 'env' for use with cross-compilers.
func (m *Manager) initXCC(env map[string]string, pdb *profiles.DB, target profiles.Target) error {
	target.SetVersion("")
	goProfile := pdb.LookupProfileTarget(m.profileInstaller, "go", target)
	if goProfile == nil {
		return fmt.Errorf("go profile is not installed for %s", target)
	}
	goEnv := envvar.VarsFromSlice(goProfile.Env.Vars)
	// TODO(ashankar): Change the go profile installation so it sets CC and CXX appropriately.
	env["CC"] = goEnv.Get("CC_FOR_TARGET")
	env["CXX"] = goEnv.Get("CXX_FOR_TARGET")
	return nil
}

func (m *Manager) initClangEnv(jirix *jiri.X, pdb *profiles.DB, target profiles.Target) (map[string]string, error) {
	target.SetVersion("")
	goProfile := pdb.LookupProfileTarget(m.profileInstaller, m.profileName, target)
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

func ndkArch(goArch string) (string, error) {
	switch goArch {
	case "386":
		return "x86", nil
	case "amd64":
		return "x86_64", nil
	case "arm":
		return "arm", nil
	default:
		return "", fmt.Errorf("NDK unsupported for GOARCH %s", goArch)
	}
}

// iosSDKName determines if we are using the simulator or device SDK for a given target architecture.
func iosSDKName(goArch string) (string, error) {
	switch goArch {
	case "386", "amd64":
		return "iphonesimulator", nil
	case "arm", "arm64":
		return "iphoneos", nil
	default:
		return "", fmt.Errorf("Unsupported architecture for iOS: %v", goArch)
	}
}

// iosSDKPath asks the system for the path to a given autodetected iOS SDK (device or simulator).
func iosSDKPath(jirix *jiri.X, target profiles.Target) (string, error) {
	sdk, err := iosSDKName(target.Arch())
	if err != nil {
		return "", err
	}

	var out bytes.Buffer
	outWriter := bufio.NewWriter(&out)
	s := jirix.NewSeq()
	if err := s.Capture(outWriter, outWriter).Last("xcrun", "--sdk", sdk, "--show-sdk-path"); err != nil {
		return "", fmt.Errorf("Unable to get iOS SDK path from xcrun: %s", out.String())
	}
	outWriter.Flush()
	return strings.TrimSpace(out.String()), nil
}

// iosToolPath asks the system for the path to a tool like clang for a given auto-detected SDK (device or simulator).
func iosToolPath(jirix *jiri.X, target profiles.Target, tool string) (string, error) {
	sdk, err := iosSDKName(target.Arch())
	if err != nil {
		return "", err
	}

	var out bytes.Buffer
	outWriter := bufio.NewWriter(&out)
	s := jirix.NewSeq()
	if err := s.Capture(outWriter, outWriter).Last("xcrun", "--sdk", sdk, "--find", tool); err != nil {
		return "", fmt.Errorf("Unable to get %s path from xcrun: %s", tool, out.String())
	}
	outWriter.Flush()
	return strings.TrimSpace(out.String()), nil
}

func iosArch(goArch string) (string, error) {
	switch goArch {
	case "arm":
		return "armv7", nil
	case "arm64":
		return "arm64", nil
	case "386":
		return "i386", nil
	case "amd64":
		return "x86_64", nil
	default:
		return "", fmt.Errorf("Unsupported architecture for iOS: %v", goArch)
	}
}

// initIOSEnv sets the appropriate environmental vars based on autodetecting the
// device or simulator environment (from the target architecture) to use the right clang and
// configure it for the iOS SDK. It returns the clang env flags or error.
func initIOSEnv(jirix *jiri.X, target profiles.Target) (map[string]string, error) {
	sdkName, err := iosSDKName(target.Arch())
	if err != nil {
		return nil, err
	}
	sysroot, err := iosSDKPath(jirix, target)
	if err != nil {
		return nil, err
	}
	clangPath, err := iosToolPath(jirix, target, "clang")
	if err != nil {
		return nil, err
	}
	clangxxPath, err := iosToolPath(jirix, target, "clang++")
	if err != nil {
		return nil, err
	}
	iosArch, err := iosArch(target.Arch())
	if err != nil {
		return nil, err
	}
	// Currently we are setting a deployment target of 8 as it gives us the most APIs and the
	// ability to load shared libraries while achieving a high usage rate (~96% as of Jan 15 2016).
	deploymentTarget := "8.0"
	// TODO(zinman): Enable bitcode via -fembed-bitcode as currently it errors with:
	// ld: -bind_at_load and -bitcode_bundle (Xcode setting ENABLE_BITCODE=YES) cannot be used together
	minVersionEnvFlag := sdkName
	if minVersionEnvFlag == "iphonesimulator" {
		minVersionEnvFlag = "ios-simulator"
	}
	// either -miphoneos-version-min or -mios-simulator-version-min
	iosFlags := fmt.Sprintf("-m%v-version-min=%v -isysroot %v", minVersionEnvFlag, deploymentTarget, sysroot)
	env := map[string]string{
		"IPHONEOS_DEPLOYMENT_TARGET": deploymentTarget,
		"CFLAGS":                     fmt.Sprintf("%v -arch %v", iosFlags, iosArch),
		"CXXFLAGS":                   fmt.Sprintf("%v -arch %v", iosFlags, iosArch),
		"LDFLAGS":                    iosFlags,
		"CC":                         clangPath,
		"CXX":                        clangxxPath,
	}
	if sdkName == "iphoneos" {
		env["TARGET"] = "arm-apple-darwin" // this is true for 32 and 64-bits
	}

	return env, nil
}

// installSyncbaseCommon installs the syncbase profile.
func (m *Manager) installCommon(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) (e error) {
	if target.Arch() != runtime.GOARCH && target.Arch() != "386" && runtime.GOOS != "darwin" {
		// In this special circumstance, old installations of version
		// "1" had a bug - the code was built for the host
		// architecture.  So, uninstall the buggy, unusable V1 in this
		// case.
		if target.Version() != "1" {
			// Check if V1 is installed and if so uninstall it.
			v1 := target
			v1.SetVersion("1")
			if gotV1 := pdb.LookupProfileTarget(m.profileInstaller, m.profileName, v1); gotV1 != nil {
				if jirix.Verbose() {
					fmt.Fprintf(jirix.Stdout(), "Uninstalling bad %v target : %v", m.profileName, gotV1.String())
				}
				if err := m.Uninstall(jirix, pdb, root, *gotV1); err != nil {
					return err
				}
			}
		}
	}
	// Build and install Snappy.
	installSnappyFn := func() error {
		s := jirix.NewSeq()
		snappySrcDir := m.snappySrcDir.Abs(jirix)
		// Ignore errors from make distclean.
		s.Pushd(snappySrcDir).Capture(ioutil.Discard, ioutil.Discard).Last("make", "distclean")
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
		case target.OS() == "android":
			ev := envvar.VarsFromSlice(target.CommandLineEnv().Vars)
			jiri.ExpandEnv(jirix, ev)
			ndk := ev.Get("ANDROID_NDK_DIR")
			if len(ndk) == 0 {
				return fmt.Errorf("ANDROID_NDK_DIR not specified in the command line environment")
			}
			abi, err := androidABI(target.Arch())
			if err != nil {
				return err
			}
			env["CC"] = filepath.Join(ndk, "bin", fmt.Sprintf("%s-gcc", abi))
			env["CXX"] = filepath.Join(ndk, "bin", fmt.Sprintf("%s-g++", abi))
			env["AR"] = filepath.Join(ndk, "bin", fmt.Sprintf("%s-ar", abi))
			env["RANLIB"] = filepath.Join(ndk, "bin", fmt.Sprintf("%s-ranlib", abi))
			args = append(args,
				fmt.Sprintf("--host=%s", abi),
				fmt.Sprintf("--target=%s", abi),
			)
		case target.OS() == "ios":
			clangEnv, err := initIOSEnv(jirix, target)
			if err != nil {
				return err
			}
			env = envvar.MergeMaps(env, clangEnv)
			if target, ok := clangEnv["TARGET"]; ok {
				args = append(args, "--host="+target)
			}
		case target.OS() == "fnl" && target.Arch() == "amd64" && runtime.GOOS == "linux":
			fnlRoot := os.Getenv("FNL_JIRI_ROOT")
			if len(fnlRoot) == 0 {
				return fmt.Errorf("FNL_JIRI_ROOT not specified in the command line environment")
			}
			muslBin := filepath.Join(fnlRoot, "out/root/tools/x86_64-fuchsia-linux-musl/bin")
			env["CC"] = filepath.Join(muslBin, "x86_64-fuchsia-linux-musl-gcc")
			env["CXX"] = filepath.Join(muslBin, "x86_64-fuchsia-linux-musl-g++")
			args = append(args, "--host=amd64-linux")
		case target.OS() == "linux" && target.Arch() == "arm" && runtime.GOOS == "darwin":
			clangEnv, err := m.initClangEnv(jirix, pdb, target)
			if err != nil {
				return err
			}
			env = clangEnv
			args = append(args,
				"--host="+clangEnv["TARGET"],
				"--target="+clangEnv["TARGET"],
			)
		case target.Arch() == "386":
			env["CC"] = "gcc -m32"
			env["CXX"] = "g++ -m32"
		case target.Arch() != runtime.GOARCH:
			if err := m.initXCC(env, pdb, target); err != nil {
				return err
			}
			args = append(args,
				"--host="+target.Arch(),
				"--target="+target.Arch())
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
	if err := profilesutil.AtomicAction(jirix, installSnappyFn, m.snappyInstDir.Abs(jirix), "Build and install Snappy"); err != nil {
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
		switch {
		case target.OS() == "android":
			ev := envvar.VarsFromSlice(target.CommandLineEnv().Vars)
			jiri.ExpandEnv(jirix, ev)
			ndk := ev.Get("ANDROID_NDK_DIR")
			if len(ndk) == 0 {
				return fmt.Errorf("ANDROID_NDK_DIR not specified in the command line environment")
			}
			abi, err := androidABI(target.Arch())
			if err != nil {
				return err
			}
			env["CC"] = filepath.Join(ndk, "bin", fmt.Sprintf("%s-gcc", abi))
			env["CXX"] = filepath.Join(ndk, "bin", fmt.Sprintf("%s-g++", abi))
			env["TARGET_OS"] = "OS_ANDROID_CROSSCOMPILE"
			env["AR"] = filepath.Join(ndk, "bin", fmt.Sprintf("%s-ar", abi))
			env["RANLIB"] = filepath.Join(ndk, "bin", fmt.Sprintf("%s-ranlib", abi))
		case target.OS() == "ios":
			// NOTE(zinman): LevelDB has its own ability to prepare for the iOS platform by setting TARGET_OS,
			// but we still want to use our existing minimum iOS deployment target.
			clangEnv, err := initIOSEnv(jirix, target)
			if err != nil {
				return err
			}
			env["TARGET_OS"] = "IOS"
			env["IPHONEOS_DEPLOYMENT_TARGET"] = clangEnv["IPHONEOS_DEPLOYMENT_TARGET"]
		case target.OS() == "fnl" && target.Arch() == "amd64" && runtime.GOOS == "linux":
			fnlRoot := os.Getenv("FNL_JIRI_ROOT")
			if len(fnlRoot) == 0 {
				return fmt.Errorf("FNL_JIRI_ROOT not specified in the command line environment")
			}
			muslBin := filepath.Join(fnlRoot, "out/root/tools/x86_64-fuchsia-linux-musl/bin")
			env["CC"] = filepath.Join(muslBin, "x86_64-fuchsia-linux-musl-gcc")
			env["CXX"] = filepath.Join(muslBin, "x86_64-fuchsia-linux-musl-g++")
			env["AR"] = filepath.Join(muslBin, "x86_64-fuchsia-linux-musl-ar")
		case target.OS() == "linux" && target.Arch() == "arm" && runtime.GOOS == "darwin":
			clangEnv, err := m.initClangEnv(jirix, pdb, target)
			if err != nil {
				return err
			}
			env["CXXFLAGS"] = "-I" + filepath.Join(m.snappyInstDir.Abs(jirix), "include")
			env["TARGET_OS"] = "Linux"
			env = envvar.MergeMaps(env, clangEnv)
		case target.Arch() == "386":
			env["CC"] = "gcc -m32"
			env["CXX"] = "g++ -m32"
		case target.Arch() != runtime.GOARCH:
			if err := m.initXCC(env, pdb, target); err != nil {
				return err
			}
		}
		if jirix.Verbose() {
			fmt.Fprintf(jirix.Stdout(), "Environment: %s\n", strings.Join(envvar.MapToSlice(env), " "))
		}

		err = s.Run("make", "clean").
			Env(env).Last("make", "static")
		if err != nil {
			return err
		}
		if target.OS() == "ios" {
			// Clean up the iOS binary to trim for this target architecture. The leveldb makefile will be
			// produce VERY fat binaries (i386, x86_64, armv6, armv7, armv7s, arm64). As we eventually combine
			// all our libraries at a future point into a fat binary for distribution, we seek to minimize
			// conflicts by removing unnecessary architectures here at this build juncture.
			// N.B. Apple's "standard architectures" are only armv7 (pre-iPhone 5) and arm64 (future)
			// at this point (Jan 15 2016). iOS 8, our current minimum, runs on armv7.
			leveldbStaticLibPath := filepath.Join(leveldbLibDir, "libleveldb.a")
			targetIosArch, err := iosArch(target.Arch())
			if err != nil {
				return err
			}
			if err := s.Last("lipo", leveldbStaticLibPath, "-output", leveldbStaticLibPath, "-thin", targetIosArch); err != nil {
				return err
			}
		}
		return nil
	}
	if err := profilesutil.AtomicAction(jirix, installLeveldbFn, m.leveldbInstDir.Abs(jirix), "Build and install LevelDB"); err != nil {
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
