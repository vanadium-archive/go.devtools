// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package go_profile

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"v.io/jiri/collect"
	"v.io/jiri/profiles"
	"v.io/jiri/project"
	"v.io/jiri/tool"
	"v.io/x/lib/envvar"
)

var (
	profileName      = "go"
	profileVersion   = "1.5"
	patchFiles       = []string{}
	go15GitRemote    = "https://github.com/golang/go.git"
	go15GitRevision  = "cc6554f750ccaf63bcdcc478b2a60d71ca76d342"
	goInstallDirFlag = ""
	goSysRootFlag    = ""
)

// Supported cross compilation toolchains.
type xspec struct{ arch, os string }
type xbuilder func(*tool.Context, *Manager, profiles.Target, profiles.Action) (bindir string, env []string, e error)

var xcompilers = map[xspec]map[xspec]xbuilder{
	xspec{"amd64", "darwin"}: {
		xspec{"amd64", "linux"}: darwin_to_linux,
		xspec{"arm", "linux"}:   darwin_to_linux,
	},
	xspec{"amd64", "linux"}: {
		xspec{"arm", "linux"}: linux_to_linux,
	},
}

func init() {
	profiles.Register(profileName, &Manager{})
}

type Manager struct {
	root   string
	goRoot string
}

func (_ Manager) Name() string {
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
	m.goRoot = goInstallDirFlag
	if len(m.goRoot) == 0 {
		m.goRoot = filepath.Join(m.root, "profiles", "go")
	}
}

func (m *Manager) AddFlags(flags *flag.FlagSet, action profiles.Action) {
	flags.StringVar(&goInstallDirFlag, profileName+".install-dir", "", "installation directory for go profile builds.")
	flags.StringVar(&goSysRootFlag, profileName+".sysroot", "", "sysroot for cross compiling to the currently specified target")
}

func (m Manager) targetDir(target *profiles.Target) string {
	return filepath.Join(m.goRoot, profiles.TargetSpecificDirname(*target, false))
}

func (m *Manager) Install(ctx *tool.Context, target profiles.Target) error {
	target.Version = profileVersion
	if target.CrossCompiling() {
		// We may need to install an additional cross compilation toolchain
		// for cgo to work.
		if builder := xcompilers[xspec{runtime.GOARCH, runtime.GOOS}][xspec{target.Arch, target.OS}]; builder != nil {
			bindir, vars, err := builder(ctx, m, target, profiles.Install)
			if err != nil {
				return err
			}
			target.Env.Vars = envvar.MergeSlices(target.Env.Vars, vars, []string{"XTOOLS="+bindir})
		}
	}

	// Force GOARM, GOARCH to have the values specified in the target.
	target.Env.Vars = envvar.MergeSlices(target.Env.Vars, []string{
		"GOARCH=" + target.Arch,
		"GOOS=" + target.OS,
	})

	env := envvar.VarsFromSlice(target.Env.Vars)
	targetDir := m.targetDir(&target)
	go15Root, err := installGo15(ctx, m.goRoot, targetDir, patchFiles, env)
	if err != nil {
		return err
	}
	// Merge the environment variables as set via the OS and those set in
	// the profile and write them back to the target so that they get
	// written to the manifest.
	target.Env.Vars = envvar.MergeSlices(profiles.GoEnvironmentFromOS(), target.Env.Vars)
	// Now make sure that GOROOT is set to the newly installed go directory.
	target.Env.Vars = envvar.MergeSlices(target.Env.Vars, []string{"GOROOT=" + go15Root})
	target.InstallationDir = go15Root
	profiles.InstallProfile(profileName, m.goRoot)
	return profiles.AddProfileTarget(profileName, target)
}

func (m *Manager) Uninstall(ctx *tool.Context, target profiles.Target) error {
	if target.CrossCompiling() {
		// We may need to install an additional cross compilation toolchain
		// for cgo to work.
		if builder := xcompilers[xspec{runtime.GOARCH, runtime.GOOS}][xspec{target.Arch, target.OS}]; builder != nil {
			if _, _, err := builder(ctx, m, target, profiles.Uninstall); err != nil {
				return err
			}
		}
	}
	// Force GOARM, GOARCH to have the values specified in the target.
	target.Env.Vars = envvar.MergeSlices(target.Env.Vars, []string{
		"GOARCH=" + target.Arch,
		"GOOS=" + target.OS,
	})
	targetDir := m.targetDir(&target)
	if err := ctx.Run().RemoveAll(targetDir); err != nil {
		return err
	}
	if profiles.RemoveProfileTarget(profileName, target) {
		// If there are no more targets then remove the entire go directory,
		// including the bootstrap one.
		return ctx.Run().RemoveAll(m.goRoot)
	}
	return nil
}

func (m *Manager) Update(ctx *tool.Context, target profiles.Target) error {
	return profiles.ErrNoIncrementalUpdate
}

func isInstalled(ctx *tool.Context, bin string, re *regexp.Regexp) bool {
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	if err := ctx.Run().CommandWithOpts(opts, bin, "version"); err != nil {
		return false
	}
	return re.Match(out.Bytes())
}

// installGo14 installs Go 1.4 at a given location, using the provided
// environment during compilation.
func installGo14(ctx *tool.Context, goDir string, env *envvar.Vars) error {
	if isInstalled(ctx, filepath.Join(goDir, "bin", "go"), regexp.MustCompile("go1.4")) {
		return nil
	}
	tmpDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		return err
	}
	defer ctx.Run().RemoveAll(tmpDir)
	name := "go1.4.2.src.tar.gz"
	remote, local := "https://storage.googleapis.com/golang/"+name, filepath.Join(tmpDir, name)
	if err := profiles.RunCommand(ctx, "curl", []string{"-Lo", local, remote}, nil); err != nil {
		return err
	}
	if err := profiles.RunCommand(ctx, "tar", []string{"-C", tmpDir, "-xzf", local}, nil); err != nil {
		return err
	}
	if err := ctx.Run().RemoveAll(local); err != nil {
		return err
	}
	if err := ctx.Run().Rename(filepath.Join(tmpDir, "go"), goDir); err != nil {
		return err
	}
	goSrcDir := filepath.Join(goDir, "src")
	if err := ctx.Run().Chdir(goSrcDir); err != nil {
		return err
	}
	makeBin := filepath.Join(goSrcDir, "make.bash")
	makeArgs := []string{"--no-clean"}
	if err := profiles.RunCommand(ctx, makeBin, makeArgs, env.ToMap()); err != nil {
		return err
	}
	return nil
}

// installGo15 installs Go 1.5 at a given location, using the provided
// environment during compilation.
func installGo15(ctx *tool.Context, bootstrapDir, goDir string, patchFiles []string, env *envvar.Vars) (string, error) {

	go15Dir := filepath.Join(goDir, go15GitRevision)
	if isInstalled(ctx, filepath.Join(go15Dir, "bin", "go"), regexp.MustCompile("go1.5")) {
		return go15Dir, nil
	}

	// First install bootstrap Go 1.4 for the host.
	if err := ctx.Run().MkdirAll(goDir, profiles.DefaultDirPerm); err != nil {
		return "", err
	}

	goBootstrapDir := filepath.Join(bootstrapDir, "go-bootstrap")

	if err := installGo14(ctx, goBootstrapDir, envvar.VarsFromOS()); err != nil {
		return "", err
	}

	tmpDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		return "", err
	}
	defer ctx.Run().RemoveAll(tmpDir)

	if err := profiles.GitCloneRepo(ctx, go15GitRemote, go15GitRevision, tmpDir, profiles.DefaultDirPerm); err != nil {

		return "", err
	}
	if err := ctx.Run().Chdir(tmpDir); err != nil {
		return "", err
	}

	// Check out the go1.5 release branch.
	if err := profiles.RunCommand(ctx, "git", []string{"checkout", "go1.5"}, nil); err != nil {
		return "", err
	}

	if err := profiles.RunCommand(ctx, "git", []string{"checkout", "-b", "go1.5"}, nil); err != nil {
		return "", err
	}

	if profiles.DirectoryExists(ctx, go15Dir) {
		ctx.Run().RemoveAll(go15Dir)
	}
	if err := ctx.Run().Rename(tmpDir, go15Dir); err != nil {
		return "", err
	}
	goSrcDir := filepath.Join(go15Dir, "src")
	if err := ctx.Run().Chdir(goSrcDir); err != nil {
		return "", err
	}
	// Apply patches, if any.
	for _, patchFile := range patchFiles {
		if err := profiles.RunCommand(ctx, "git", []string{"apply", patchFile}, nil); err != nil {
			return "", err
		}
	}
	makeBin := filepath.Join(goSrcDir, "make.bash")
	env.Set("GOROOT_BOOTSTRAP", goBootstrapDir)
	if err := profiles.RunCommand(ctx, makeBin, nil, env.ToMap()); err != nil {
		return "", err
	}
	return go15Dir, nil
}

func linux_to_linux(ctx *tool.Context, m *Manager, target profiles.Target, action profiles.Action) (bindir string, env []string, e error) {
	targetABI := ""
	switch target.Arch {
	case "arm":
		targetABI = "arm-unknown-linux-gnueabi"
	default:
		return "", nil, fmt.Errorf("Arch %q is not yet supported for crosstools xgcc", target.Arch)
	}
	xgccOutDir := filepath.Join(m.root, "profiles", "cout", "xgcc")
	xtoolInstDir := filepath.Join(xgccOutDir, "crosstools-ng-"+targetABI)
	xgccInstDir := filepath.Join(xgccOutDir, targetABI)
	xgccLinkInstDir := filepath.Join(xgccOutDir, "links-"+targetABI)
	switch action {
	case profiles.Uninstall:
		for _, dir := range []string{xtoolInstDir, xgccInstDir, xgccLinkInstDir} {
			if err := ctx.Run().RemoveAll(dir); err != nil {
				return "", nil, err
			}
		}
		return "", nil, nil
	case profiles.Update:
		return "", nil, nil
	}
	// Install dependencies.
	pkgs := []string{
		"automake", "bison", "bzip2", "curl", "flex", "g++", "gawk", "libexpat1-dev",
		"gettext", "gperf", "libncurses5-dev", "libtool", "subversion", "texinfo",
	}
	if err := profiles.InstallPackages(ctx, pkgs); err != nil {
		return "", nil, err
	}

	// Build and install crosstool-ng.
	installNgFn := func() error {
		xgccSrcDir := filepath.Join(m.root, "third_party", "csrc", "crosstool-ng-1.20.0")
		if err := ctx.Run().Chdir(xgccSrcDir); err != nil {
			return err
		}
		if err := profiles.RunCommand(ctx, "autoreconf", []string{"--install", "--force", "--verbose"}, nil); err != nil {
			return err
		}
		if err := profiles.RunCommand(ctx, "./configure", []string{fmt.Sprintf("--prefix=%v", xtoolInstDir)}, nil); err != nil {
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
	if err := profiles.AtomicAction(ctx, installNgFn, xtoolInstDir, "Build and install crosstool-ng"); err != nil {
		return "", nil, err
	}

	// Build arm/linux gcc tools.
	installXgccFn := func() error {
		tmpDir, err := ctx.Run().TempDir("", "")
		if err != nil {
			return fmt.Errorf("TempDir() failed: %v", err)
		}
		defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpDir) }, &e)
		if err := ctx.Run().Chdir(tmpDir); err != nil {
			return err
		}
		bin := filepath.Join(xtoolInstDir, "bin", "ct-ng")
		if err := profiles.RunCommand(ctx, bin, []string{targetABI}, nil); err != nil {
			return err
		}
		dataPath, err := project.DataDirPath(ctx, "")
		if err != nil {
			return err
		}
		configFile := filepath.Join(dataPath, "crosstool-ng-1.20.0.config")
		config, err := ioutil.ReadFile(configFile)
		if err != nil {
			return fmt.Errorf("ReadFile(%v) failed: %v", configFile, err)
		}
		old, new := "/usr/local/vanadium", filepath.Join(m.root, "profiles", "cout")
		newConfig := strings.Replace(string(config), old, new, -1)
		newConfigFile := filepath.Join(tmpDir, ".config")
		if err := ctx.Run().WriteFile(newConfigFile, []byte(newConfig), profiles.DefaultFilePerm); err != nil {
			return fmt.Errorf("WriteFile(%v) failed: %v", newConfigFile, err)
		}
		if err := profiles.RunCommand(ctx, bin, []string{"oldconfig"}, nil); err != nil {
			return err
		}
		if err := profiles.RunCommand(ctx, bin, []string{"build"}, nil); err != nil {
			// Temp to get saved output
			return err
		}
		// crosstool-ng build creates the tool output directory with no write
		// permissions. Change it so that AtomicAction can create the
		// "action completed" file.
		dirinfo, err := ctx.Run().Stat(xgccInstDir)
		if err != nil {
			return err
		}
		if err := os.Chmod(xgccInstDir, dirinfo.Mode()|0755); err != nil {
			return err
		}
		return nil
	}
	if err := profiles.AtomicAction(ctx, installXgccFn, xgccInstDir, "Build arm/linux gcc tools"); err != nil {
		return "", nil, err
	}

	linkBinDir := filepath.Join(xgccLinkInstDir, "bin")
	// Create arm/linux gcc symlinks.
	installLinksFn := func() error {
		if err := ctx.Run().MkdirAll(linkBinDir, profiles.DefaultDirPerm); err != nil {
			return err
		}
		if err := ctx.Run().Chdir(xgccLinkInstDir); err != nil {
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
				if err := ctx.Run().Symlink(src, dst); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := profiles.AtomicAction(ctx, installLinksFn, xgccLinkInstDir, "Create gcc symlinks"); err != nil {
		return "", nil, err
	}
	vars := []string{
		"CC_FOR_TARGET=" + filepath.Join(linkBinDir, "gcc"),
		"CXX_FOR_TARGET=" + filepath.Join(linkBinDir, "g++"),
	}
	return linkBinDir, vars, nil
}

func darwin_to_linux(ctx *tool.Context, m *Manager, target profiles.Target, action profiles.Action) (bindir string, env []string, e error) {
	return "", nil, fmt.Errorf("cross compilation from darwin to linux is not yet supported.")
}
