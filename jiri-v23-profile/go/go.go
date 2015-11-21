// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package go_profile

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"v.io/jiri/collect"
	"v.io/jiri/jiri"
	"v.io/jiri/profiles"
	"v.io/jiri/project"
	"v.io/x/lib/envvar"
)

var (
	profileName   = "go"
	go15GitRemote = "https://github.com/golang/go.git"
	goSysRootFlag = ""
)

// Supported cross compilation toolchains.
type xspec struct{ arch, os string }
type xbuilder func(*jiri.X, *Manager, profiles.RelativePath, profiles.Target, profiles.Action) (bindir string, env []string, e error)

var xcompilers = map[xspec]map[xspec]xbuilder{
	xspec{"amd64", "darwin"}: {
		xspec{"amd64", "linux"}: darwin_to_linux,
		xspec{"arm", "linux"}:   darwin_to_linux,
		xspec{"arm", "android"}: to_android,
	},
	xspec{"amd64", "linux"}: {
		xspec{"amd64", "fnl"}:   to_fnl,
		xspec{"arm", "linux"}:   linux_to_linux,
		xspec{"arm", "android"}: to_android,
	},
}

type versionSpec struct {
	gitRevision string
	patchFiles  []string
}

func init() {
	m := &Manager{
		versionInfo: profiles.NewVersionInfo(profileName, map[string]interface{}{
			"1.5": &versionSpec{
				"cc6554f750ccaf63bcdcc478b2a60d71ca76d342", nil},
			"1.5.1": &versionSpec{
				"f2e4c8b5fb3660d793b2c545ef207153db0a34b1", nil},
			// 1.5.1 at a specific git revision, create a new version anytime
			// a new profile is checked in.
			"1.5.1.1:2738c5e0": &versionSpec{
				"492a62e945555bbf94a6f9dd6d430f712738c5e0", nil},
		}, "1.5.1"),
	}
	profiles.Register(profileName, m)
}

type Manager struct {
	root, goRoot, targetDir, goInstDir profiles.RelativePath
	versionInfo                        *profiles.VersionInfo
	spec                               versionSpec
}

func (_ Manager) Name() string {
	return profileName
}

func (m Manager) String() string {
	return fmt.Sprintf("%s[%s]", profileName, m.versionInfo.Default())
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
	flags.StringVar(&goSysRootFlag, profileName+".sysroot", "", "sysroot for cross compiling to the currently specified target")
}

func (m *Manager) initForTarget(jirix *jiri.X, root profiles.RelativePath, target *profiles.Target) error {
	m.root = root
	m.goRoot = root.Join("go")
	if err := m.versionInfo.Lookup(target.Version(), &m.spec); err != nil {
		return err
	}
	m.targetDir = m.goRoot.Join(target.TargetSpecificDirname())
	m.goInstDir = m.targetDir.Join(m.spec.gitRevision)
	if jirix.Verbose() {
		fmt.Fprintf(jirix.Stdout(), "Installation Directories for: %s\n", target)
		fmt.Fprintf(jirix.Stdout(), "Go Profiles: %s\n", m.goRoot)
		fmt.Fprintf(jirix.Stdout(), "Go Target+Revision %s\n", m.goInstDir)
	}
	return nil
}

func relPath(rp profiles.RelativePath) string {
	if profiles.SchemaVersion() >= 4 {
		return rp.String()
	}
	return rp.Expand()
}

func (m *Manager) Install(jirix *jiri.X, root profiles.RelativePath, target profiles.Target) error {
	if err := m.initForTarget(jirix, root, &target); err != nil {
		return err
	}

	jirix.NewSeq().RemoveAll(m.goRoot.Join("go-bootstrap").Expand())
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

	env := envvar.VarsFromSlice(target.Env.Vars)
	if err := m.installGo15Plus(jirix, target.Version(), env); err != nil {
		return err
	}

	// Merge our target environment and GOROOT
	goEnv := []string{"GOROOT=" + relPath(m.goInstDir)}
	profiles.MergeEnv(profiles.ProfileMergePolicies(), env, goEnv)
	target.Env.Vars = env.ToSlice()
	if profiles.SchemaVersion() >= 4 {
		target.InstallationDir = m.goInstDir.RelativePath()
		profiles.InstallProfile(profileName, m.goRoot.RelativePath())
	} else {
		target.InstallationDir = m.goInstDir.Expand()
		profiles.InstallProfile(profileName, m.goRoot.Expand())
	}
	return profiles.AddProfileTarget(profileName, target)
}

func (m *Manager) Uninstall(jirix *jiri.X, root profiles.RelativePath, target profiles.Target) error {
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
	if err := s.RemoveAll(m.targetDir.Expand()).Done(); err != nil {
		return err
	}
	if profiles.RemoveProfileTarget(profileName, target) {
		// If there are no more targets then remove the entire go directory,
		// including the bootstrap one.
		return s.RemoveAll(m.goRoot.Expand()).Done()
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

		name := "go1.4.2.src.tar.gz"
		remote, local := "https://storage.googleapis.com/golang/"+name, filepath.Join(tmpDir, name)
		parentDir := filepath.Dir(go14Dir)
		goSrcDir := filepath.Join(go14Dir, "src")
		makeBin := filepath.Join(goSrcDir, "make.bash")

		if err := s.Run("curl", "-Lo", local, remote).
			Run("tar", "-C", tmpDir, "-xzf", local).
			Remove(local).
			RemoveAll(go14Dir).
			MkdirAll(parentDir, profiles.DefaultDirPerm).Done(); err != nil {
			return err
		}

		opts := s.GetOpts()
		opts.Env = env.ToMap()
		return s.Rename(filepath.Join(tmpDir, "go"), go14Dir).
			Chdir(goSrcDir).
			Opts(opts).Last(makeBin, "--no-clean")
	}
	return profiles.AtomicAction(jirix, installGo14Fn, go14Dir, "Build and install Go 1.4")
}

// installGo15Plus installs any version of go past 1.5 at the specified git and go
// revision.
func (m *Manager) installGo15Plus(jirix *jiri.X, version string, env *envvar.Vars) error {
	// First install bootstrap Go 1.4 for the host.
	goBootstrapDir := m.targetDir.Join("go-bootstrap")
	if err := installGo14(jirix, goBootstrapDir.Expand(), envvar.VarsFromOS()); err != nil {
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

		goInstDir := m.goInstDir.Expand()
		goSrcDir := filepath.Join(goInstDir, "src")

		// Git clone the code and get into the right directory.
		if err := s.Pushd(tmpDir).
			Call(func() error { return jirix.Git().Clone(go15GitRemote, tmpDir) }, "").
			Call(func() error { return jirix.Git().Reset(m.spec.gitRevision) }, "").
			Popd().
			RemoveAll(goInstDir).
			Rename(tmpDir, goInstDir).
			MkdirAll(m.targetDir.Expand(), profiles.DefaultDirPerm).
			Done(); err != nil {
			return err
		}

		// Apply patches, if any and build.
		s.Pushd(goSrcDir)
		for _, patchFile := range m.spec.patchFiles {
			s.Run("git", "apply", patchFile)
		}
		makeBin := filepath.Join(goSrcDir, "make.bash")
		env.Set("GOROOT_BOOTSTRAP", goBootstrapDir.Expand())
		opts := s.GetOpts()
		opts.Env = env.ToMap()
		if err := s.Opts(opts).Last(makeBin); err != nil {
			s.RemoveAll(filepath.Join(goInstDir, "bin")).Done()
			return err
		}
		return nil
	}
	if err := profiles.AtomicAction(jirix, installGo15Fn, m.goInstDir.Expand(), "Build and install Go "+version+" @ "+m.spec.gitRevision); err != nil {
		return err
	}
	env.Set("GOROOT_BOOTSTRAP", relPath(goBootstrapDir))
	return nil
}

func linux_to_linux(jirix *jiri.X, m *Manager, root profiles.RelativePath, target profiles.Target, action profiles.Action) (bindir string, env []string, e error) {
	targetABI := ""
	switch target.Arch() {
	case "arm":
		targetABI = "arm-unknown-linux-gnueabi"
	default:
		return "", nil, fmt.Errorf("Arch %q is not yet supported for crosstools xgcc", target.Arch())
	}
	xgccOutDir := filepath.Join(m.root.Expand(), "profiles", "cout", "xgcc")
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
	if err := profiles.InstallPackages(jirix, pkgs); err != nil {
		return "", nil, err
	}

	// Build and install crosstool-ng.
	installNgFn := func() error {
		xgccSrcDir := filepath.Join(m.root.RootJoin("third_party", "csrc", "crosstool-ng-1.20.0").Expand())
		return jirix.NewSeq().
			Pushd(xgccSrcDir).
			Run("autoreconf", "--install", "--force", "--verbose").
			Run("./configure", fmt.Sprintf("--prefix=%v", xtoolInstDir)).
			Run("make", fmt.Sprintf("-j%d", runtime.NumCPU())).
			Run("make", "install").
			Last("make", "distclean")
	}
	if err := profiles.AtomicAction(jirix, installNgFn, xtoolInstDir, "Build and install crosstool-ng"); err != nil {
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
		old, new := "/usr/local/vanadium", filepath.Join(m.root.Expand(), "profiles", "cout")
		newConfig := strings.Replace(string(config), old, new, -1)
		newConfigFile := filepath.Join(tmpDir, ".config")
		if err := s.WriteFile(newConfigFile, []byte(newConfig), profiles.DefaultFilePerm).Done(); err != nil {
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
	if err := profiles.AtomicAction(jirix, installXgccFn, xgccInstDir, "Build arm/linux gcc tools"); err != nil {
		return "", nil, err
	}

	linkBinDir := filepath.Join(xgccLinkInstDir, "bin")
	// Create arm/linux gcc symlinks.
	installLinksFn := func() error {
		s := jirix.NewSeq()
		err := s.MkdirAll(linkBinDir, profiles.DefaultDirPerm).
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
	if err := profiles.AtomicAction(jirix, installLinksFn, xgccLinkInstDir, "Create gcc symlinks"); err != nil {
		return "", nil, err
	}
	vars := []string{
		"CC_FOR_TARGET=" + filepath.Join(linkBinDir, "gcc"),
		"CXX_FOR_TARGET=" + filepath.Join(linkBinDir, "g++"),
	}
	return linkBinDir, vars, nil
}

func darwin_to_linux(jirix *jiri.X, m *Manager, root profiles.RelativePath, target profiles.Target, action profiles.Action) (bindir string, env []string, e error) {
	return "", nil, fmt.Errorf("cross compilation from darwin to linux is not yet supported.")
}

func to_android(jirix *jiri.X, m *Manager, root profiles.RelativePath, target profiles.Target, action profiles.Action) (bindir string, env []string, e error) {
	if action == profiles.Uninstall {
		return "", nil, nil
	}
	ev := envvar.VarsFromSlice(target.CommandLineEnv().Vars)
	root.ExpandEnv(ev)
	ndk := ev.Get("ANDROID_NDK_DIR")
	if len(ndk) == 0 {
		return "", nil, fmt.Errorf("ANDROID_NDK_DIR not specified in the command line environment")
	}
	ndkBin := filepath.Join(ndk, "bin")
	vars := []string{
		"CC_FOR_TARGET=" + filepath.Join(ndkBin, "arm-linux-androideabi-gcc"),
		"CXX_FOR_TARGET=" + filepath.Join(ndkBin, "arm-linux-androideabi-g++"),
	}
	return ndkBin, vars, nil
}

func to_fnl(jirix *jiri.X, m *Manager, root profiles.RelativePath, target profiles.Target, action profiles.Action) (bindir string, env []string, e error) {
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
