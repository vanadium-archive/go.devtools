// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package go_profile

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"v.io/jiri"
	"v.io/jiri/collect"
	"v.io/jiri/gitutil"
	"v.io/jiri/profiles"
	"v.io/jiri/profiles/profilesmanager"
	"v.io/jiri/profiles/profilesutil"
	"v.io/jiri/project"
	"v.io/x/lib/envvar"
)

const (
	go15GitRemote = "https://github.com/golang/go.git"
)

var (
	goSysrootFlag        = ""
	goSysrootDirs        = ""
	defaultGoSysrootDirs = "/lib:/usr/lib:/usr/include"
)

// Supported cross compilation toolchains.
type xspec struct{ arch, os string }
type xbuilder func(*jiri.X, *Manager, jiri.RelPath, profiles.Target, profiles.Action) (bindir string, env []string, e error)

var xcompilers = map[xspec]map[xspec]xbuilder{
	xspec{"amd64", "darwin"}: {
		xspec{"amd64", "linux"}:   darwin_to_linux,
		xspec{"arm", "linux"}:     darwin_to_linux,
		xspec{"arm", "android"}:   to_android,
		xspec{"amd64", "android"}: to_android,
		xspec{"arm", "ios"}:       darwin_to_ios,
		xspec{"arm64", "ios"}:     darwin_to_ios,
		xspec{"386", "ios"}:       darwin_to_ios, // iOS simulator
		xspec{"amd64", "ios"}:     darwin_to_ios, // iOS simulator
	},
	xspec{"amd64", "linux"}: {
		xspec{"amd64", "fnl"}:     to_fnl,
		xspec{"arm", "linux"}:     linux_to_linux,
		xspec{"arm", "android"}:   to_android,
		xspec{"amd64", "android"}: to_android,
	},
}

type versionSpec struct {
	gitRevision string
	patchFiles  []string
}

// goRelease enables installation of specific release versions of the Go toolchain.
type goRelease struct {
	file   string
	sha256 string
}

func newGoRelease(version string) *goRelease {
	arch := runtime.GOARCH
	if arch == "arm" {
		arch = "armv6l"
	}
	file := fmt.Sprintf("go%s.%s-%s.tar.gz", version, runtime.GOOS, arch)
	// From: https://golang.org/dl/
	shamap := map[string]string{
		"go1.6.darwin-amd64.tar.gz":  "8b686ace24c0166738fd9f6003503f9d55ce03b7f24c963b043ba7bb56f43000",
		"go1.6.freebsd-386.tar.gz":   "67f0278e0650b303156adbfe012317b9ce75396e3a28cbc0a8210284bb07ab85",
		"go1.6.freebsd-amd64.tar.gz": "3763015cdc7971e10f90fb5bec80d885e9956f836277dcb35a2166ffbd7af9b5",
		"go1.6.linux-386.tar.gz":     "7a240a0f45e559d47ea07319d9faf838225eb9e18174f56a76ccaf9860dbb9b1",
		"go1.6.linux-amd64.tar.gz":   "5470eac05d273c74ff8bac7bef5bad0b5abbd1c4052efbdbc8db45332e836b0b",
		"go1.6.linux-armv6l.tar.gz":  "c6c1859acd3727f23f900bde855b5fd0f74d36b1d10f6dd7beddebfb57513d0b",
		"go1.6.windows-386.zip":      "ac41a46f44d0ea5b83ad7e6a55ee1d58c6a01b7ab7342e243f232510342f16f0",
		"go1.6.windows-amd64.zip":    "1be06afa469666d636a00928755c4bcd6403a01f5761946b2b13b8a664f86bac",
	}
	sha256, ok := shamap[file]
	if !ok {
		return nil
	}
	return &goRelease{file, sha256}
}

func (g *goRelease) install(jirix *jiri.X, dir string) error {
	s := jirix.NewSeq()
	tmpDir, err := s.TempDir("", "")
	if err != nil {
		return err
	}
	defer jirix.NewSeq().RemoveAll(tmpDir)
	local := filepath.Join(tmpDir, g.file)
	remote := "https://storage.googleapis.com/golang/" + g.file
	if err := profilesutil.Fetch(jirix, local, remote); err != nil {
		return err
	}
	// Verify the checksum of the downloaded file
	csum := sha256.New()
	file, err := os.Open(local)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := io.Copy(csum, file); err != nil {
		return fmt.Errorf("failed to checksum %v: %v", file, err)
	}
	file.Close()
	if got, want := hex.EncodeToString(csum.Sum(nil)), g.sha256; got != want {
		return fmt.Errorf("checksum mismatch in download of %q. Got %v, want %v", remote, got, want)
	}
	if strings.HasSuffix(local, ".zip") {
		if err := profilesutil.Unzip(jirix, local, tmpDir); err != nil {
			return err
		}
	} else {
		s = s.Run("tar", "-C", tmpDir, "-xzf", local)
	}
	if err := s.Remove(local).
		MkdirAll(filepath.Dir(dir), profilesutil.DefaultDirPerm).
		Rename(filepath.Join(tmpDir, "go"), dir).
		Done(); err != nil {
		return err
	}
	return nil
}

func Register(installer, profile string) {
	m := &Manager{
		profileInstaller: installer,
		profileName:      profile,
		qualifiedName:    profiles.QualifiedProfileName(installer, profile),
		versionInfo: profiles.NewVersionInfo(profile, map[string]interface{}{
			"1.5": &versionSpec{
				"cc6554f750ccaf63bcdcc478b2a60d71ca76d342", nil},
			"1.5.1": &versionSpec{
				"f2e4c8b5fb3660d793b2c545ef207153db0a34b1", nil},
			// 1.5.1 at a specific git revision, create a new version anytime
			// a new profile is checked in.
			"1.5.1.1:2738c5e0": &versionSpec{
				"492a62e945555bbf94a6f9dd6d430f712738c5e0", nil},
			// 1.5.2 with x86_64 android support
			"1.5.2.1:56093743": &versionSpec{
				"560937434d5f2857bb69e0a6881a38201a197a8d", nil},
			"1.6": &versionSpec{
				"e805bf39458915365924228dc53969ce04e32813", nil},
		}, "1.6"),
	}
	profilesmanager.Register(m)
}

type Manager struct {
	profileInstaller, profileName, qualifiedName string
	root, goRoot                                 jiri.RelPath
	versionInfo                                  *profiles.VersionInfo
	spec                                         versionSpec
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
The go profile manages installations of the go compiler and in particular configures
them for cross compilation with cgo. A separate build of each cross-compilation
environment is maintained to simplify use of cgo albeit at the cost of some disk space.`
}

func (m Manager) VersionInfo() *profiles.VersionInfo {
	return m.versionInfo
}

func (m *Manager) AddFlags(flags *flag.FlagSet, action profiles.Action) {
	flags.StringVar(&goSysrootFlag, m.profileName+".sysroot-image", "", "sysroot image for cross compiling to the currently specified target")
	flags.StringVar(&goSysrootDirs, m.profileName+".sysroot-image-dirs-to-use", defaultGoSysrootDirs, "a colon separated list of directories to use from the sysroot image")
}

func (m *Manager) initForTarget(jirix *jiri.X, root jiri.RelPath, target *profiles.Target) error {
	m.root = root
	m.goRoot = root.Join("go")
	if err := m.versionInfo.Lookup(target.Version(), &m.spec); err != nil {
		return err
	}
	if jirix.Verbose() {
		fmt.Fprintf(jirix.Stdout(), "Go Profiles: %s\n", m.goRoot)
	}
	return nil
}

func (m *Manager) Install(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	if err := m.initForTarget(jirix, root, &target); err != nil {
		return err
	}

	jirix.NewSeq().RemoveAll(m.goRoot.Join("go-bootstrap").Abs(jirix))
	cgo := true
	if target.CrossCompiling() {
		// We may need to install an additional cross compilation toolchain
		// for cgo to work.
		if builder := xcompilers[xspec{runtime.GOARCH, runtime.GOOS}][xspec{target.Arch(), target.OS()}]; builder != nil {
			_, vars, err := builder(jirix, m, root, target, profiles.Install)
			if err != nil {
				return err
			}
			target.Env.Vars = vars
		} else if runtime.GOARCH == "amd64" && runtime.GOOS == "linux" && target.Arch() == "386" && target.OS() == "linux" {
			// CGO for 386-linux works on amd64-linux host without cross-compilation.
			cgo = true
		} else {
			// CGO is not supported.
			cgo = false
		}
	}

	// Set GOARCH, GOOS to the values specified in the target, if not set.
	target.Env.Vars = envvar.MergeSlices([]string{
		"GOARCH=" + target.Arch(),
		"GOOS=" + target.OS(),
	}, target.Env.Vars)
	if cgo {
		target.Env.Vars = append(target.Env.Vars, "CGO_ENABLED=1")
	}

	// If a release version of the go toolchain can be used, use that.
	// This allows multiple target architectures to share a single
	// toolchain installation, saving time and space.
	//
	// A release version cannot be used if:
	// (1) Don't know the URL for obtaining it (newGoRelease returns nil), OR
	// (2) Any patches need to be applied to the toolchain code (m.spec.patchFiles), OR
	// (3) Any custom build flags (set via GO_FLAGS environment variable)
	var goInstDir jiri.RelPath
	env := envvar.VarsFromSlice(target.Env.Vars)
	if release := newGoRelease(target.Version()); release != nil &&
		len(m.spec.patchFiles) == 0 &&
		!env.Contains("GO_FLAGS") {
		// Not using a bootstrapped Go, delete any references from env.
		env.Delete("GOROOT_BOOTSTRAP")
		goInstDir = m.goRoot.Join("shared").Join(target.Version())
		fn := func() error { return release.install(jirix, goInstDir.Abs(jirix)) }
		if err := profilesutil.AtomicAction(jirix, fn, goInstDir.Abs(jirix), "Install a release version of the Go toolchain"); err != nil {
			return err
		}
		// CC_FOR_TARGET and CXX_FOR_TARGET might have been set by the
		// cross-compiler setup above. Those variables only take effect
		// when building a Go toolchain.  Instead set CC and CXX.
		if k := "CC_FOR_TARGET"; env.Contains(k) {
			env.Set("CC", abs2symbolic(jirix, env.Get(k)))
			env.Delete(k)
		}
		if k := "CXX_FOR_TARGET"; env.Contains(k) {
			env.Set("CXX", abs2symbolic(jirix, env.Get(k)))
			env.Delete(k)
		}
		if jirix.Verbose() {
			fmt.Fprintf(jirix.Stdout(), "Using Go toolchain in %v for target %v", goInstDir, target)
		}
	} else {
		if jirix.Verbose() {
			fmt.Fprintf(jirix.Stdout(), "Compiling go toolchain at revision %v", m.spec.gitRevision)
		}
		// Compile our own Go toolchain
		targetDir := m.goRoot.Join(target.TargetSpecificDirname())
		goInstDir = targetDir.Join(m.spec.gitRevision)
		goBootstrapDir := targetDir.Join("go-bootstrap")
		if jirix.Verbose() {
			fmt.Fprintf(jirix.Stdout(), "Go Target+Revision %s\n", goInstDir.Abs(jirix))
		}
		if err := m.installGo15Plus(jirix, goBootstrapDir, goInstDir, target.Version(), env); err != nil {
			return err
		}
	}
	env.Set("GOROOT", goInstDir.Symbolic())
	target.Env.Vars = env.ToSlice()
	target.InstallationDir = string(goInstDir)

	pdb.InstallProfile(m.profileInstaller, m.profileName, string(m.goRoot))
	return pdb.AddProfileTarget(m.profileInstaller, m.profileName, target)
}

func (m *Manager) Uninstall(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	if err := m.initForTarget(jirix, root, &target); err != nil {
		return err
	}

	if target.CrossCompiling() {
		// We may need to install an additional cross compilation toolchain
		// for cgo to work.
		def := profiles.DefaultTarget()
		if builder := xcompilers[xspec{def.Arch(), def.OS()}][xspec{target.Arch(), target.OS()}]; builder != nil {
			if _, _, err := builder(jirix, m, root, target, profiles.Uninstall); err != nil {
				return err
			}
		}
	}
	s := jirix.NewSeq()
	if err := s.RemoveAll(m.goRoot.Join(target.TargetSpecificDirname()).Abs(jirix)).Done(); err != nil {
		return err
	}
	if pdb.RemoveProfileTarget(m.profileInstaller, m.profileName, target) {
		// If there are no more targets then remove the entire go directory,
		// including the bootstrap one.
		return s.RemoveAll(m.goRoot.Abs(jirix)).Done()
	}
	return nil
}

// installGo14 installs Go 1.4 at a given location, using the provided
// environment during compilation.
func installGo14(jirix *jiri.X, go14Dir string, env *envvar.Vars) error {
	installGo14Fn := func() error {
		s := jirix.NewSeq()
		tmpDir, err := s.TempDir("", "")
		if err != nil {
			return err
		}
		defer jirix.NewSeq().RemoveAll(tmpDir)

		name := "go1.4.3.src.tar.gz"
		remote, local := "https://storage.googleapis.com/golang/"+name, filepath.Join(tmpDir, name)
		parentDir := filepath.Dir(go14Dir)
		goSrcDir := filepath.Join(go14Dir, "src")
		makeBin := filepath.Join(goSrcDir, "make.bash")

		if err := s.Run("curl", "-Lo", local, remote).
			Run("tar", "-C", tmpDir, "-xzf", local).
			Remove(local).
			RemoveAll(go14Dir).
			MkdirAll(parentDir, profilesutil.DefaultDirPerm).Done(); err != nil {
			return err
		}
		return s.Rename(filepath.Join(tmpDir, "go"), go14Dir).
			Chdir(goSrcDir).
			Env(env.ToMap()).Last(makeBin, "--no-clean")
	}
	return profilesutil.AtomicAction(jirix, installGo14Fn, go14Dir, "Build and install Go 1.4")
}

// installGo15Plus installs any version of go past 1.5 at the specified git and go
// revision.
func (m *Manager) installGo15Plus(jirix *jiri.X, goBootstrapDir, goInstDir jiri.RelPath, version string, env *envvar.Vars) error {
	// First install bootstrap Go 1.4 for the host.
	if err := installGo14(jirix, goBootstrapDir.Abs(jirix), envvar.VarsFromOS()); err != nil {
		return err
	}
	installGo15Fn := func() error {
		// Clone go1.5 into a tmp directory and then rename it to the
		// final destination.
		s := jirix.NewSeq()
		tmpDir, err := s.TempDir("", "")
		if err != nil {
			return err
		}
		defer jirix.NewSeq().RemoveAll(tmpDir)

		goSrcDir := filepath.Join(goInstDir.Abs(jirix), "src")

		// Git clone the code and get into the right directory.
		if err := s.Pushd(tmpDir).
			Call(func() error { return gitutil.New(jirix.NewSeq()).Clone(go15GitRemote, tmpDir) }, "").
			Call(func() error { return gitutil.New(jirix.NewSeq()).Reset(m.spec.gitRevision) }, "").
			Popd().
			RemoveAll(goInstDir.Abs(jirix)).
			Rename(tmpDir, goInstDir.Abs(jirix)).
			Chmod(goInstDir.Abs(jirix), profilesutil.DefaultDirPerm).
			Done(); err != nil {
			return err
		}

		// Apply patches, if any and build.
		s.Pushd(goSrcDir)
		for _, patchFile := range m.spec.patchFiles {
			s.Run("git", "apply", patchFile)
		}
		makeBin := filepath.Join(goSrcDir, "make.bash")
		env.Set("GOROOT_BOOTSTRAP", goBootstrapDir.Abs(jirix))
		if err := s.Env(env.ToMap()).Last(makeBin); err != nil {
			s.RemoveAll(filepath.Join(goInstDir.Abs(jirix), "bin")).Done()
			return err
		}
		return nil
	}
	if err := profilesutil.AtomicAction(jirix, installGo15Fn, goInstDir.Abs(jirix), "Build and install Go "+version+" @ "+m.spec.gitRevision); err != nil {
		return err
	}
	env.Set("GOROOT_BOOTSTRAP", goBootstrapDir.Symbolic())
	return nil
}

func linux_to_linux(jirix *jiri.X, m *Manager, root jiri.RelPath, target profiles.Target, action profiles.Action) (bindir string, env []string, e error) {
	targetABI := ""
	switch target.Arch() {
	case "arm":
		targetABI = "arm-unknown-linux-gnueabi"
	default:
		return "", nil, fmt.Errorf("Arch %q is not yet supported for crosstools xgcc", target.Arch())
	}
	xgccOutDir := filepath.Join(m.root.Abs(jirix), "profiles", "cout", "xgcc")
	xtoolInstDir := filepath.Join(xgccOutDir, "crosstools-ng-"+targetABI)
	xgccInstDir := filepath.Join(xgccOutDir, targetABI)
	xgccLinkInstDir := filepath.Join(xgccOutDir, "links-"+targetABI)
	if action == profiles.Uninstall {
		s := jirix.NewSeq()
		s.Last("chmod", "-R", "+w", xgccInstDir)
		for _, dir := range []string{xtoolInstDir, xgccInstDir, xgccLinkInstDir} {
			if err := s.RemoveAll(dir).Done(); err != nil {
				return "", nil, err
			}
		}
		return "", nil, nil
	}
	// Install dependencies.
	pkgs := []string{
		"automake", "bison", "bzip2", "curl", "flex", "g++", "gawk", "libexpat1-dev",
		"gettext", "gperf", "libncurses5-dev", "libtool", "subversion", "texinfo",
	}
	if err := profilesutil.InstallPackages(jirix, pkgs); err != nil {
		return "", nil, err
	}

	// Build and install crosstool-ng.
	installNgFn := func() error {
		xgccSrcDir := jiri.NewRelPath("third_party", "csrc", "crosstool-ng-1.20.0").Abs(jirix)
		return jirix.NewSeq().
			Pushd(xgccSrcDir).
			Run("autoreconf", "--install", "--force", "--verbose").
			Run("./configure", fmt.Sprintf("--prefix=%v", xtoolInstDir)).
			Run("make", fmt.Sprintf("-j%d", runtime.NumCPU())).
			Run("make", "install").
			Last("make", "distclean")
	}
	if err := profilesutil.AtomicAction(jirix, installNgFn, xtoolInstDir, "Build and install crosstool-ng"); err != nil {
		return "", nil, err
	}

	// Build arm/linux gcc tools.
	installXgccFn := func() error {
		s := jirix.NewSeq()
		tmpDir, err := s.TempDir("", "")
		if err != nil {
			return fmt.Errorf("TempDir() failed: %v", err)
		}
		defer collect.Error(func() error { return jirix.NewSeq().RemoveAll(tmpDir).Done() }, &e)

		bin := filepath.Join(xtoolInstDir, "bin", "ct-ng")
		if err := s.Chdir(tmpDir).Last(bin, targetABI); err != nil {
			return err
		}
		dataPath, err := project.DataDirPath(jirix, "")
		if err != nil {
			return err
		}
		configFile := filepath.Join(dataPath, "crosstool-ng-1.20.0.config")
		config, err := ioutil.ReadFile(configFile)
		if err != nil {
			return fmt.Errorf("ReadFile(%v) failed: %v", configFile, err)
		}
		old, new := "/usr/local/vanadium", filepath.Join(m.root.Abs(jirix), "profiles", "cout")
		newConfig := strings.Replace(string(config), old, new, -1)
		newConfigFile := filepath.Join(tmpDir, ".config")
		if err := s.WriteFile(newConfigFile, []byte(newConfig), profilesutil.DefaultFilePerm).Done(); err != nil {
			return fmt.Errorf("WriteFile(%v) failed: %v", newConfigFile, err)
		}

		dirinfo, err := s.Run(bin, "oldconfig").
			Run(bin, "build").
			Stat(xgccInstDir)
		if err != nil {
			return err
		}
		// crosstool-ng build creates the tool output directory with no write
		// permissions. Change it so that AtomicAction can create the
		// "action completed" file.
		return s.Chmod(xgccInstDir, dirinfo.Mode()|0755).Done()
	}
	if err := profilesutil.AtomicAction(jirix, installXgccFn, xgccInstDir, "Build arm/linux gcc tools"); err != nil {
		return "", nil, err
	}

	linkBinDir := filepath.Join(xgccLinkInstDir, "bin")
	// Create arm/linux gcc symlinks.
	installLinksFn := func() error {
		s := jirix.NewSeq()
		err := s.MkdirAll(linkBinDir, profilesutil.DefaultDirPerm).
			Chdir(xgccLinkInstDir).Done()
		if err != nil {
			return err
		}
		binDir := filepath.Join(xgccInstDir, "bin")
		fileInfoList, err := ioutil.ReadDir(binDir)
		if err != nil {
			return fmt.Errorf("ReadDir(%v) failed: %v", binDir, err)
		}
		for _, fileInfo := range fileInfoList {
			prefix := targetABI + "-"
			if strings.HasPrefix(fileInfo.Name(), prefix) {
				src := filepath.Join(binDir, fileInfo.Name())
				dst := filepath.Join(linkBinDir, strings.TrimPrefix(fileInfo.Name(), prefix))
				if err := s.Symlink(src, dst).Done(); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := profilesutil.AtomicAction(jirix, installLinksFn, xgccLinkInstDir, "Create gcc symlinks"); err != nil {
		return "", nil, err
	}
	vars := []string{
		"CC_FOR_TARGET=" + filepath.Join(linkBinDir, "gcc"),
		"CXX_FOR_TARGET=" + filepath.Join(linkBinDir, "g++"),
	}
	return linkBinDir, vars, nil
}

func darwin_to_linux(jirix *jiri.X, m *Manager, root jiri.RelPath, target profiles.Target, action profiles.Action) (bindir string, env []string, e error) {
	if target.Arch() == "arm" {
		return useLLVM(jirix, m, root, target, action)
	}
	return "", nil, fmt.Errorf("cross compilation from darwin to %s linux is not yet supported.", target.Arch())
}

func darwin_to_ios(jirix *jiri.X, m *Manager, root jiri.RelPath, target profiles.Target, action profiles.Action) (bindir string, env []string, e error) {
	if action == profiles.Uninstall {
		return "", nil, nil
	}

	// As of Go 1.6, unable to generate PIC code for 32-bit arm.
	// As of Go 1.5.x, the linker fails for darwin/32-bit
	// As of Go 1.5.1 the linker fails for darwin/32-bit, and is unable to generate PIC code for 32-bit arm
	if target.Arch() == "386" && (target.Version() == "1.5" || strings.HasPrefix(target.Version(), "1.5.")) {
		return "", nil, fmt.Errorf("32-bit iOS simulator is not supported by go yet with c-archive. " +
			"See https://github.com/golang/go/issues/12683")
	}
	if target.Arch() == "arm" {
		return "", nil, fmt.Errorf("32-bit ARM is not supported by go yet as it does not generate PIC code. " +
			"See https://github.com/golang/go/issues/12681")
	}

	vars := []string{
		"CGO_ENABLED=1",
		"CC_FOR_TARGET=" + filepath.Join(jirix.Root, "release/swift/clang/clangwrap.sh"),
		"CXX_FOR_TARGET=" + filepath.Join(jirix.Root, "release/swift/clang/clangwrap++.sh"),
		"GOOS=darwin",
		"GOHOSTARCH=amd64",
		"GOHOSTOS=darwin",
		// We need to explicitly pass the ios build tag to the make.bash script for go so that it won't compile
		// crypto code that's meant for the mac. It'll work on the device because those files have a build tag
		// of !arm64, but the simulator will fail because those specific APIs don't exist on iOS.
		"GO_FLAGS=-tags ios",
	}

	// 32-bit arm is always armv7 in Apple-land
	if target.Arch() == "arm" {
		vars = append(vars, "GOARM=7")
	} else if target.Arch() == "arm64" {
		vars = append(vars, "GOARM=arm64")
	}

	// Add patch for text-relocation errors on arm64
	// Submitted to golang, currently marked for 1.7: https://go-review.googlesource.com/#/c/19206/
	patchPath := filepath.Join(jirix.Root, "release/go/src/v.io/x/devtools/jiri-v23-profile/go/macho_linker.patch")
	m.spec.patchFiles = append(m.spec.patchFiles, patchPath)
	return "", vars, nil
}

func to_android(jirix *jiri.X, m *Manager, root jiri.RelPath, target profiles.Target, action profiles.Action) (bindir string, env []string, e error) {
	if action == profiles.Uninstall {
		return "", nil, nil
	}
	ev := envvar.VarsFromSlice(target.CommandLineEnv().Vars)
	jiri.ExpandEnv(jirix, ev)
	ndk := ev.Get("ANDROID_NDK_DIR")
	if len(ndk) == 0 {
		return "", nil, fmt.Errorf("ANDROID_NDK_DIR not specified in the command line environment")
	}
	ndkBin := filepath.Join(ndk, "bin")
	var abi string
	switch target.Arch() {
	case "amd64":
		abi = "x86_64-linux-android"
	case "arm":
		abi = "arm-linux-androideabi"
	default:
		return "", nil, fmt.Errorf("could not locate android abi for target arch %s", target.Arch())
	}
	var (
		cc   = filepath.Join(ndkBin, fmt.Sprintf("%s-gcc", abi))
		cxx  = filepath.Join(ndkBin, fmt.Sprintf("%s-g++", abi))
		vars = []string{
			"CC_FOR_TARGET=" + cc,
			"CXX_FOR_TARGET=" + cxx,
			"CLANG=" + cc,
			"CLANG++=" + cxx,
		}
	)
	return ndkBin, vars, nil
}

func to_fnl(jirix *jiri.X, m *Manager, root jiri.RelPath, target profiles.Target, action profiles.Action) (bindir string, env []string, e error) {
	if action == profiles.Uninstall {
		return "", nil, nil
	}
	fnlRoot := os.Getenv("FNL_JIRI_ROOT")
	if len(fnlRoot) == 0 {
		return "", nil, fmt.Errorf("FNL_JIRI_ROOT not specified in the command line environment")
	}
	muslBin := filepath.Join(fnlRoot, "out/root/tools/x86_64-fuchsia-linux-musl/bin")
	// This cross compiles by building a go compiler with HOST=386 rather than amd64 because
	// the go compiler build process doesn't support building two different versions when
	// host and target are the same.
	// TODO(bprosnitz) Determine whether fnl is linux or a separate os and make the build process cleaner.
	vars := []string{
		"CC_FOR_TARGET=" + filepath.Join(muslBin, "x86_64-fuchsia-linux-musl-gcc"),
		"CXX_FOR_TARGET=" + filepath.Join(muslBin, "x86_64-fuchsia-linux-musl-g++"),
		"GOHOSTARCH=386",
		"GOHOSTOS=linux",
		"GOARCH=amd64",
		"GOOS=linux",
		// Under $JIRI_ROOT, packages for both standard amd64 linux machines and fnl
		// will be placed under "release/go/pkg/linux_amd64" yet be incompatible.
		// This sets a suffix "linux_amd64_musl" for fnl target packages.
		// Note that this isn't a problem for the fnl go cross compiler itself
		// because it is cross compiling from 386 to amd64 and the package
		// directories will not conflict.
		// GO_FLAGS is an environment variable used by go's compiler build scripts
		// to inject flags into the build commands.
		"GO_FLAGS=-installsuffix=musl",
	}
	return "", vars, nil
}

// abs2symbolic is the inverse of jiri.RelPath.Abs
func abs2symbolic(jirix *jiri.X, path string) string {
	if !strings.HasPrefix(path, jirix.Root) {
		return path
	}
	rel, err := filepath.Rel(jirix.Root, path)
	if err != nil {
		return rel
	}
	return jiri.NewRelPath(rel).Symbolic()
}
