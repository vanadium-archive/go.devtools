package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"tools/lib/cmdline"
	"tools/lib/envutil"
	"tools/lib/runutil"
	"tools/lib/util"
)

var (
	defaultDirPerm  = os.FileMode(0755)
	defaultFilePerm = os.FileMode(0644)
	knownProfiles   = map[string]string{
		"arm":           "ARM cross-compilation components of the veyron project",
		"mobile":        "mobile components of the veyron project",
		"proximity":     "proximity components of the veyron project",
		"proximity-arm": "ARM cross-compilation of the proximity components of the veyron project",
		"web":           "web components of the veyron project",
	}
)

// cmdProfile represents the 'profile' command of the veyron tool.
var cmdProfile = &cmdline.Command{
	Name:  "profile",
	Short: "Manage veyron profiles",
	Long: `
To facilitate development across different platforms, veyron defines
platform-independent profiles that map different platforms to a set
of libraries and tools that can be used for a factor of veyron
development.
`,
	Children: []*cmdline.Command{cmdProfileList, cmdProfileSetup},
}

// cmdProfileList represents the 'list' sub-command of the
// 'profile' command of the veyron tool.
var cmdProfileList = &cmdline.Command{
	Run:   runProfileList,
	Name:  "list",
	Short: "List known veyron profiles",
	Long:  "List known veyron profiles.",
}

func runProfileList(command *cmdline.Command, _ []string) error {
	fmt.Fprintf(command.Stdout(), "Known profiles:\n")
	for name, description := range knownProfiles {
		fmt.Fprintf(command.Stdout(), "  %q: %s\n", name, description)
	}
	return nil
}

// cmdProfileSetup represents the 'setup' sub-command of the 'profile'
// command of the veyron tool.
var cmdProfileSetup = &cmdline.Command{
	Run:      runProfileSetup,
	Name:     "setup",
	Short:    "Set up the given veyron profiles",
	Long:     "Set up the given veyron profiles.",
	ArgsName: "<profiles>",
	ArgsLong: "<profiles> is a list of profiles to set up.",
}

func runProfileSetup(command *cmdline.Command, args []string) error {
	// Check that the profiles to be set up exist.
	for _, arg := range args {
		if _, ok := knownProfiles[arg]; !ok {
			return command.UsageErrorf("profile %v does not exist", arg)
		}
	}

	// Setup the profiles.
	ctx := util.NewContextFromCommand(command, true)
	for _, arg := range args {
		setupFn := func() error {
			return setup(ctx, runtime.GOOS, arg)
		}
		if err := ctx.Run().Function(setupFn, fmt.Sprintf("Set up profile %q", arg)); err != nil {
			return err
		}
	}
	return nil
}

type unknownProfileErr string

func (e unknownProfileErr) Error() string {
	return fmt.Sprintf("unknown profile %q", e)
}

func reportNotImplemented(ctx *util.Context, os, profile string) {
	ctx.Run().Output([]string{fmt.Sprintf("profile %q is not implemented on %q", profile, os)})
}

func setup(ctx *util.Context, os, profile string) error {
	switch os {
	case "darwin":
		switch profile {
		case "web":
			return setupWebDarwin(ctx)
		case "arm", "mobile", "proximity", "proximity-arm":
			reportNotImplemented(ctx, os, profile)
		}
	case "linux":
		switch profile {
		case "arm":
			return setupArmLinux(ctx)
		case "mobile":
			return setupMobileLinux(ctx)
		case "proximity":
			return setupProximityLinux(ctx)
		case "proximity-arm":
			return setupProximityArmLinux(ctx)
		case "web":
			return setupWebLinux(ctx)
		}
	default:
		reportNotImplemented(ctx, os, profile)
	}
	return nil
}

func atomicAction(ctx *util.Context, installFn func() error, dir, message string) error {
	atomicFn := func() error {
		if dir != "" && directoryExists(ctx, dir) {
			return nil
		}
		if err := installFn(); err != nil {
			if dir != "" {
				ctx.Run().Function(runutil.RemoveAll(dir))
			}
			return err
		}
		return nil
	}
	return ctx.Run().Function(atomicFn, message)
}

func directoryExists(ctx *util.Context, dir string) bool {
	return ctx.Run().Command("test", "-d", dir) == nil
}

type androidPkg struct {
	name      string
	directory string
}

func installAndroidPkg(ctx *util.Context, sdkRoot string, pkg androidPkg) error {
	installPkgFn := func() error {
		// Identify all indexes that match the given package.
		var out bytes.Buffer
		androidBin := filepath.Join(sdkRoot, "tools", "android")
		androidArgs := []string{"list", "sdk", "--all"}
		opts := ctx.Run().Opts()
		opts.Stdout = &out
		opts.Stderr = &out
		if err := ctx.Run().CommandWithOpts(opts, androidBin, androidArgs...); err != nil {
			return err
		}
		scanner, indexes := bufio.NewScanner(&out), []int{}
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Index(line, pkg.name) != -1 {
				// The output of "android list sdk --all" looks as follows:
				//
				// Packages available for installation or update: 118
				//    1- Android SDK Tools, revision 23.0.5
				//    2- Android SDK Platform-tools, revision 21
				//    3- Android SDK Build-tools, revision 21.1
				// ...
				//
				// The following logic gets the package index.
				index, err := strconv.Atoi(strings.TrimSpace(line[:4]))
				if err != nil {
					return fmt.Errorf("%v", err)
				}
				indexes = append(indexes, index)
			}
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("Scan() failed: %v", err)
		}
		switch {
		case len(indexes) == 0:
			return fmt.Errorf("no package matching %q found", pkg.name)
		case len(indexes) > 1:
			return fmt.Errorf("multiple packages matching %q found", pkg.name)
		}

		// Install the target package.
		androidArgs = []string{"update", "sdk", "--no-ui", "--all", "--filter", fmt.Sprintf("%d", indexes[0])}
		var stdin, stdout bytes.Buffer
		stdin.WriteString("y") // pasing "y" to accept Android's license agreement
		opts = ctx.Run().Opts()
		opts.Stdin = &stdin
		opts.Stdout = &stdout
		opts.Stderr = &stdout
		err := ctx.Run().CommandWithOpts(opts, androidBin, androidArgs...)
		if err != nil || verboseFlag {
			fmt.Fprintf(ctx.Stdout(), out.String())
		}
		return err
	}
	return atomicAction(ctx, installPkgFn, pkg.directory, fmt.Sprintf("Install %s", pkg.name))
}

// installDeps identifies the dependencies that need to be installed
// and installs them using the OS-specific package manager.
func installDeps(ctx *util.Context, pkgs []string) error {
	installDepsFn := func() error {
		switch runtime.GOOS {
		case "linux":
			installPkgs := []string{}
			for _, pkg := range pkgs {
				if err := run(ctx, "dpkg", []string{"-s", pkg}, nil); err != nil {
					installPkgs = append(installPkgs, pkg)
				}
			}
			if len(installPkgs) > 0 {
				args := append([]string{"apt-get", "install", "-y"}, installPkgs...)
				if err := ctx.Run().Command("sudo", args...); err != nil {
					return err
				}
			}
		case "brew":
			installPkgs := []string{}
			for _, pkg := range pkgs {
				if err := run(ctx, "brew", []string{"ls", "--versions", pkg}, nil); err != nil {
					installPkgs = append(installPkgs, pkg)
				}
			}
			if len(installPkgs) > 0 {
				args := append([]string{"install"}, installPkgs...)
				if err := ctx.Run().Command("brew", args...); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return ctx.Run().Function(installDepsFn, "Install dependencies")
}

func run(ctx *util.Context, bin string, args []string, env map[string]string) error {
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	opts.Env = env
	err := ctx.Run().CommandWithOpts(opts, bin, args...)
	if err != nil || verboseFlag {
		fmt.Fprintf(ctx.Stdout(), out.String())
	}
	return err
}

// setupArmLinux sets up the arm profile for linux.
//
// For more on Go cross-compilation for arm/linux information see:
// http://www.bootc.net/archives/2012/05/26/how-to-build-a-cross-compiler-for-your-raspberry-pi/
func setupArmLinux(ctx *util.Context) error {
	root, err := util.VeyronRoot()
	if err != nil {
		return err
	}

	// Install dependencies.
	pkgs := []string{
		"automake", "bison", "bzip2", "curl", "flex", "g++", "gawk",
		"gettext", "gperf", "libncurses5-dev", "libtool", "subversion", "texinfo",
	}
	if err := installDeps(ctx, pkgs); err != nil {
		return err
	}

	// Download and build arm/linux cross-compiler for Go.
	goDir := filepath.Join(root, "environment", "go", "linux", "arm")
	installGoFn := func() error {
		if err := ctx.Run().Function(runutil.MkdirAll(goDir, defaultDirPerm)); err != nil {
			return err
		}
		name := "go1.3.3.src.tar.gz"
		remote, local := "https://storage.googleapis.com/golang/"+name, filepath.Join(goDir, name)
		if err := run(ctx, "curl", []string{"-Lo", local, remote}, nil); err != nil {
			return err
		}
		if err := run(ctx, "tar", []string{"-C", goDir, "-xzf", local}, nil); err != nil {
			return err
		}
		if err := ctx.Run().Function(runutil.RemoveAll(local)); err != nil {
			return err
		}
		goSrcDir := filepath.Join(goDir, "go", "src")
		if err := ctx.Run().Function(runutil.Chdir(goSrcDir)); err != nil {
			return err
		}
		makeBin := filepath.Join(goSrcDir, "make.bash")
		makeArgs := []string{"--no-clean"}
		makeEnv := envutil.NewSnapshotFromOS()
		unsetGoEnv(makeEnv)
		makeEnv.Set("GOARCH", "arm")
		makeEnv.Set("GOAOS", "linux")
		if err := run(ctx, makeBin, makeArgs, makeEnv.Map()); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, installGoFn, goDir, "Download and build Go for arm/linux"); err != nil {
		return err
	}

	// Build and install crosstool-ng.
	xgccOutDir := filepath.Join(root, "environment", "cout", "xgcc")
	installNgFn := func() error {
		xgccSrcDir := filepath.Join(root, "environment", "csrc", "crosstool-ng-1.19.0")
		if err := ctx.Run().Function(runutil.Chdir(xgccSrcDir)); err != nil {
			return err
		}
		if err := run(ctx, "autoreconf", []string{"--install", "--force", "--verbose"}, nil); err != nil {
			return err
		}
		if err := run(ctx, "./configure", []string{fmt.Sprintf("--prefix=%v", xgccOutDir)}, nil); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{fmt.Sprintf("-j%d", runtime.NumCPU())}, nil); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"install"}, nil); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"distclean"}, nil); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, installNgFn, xgccOutDir, "Build and install crosstool-ng"); err != nil {
		return err
	}

	// Build arm/linux gcc tools.
	xgccToolDir := filepath.Join(xgccOutDir, "arm-unknown-linux-gnueabi")
	installXgccFn := func() error {
		dir, prefix := "", ""
		tmpDir, err := ioutil.TempDir(dir, prefix)
		if err != nil {
			return fmt.Errorf("TempDir(%v, %v) failed: %v", dir, prefix, err)
		}
		defer os.RemoveAll(tmpDir)
		if err := ctx.Run().Function(runutil.Chdir(tmpDir)); err != nil {
			return err
		}
		bin := filepath.Join(xgccOutDir, "bin", "ct-ng")
		if err := run(ctx, bin, []string{"arm-unknown-linux-gnueabi"}, nil); err != nil {
			return err
		}
		configFile := filepath.Join(root, "tools", "conf", "crosstool.config")
		config, err := ioutil.ReadFile(configFile)
		if err != nil {
			return fmt.Errorf("ReadFile(%v) failed: %v", configFile, err)
		}
		old, new := "/usr/local/veyron", filepath.Join(root, "environment", "cout")
		newConfig := strings.Replace(string(config), old, new, -1)
		newConfigFile := filepath.Join(tmpDir, ".config")
		if err := ioutil.WriteFile(newConfigFile, []byte(newConfig), defaultFilePerm); err != nil {
			return fmt.Errorf("WriteFile(%v) failed: %v", newConfigFile, err)
		}
		if err := run(ctx, bin, []string{"oldconfig"}, nil); err != nil {
			return err
		}
		if err := run(ctx, bin, []string{"build"}, nil); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, installXgccFn, xgccToolDir, "Build arm/linux gcc tools"); err != nil {
		ctx.Run().Function(runutil.RemoveAll(xgccToolDir))
		return err
	}

	// Create arm/linux gcc symlinks.
	xgccLinkDir := filepath.Join(xgccOutDir, "cross_arm")
	installLinksFn := func() error {
		if err := ctx.Run().Function(runutil.MkdirAll(xgccLinkDir, defaultDirPerm)); err != nil {
			return err
		}
		if err := ctx.Run().Function(runutil.Chdir(xgccLinkDir)); err != nil {
			return err
		}
		binDir := filepath.Join(xgccToolDir, "bin")
		fileInfoList, err := ioutil.ReadDir(binDir)
		if err != nil {
			return fmt.Errorf("ReadDir(%v) failed: %v", binDir, err)
		}
		for _, fileInfo := range fileInfoList {
			prefix := "arm-unknown-linux-gnueabi-"
			if strings.HasPrefix(fileInfo.Name(), prefix) {
				src := filepath.Join(binDir, fileInfo.Name())
				dst := filepath.Join(xgccLinkDir, strings.TrimPrefix(fileInfo.Name(), prefix))
				if err := ctx.Run().Function(runutil.Symlink(src, dst)); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := atomicAction(ctx, installLinksFn, xgccLinkDir, "Create arm/linux gcc symlinks"); err != nil {
		return err
	}

	return nil
}

// setupMobileLinux sets up the mobile profile for linux.
func setupMobileLinux(ctx *util.Context) error {
	root, err := util.VeyronRoot()
	if err != nil {
		return err
	}

	// Install dependencies.
	pkgs := []string{"ant", "autoconf", "bzip2", "default-jdk", "gawk", "lib32z1", "lib32stdc++6"}
	if err := installDeps(ctx, pkgs); err != nil {
		return err
	}

	// Download Java 7 JRE.
	androidRoot := filepath.Join(root, "environment", "android")
	javaDir := filepath.Join(androidRoot, "java")
	jreDir := filepath.Join(javaDir, "jre1.7.0_65")
	installJreFn := func() error {
		if err := ctx.Run().Function(runutil.MkdirAll(javaDir, defaultDirPerm)); err != nil {
			return err
		}
		tmpDir, err := ioutil.TempDir("", "")
		if err != nil {
			fmt.Errorf("TempDir() failed: %v", err)
		}
		defer os.RemoveAll(tmpDir)
		remote := "http://javadl.sun.com/webapps/download/AutoDL?BundleId=92494"
		local := filepath.Join(tmpDir, "jre.tar.gz")
		if err := run(ctx, "curl", []string{"-Lo", local, remote}, nil); err != nil {
			return err
		}
		if err := run(ctx, "tar", []string{"-C", javaDir, "-xzf", local}, nil); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, installJreFn, jreDir, "Download Java 7 JRE"); err != nil {
		return err
	}

	// Download Android SDK.
	sdkRoot := filepath.Join(androidRoot, "android-sdk-linux")
	installSdkFn := func() error {
		tmpDir, err := ioutil.TempDir("", "")
		if err != nil {
			fmt.Errorf("TempDir() failed: %v", err)
		}
		defer os.RemoveAll(tmpDir)
		remote := "http://dl.google.com/android/android-sdk_r23-linux.tgz"
		local := filepath.Join(tmpDir, "android-sdk.tgz")
		if err := run(ctx, "curl", []string{"-Lo", local, remote}, nil); err != nil {
			return err
		}
		if err := run(ctx, "tar", []string{"-C", androidRoot, "-xzf", local}, nil); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, installSdkFn, sdkRoot, "Download Android SDK"); err != nil {
		return err
	}

	// Install Android SDK packagess.
	androidPkgs := []androidPkg{
		androidPkg{"Android SDK Platform-tools", filepath.Join(sdkRoot, "platform-tools")},
		androidPkg{"SDK Platform Android 4.4.2, API 19, revision 4", filepath.Join(sdkRoot, "platforms", "android-19")},
		androidPkg{"Android SDK Build-tools, revision 21.0.2", filepath.Join(sdkRoot, "build-tools")},
		androidPkg{"ARM EABI v7a System Image, Android API 19, revision 2", filepath.Join(sdkRoot, "system-images", "android-19")},
	}
	for _, pkg := range androidPkgs {
		if err := installAndroidPkg(ctx, sdkRoot, pkg); err != nil {
			return err
		}
	}

	// Update Android SDK tools.
	toolPkg := androidPkg{"Android SDK Tools", ""}
	if err := installAndroidPkg(ctx, sdkRoot, toolPkg); err != nil {
		return err
	}

	// Download Android NDK.
	ndkRoot := filepath.Join(androidRoot, "ndk-toolchain")
	installNdkFn := func() error {
		tmpDir, err := ioutil.TempDir("", "")
		if err != nil {
			fmt.Errorf("TempDir() failed: %v", err)
		}
		defer os.RemoveAll(tmpDir)
		remote := "http://dl.google.com/android/ndk/android-ndk-r9d-linux-x86_64.tar.bz2"
		local := filepath.Join(tmpDir, "android-ndk-r9d-linux-x86_64.tar.bz2")
		if err := run(ctx, "curl", []string{"-Lo", local, remote}, nil); err != nil {
			return err
		}
		if err := run(ctx, "tar", []string{"-C", tmpDir, "-xjf", local}, nil); err != nil {
			return err
		}
		ndkBin := filepath.Join(tmpDir, "android-ndk-r9d", "build", "tools", "make-standalone-toolchain.sh")
		ndkArgs := []string{ndkBin, "--platform=android-9", "--install-dir=" + ndkRoot}
		if err := run(ctx, "bash", ndkArgs, nil); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, installNdkFn, ndkRoot, "Download Android NDK"); err != nil {
		return err
	}

	// Download and build Android Go.
	androidGo := filepath.Join(androidRoot, "go")
	installGoFn := func() error {
		if err := ctx.Run().Function(runutil.Chdir(androidRoot)); err != nil {
			return err
		}
		// Get go 1.2.2 (9c4fdd8369ca) because the go android
		// patches do not currently support 1.3. Upgrade to
		// newer versions when the patches are available.
		remote, revision := "https://code.google.com/p/go", "9c4fdd8369ca"
		if err := run(ctx, "hg", []string{"clone", remote, "-r", revision}, nil); err != nil {
			return err
		}
		tmpDir, err := ioutil.TempDir("", "")
		if err != nil {
			fmt.Errorf("TempDir() failed: %v", err)
		}
		defer os.RemoveAll(tmpDir)
		if err := run(ctx, "git", []string{"clone", "https://github.com/eliasnaur/goandroid.git", tmpDir}, nil); err != nil {
			return err
		}

		// Apply our and Android Go patches.
		hgrc := `
[extensions]
mq =
codereview = !

[ui]
username = me<me@mail.com>
`
		hgDir := filepath.Join(androidRoot, "go", ".hg")
		hgrcFile := filepath.Join(hgDir, "hgrc")
		if err := ioutil.WriteFile(hgrcFile, []byte(hgrc), 0644); err != nil {
			return err
		}
		androidGoPatches := filepath.Join(tmpDir, "patches")
		if err := run(ctx, "cp", []string{"-a", androidGoPatches, hgDir}, nil); err != nil {
			return err
		}
		androidPatches := filepath.Join(androidRoot, "patches")
		if err := run(ctx, "cp", []string{"-a", androidPatches, hgDir}, nil); err != nil {
			return err
		}
		androidGoSeries, err := ioutil.ReadFile(filepath.Join(androidGoPatches, "series"))
		if err != nil {
			return fmt.Errorf("ReadFile(%v) failed: %v", filepath.Join(androidGoPatches, "series"), err)
		}
		androidSeries, err := ioutil.ReadFile(filepath.Join(androidPatches, "series"))
		if err != nil {
			return fmt.Errorf("ReadFile(%v) failed: %v", filepath.Join(androidPatches, "series"), err)
		}
		if err := ioutil.WriteFile(filepath.Join(hgDir, "patches", "series"), append(androidGoSeries, androidSeries...), defaultFilePerm); err != nil {
			return fmt.Errorf("WriteFile(%v) failed: %v", filepath.Join(hgDir, "patches", "series"), err)
		}
		if err := ctx.Run().Function(runutil.Chdir(androidGo)); err != nil {
			return err
		}
		if err := run(ctx, "hg", []string{"qpush", "-a"}, nil); err != nil {
			return err
		}

		// Build Android Go.
		srcDir := filepath.Join(androidGo, "src")
		if err := ctx.Run().Function(runutil.Chdir(srcDir)); err != nil {
			return err
		}
		makeEnv := envutil.NewSnapshotFromOS()
		unsetGoEnv(makeEnv)
		makeEnv.Set("CGO_ENABLED", "0")
		makeEnv.Set("GOOS", "linux")
		makeEnv.Set("GOARCH", "arm")
		makeBin := filepath.Join(srcDir, "make.bash")
		makeArgs := []string{
			fmt.Sprintf("CC=%q", filepath.Join(ndkRoot, "bin", "arm-linux-androideabi-gcc")),
			"GOOS=linux", "GOARCH=arm", "GOARM=7", "CGO_ENABLED=1",
			filepath.Join(androidRoot, "go", "bin", "go"),
			"install", "-tags", "android", "-a", "-v", "std",
		}
		if err := run(ctx, makeBin, makeArgs, makeEnv.Map()); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, installGoFn, androidGo, "Download and build Android Go"); err != nil {
		return err
	}

	// Build and install JNI wrapper library.
	jniOutDir := filepath.Join(root, "environment", "cout", "jni-wrapper-1.0", "android")
	installJniFn := func() error {
		jniSrcDir := filepath.Join(root, "environment", "csrc", "jni-wrapper-1.0")
		if err := ctx.Run().Function(runutil.Chdir(jniSrcDir)); err != nil {
			return err
		}
		env := envutil.NewSnapshotFromOS()
		env.Set("CC", filepath.Join(ndkRoot, "bin", "arm-linux-androideabi-gcc"))
		if err := run(ctx, "autoreconf", []string{"--install", "--force", "--verbose"}, env.Map()); err != nil {
			return err
		}
		confArgs := []string{fmt.Sprintf("--prefix=%v", jniOutDir), "--host=arm-unknown-linux-gnu"}
		if err := run(ctx, "./configure", confArgs, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{fmt.Sprintf("-j%d", runtime.NumCPU())}, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"install"}, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"distclean"}, env.Map()); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, installJniFn, jniOutDir, "Build and install JNI wrapper library"); err != nil {
		return err
	}

	return nil
}

// setupProximityLinux sets up the proximity profile for linux.
func setupProximityLinux(ctx *util.Context) error {
	archCmd := exec.Command("uname", "-m")
	out, err := archCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get host architecture: %v\n%v\n%s", err, strings.Join(archCmd.Args, " "))
	}
	return setupProximityLinuxHelper(ctx, strings.TrimSpace(string(out)), "", "")
}

// setupProximityArmLinux sets up the proximity componenets for for arm/linux.
func setupProximityArmLinux(ctx *util.Context) error {
	root, err := util.VeyronRoot()
	if err != nil {
		return err
	}
	path := filepath.Join(root, "environment", "cout", "xgcc", "cross_arm")
	if _, err := os.Stat(path); err != nil {
		return err
	}
	return setupProximityLinuxHelper(ctx, "arm", "arm-unknown-linux-gnu", path)
}

var glibCache = `glib_cv_long_long_format=ll
glib_cv_stack_grows=no
glib_cv_sane_realloc=yes
glib_cv_have_strlcpy=no
glib_cv_va_val_copy=yes
glib_cv_rtldglobal_broken=no
glib_cv_uscore=no
glib_cv_monotonic_clock=no
ac_cv_func_nonposix_getpwuid_r=no
ac_cv_func_posix_getpwuid_r=no
ac_cv_func_posix_getgrgid_r=no
ac_cv_func_qsort_r=no
glib_cv_use_pid_surrogate=yes
ac_cv_func_printf_unix98=no
ac_cv_func_vsnprintf_c99=yes
ac_cv_func_realloc_0_nonnull=yes
ac_cv_func_realloc_works=yes
`

// setupProximityLinuxHelper sets up the proximity profile for linux.
func setupProximityLinuxHelper(ctx *util.Context, arch, host, path string) error {
	root, err := util.VeyronRoot()
	if err != nil {
		return err
	}

	// Install dependencies.
	pkgs := []string{
		"automake", "byacc", "flex", "gettext", "libdbus-1-dev", "libglib2.0-dev",
		"libtool", "libusb-dev", "libusb-1.0-0-dev", "pkg-config", "texinfo",
	}
	if err := installDeps(ctx, pkgs); err != nil {
		return err
	}

	// Build and install expat.
	expatOutDir := filepath.Join(root, "environment", "cout", "expat-2.1.0", string(arch))
	installExpatFn := func() error {
		expatSrcDir := filepath.Join(root, "environment", "csrc", "expat-2.1.0")
		if err := ctx.Run().Function(runutil.Chdir(expatSrcDir)); err != nil {
			return err
		}
		env := envutil.NewSnapshotFromOS()
		if path != "" {
			env.Set("PATH", fmt.Sprintf("%s:%s", path, env.Get("PATH")))
		}
		confArgs := []string{fmt.Sprintf("--prefix=%v", expatOutDir)}
		if host != "" {
			confArgs = append(confArgs, "--host="+host)
		}
		if err := run(ctx, "./configure", confArgs, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{fmt.Sprintf("-j%d", runtime.NumCPU())}, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"install"}, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"distclean"}, env.Map()); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, installExpatFn, expatOutDir, "Build and install expat"); err != nil {
		return err
	}

	// Build and install dbus.
	dbusOutDir := filepath.Join(root, "environment", "cout", "dbus-1.6.14", string(arch))
	installDbusFn := func() error {
		dbusSrcDir := filepath.Join(root, "environment", "csrc", "dbus-1.6.14")
		if err := ctx.Run().Function(runutil.Chdir(dbusSrcDir)); err != nil {
			return err
		}
		env := envutil.NewSnapshotFromOS()
		if path != "" {
			env.Set("PATH", fmt.Sprintf("%s:%s", path, env.Get("PATH")))
		}
		env.Set("CFLAGS", fmt.Sprintf("%s -I%s/include", env.Get("CFLAGS"), expatOutDir))
		env.Set("LDFLAGS", fmt.Sprintf("%s -L%s/lib -lexpat", env.Get("LDFLAGS"), expatOutDir))
		if err := run(ctx, "autoreconf", []string{"--install", "--force", "--verbose"}, env.Map()); err != nil {
			return err
		}
		confArgs := []string{fmt.Sprintf("--prefix=%v", dbusOutDir)}
		if host != "" {
			confArgs = append(confArgs, "--host="+host)
		}
		if err := run(ctx, "./configure", confArgs, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{fmt.Sprintf("-j%d", runtime.NumCPU())}, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"install"}, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"distclean"}, env.Map()); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, installDbusFn, dbusOutDir, "Build and install dbus"); err != nil {
		return err
	}

	// Build and install libffi.
	libffiOutDir := filepath.Join(root, "environment", "cout", "libffi-3.0.13", string(arch))
	installLibffiFn := func() error {
		libffiSrcDir := filepath.Join(root, "environment", "csrc", "libffi-3.0.13")
		if err := ctx.Run().Function(runutil.Chdir(libffiSrcDir)); err != nil {
			return err
		}
		env := envutil.NewSnapshotFromOS()
		if path != "" {
			env.Set("PATH", fmt.Sprintf("%s:%s", path, env.Get("PATH")))
		}
		confArgs := []string{fmt.Sprintf("--prefix=%v", libffiOutDir)}
		if host != "" {
			confArgs = append(confArgs, "--host="+host)
		}
		if err := run(ctx, "autoreconf", []string{"--install", "--force", "--verbose"}, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "./configure", confArgs, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{fmt.Sprintf("-j%d", runtime.NumCPU())}, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"install"}, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"distclean"}, env.Map()); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, installLibffiFn, libffiOutDir, "Build and install libffi"); err != nil {
		return err
	}

	// Build and install zlib.
	zlibOutDir := filepath.Join(root, "environment", "cout", "zlib-1.2.8", string(arch))
	installZlibFn := func() error {
		zlibSrcDir := filepath.Join(root, "environment", "csrc", "zlib-1.2.8")
		if err := ctx.Run().Function(runutil.Chdir(zlibSrcDir)); err != nil {
			return err
		}
		env := envutil.NewSnapshotFromOS()
		if path != "" {
			env.Set("PATH", fmt.Sprintf("%s:%s", path, env.Get("PATH")))
		}
		confArgs := []string{fmt.Sprintf("--prefix=%v", zlibOutDir)}
		if err := run(ctx, "./configure", confArgs, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{fmt.Sprintf("-j%d", runtime.NumCPU())}, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"install"}, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"distclean"}, env.Map()); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, installZlibFn, zlibOutDir, "Build and install zlib"); err != nil {
		return err
	}

	// Build and install glib.
	glibOutDir := filepath.Join(root, "environment", "cout", "glib-2.28.8", string(arch))
	installGlibFn := func() error {
		glibSrcDir := filepath.Join(root, "environment", "csrc", "glib-2.28.8")
		if err := ctx.Run().Function(runutil.Chdir(glibSrcDir)); err != nil {
			return err
		}
		glibCacheFile := filepath.Join(glibSrcDir, "glib.cache")
		if err := ioutil.WriteFile(glibCacheFile, []byte(glibCache), defaultFilePerm); err != nil {
			return fmt.Errorf("WriteFile(%v) failed: %v", glibCacheFile, err)
		}
		defer os.Remove(glibCacheFile)
		env := envutil.NewSnapshotFromOS()
		if path != "" {
			env.Set("PATH", fmt.Sprintf("%s:%s", path, env.Get("PATH")))
		}
		env.Set("CFLAGS", fmt.Sprintf("%s -I%s/include/dbus-1.0/dbus -I%s/lib/libffi-3.0.13/include -I%s/include",
			env.Get("CFLAGS"), dbusOutDir, libffiOutDir, zlibOutDir))
		env.Set("LDFLAGS", fmt.Sprintf("%s -L%s/lib -L%s/lib -L%s/lib -ldbus-1 -lz",
			env.Get("LDFLAGS"), dbusOutDir, libffiOutDir, zlibOutDir))
		env.Set("LD_LIBRARY_PATH", fmt.Sprintf("%s:%s/lib:%s/lib:%s/lib", env.Get("LD_LIBRARY_PATH"), dbusOutDir, libffiOutDir, zlibOutDir))
		env.Set("PKG_CONFIG_PATH", fmt.Sprintf("%s:%s/lib/pkgconfig", env.Get("PKG_CONFIG_PATH"), libffiOutDir))
		if err := run(ctx, "autoreconf", []string{"--install", "--force", "--verbose"}, env.Map()); err != nil {
			return err
		}
		confEnv := envutil.NewSnapshot(env.Map())
		confEnv.Set("NM", "nm")
		confArgs := []string{fmt.Sprintf("--prefix=%v", glibOutDir), "--enable-static", "--cache-file=glib.cache"}
		if host != "" {
			confArgs = append(confArgs, "--host="+host)
		}
		if err := run(ctx, "./configure", confArgs, confEnv.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{fmt.Sprintf("-j%d", runtime.NumCPU())}, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"install"}, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"distclean"}, env.Map()); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, installGlibFn, glibOutDir, "Build and install glib"); err != nil {
		return err
	}

	// Build and install libusb.
	libusbOutDir := filepath.Join(root, "environment", "cout", "libusb-1.0.16-rc10", string(arch))
	installLibusbFn := func() error {
		libusbSrcDir := filepath.Join(root, "environment", "csrc", "libusb-1.0.16-rc10")
		if err := ctx.Run().Function(runutil.Chdir(libusbSrcDir)); err != nil {
			return err
		}
		env := envutil.NewSnapshotFromOS()
		if path != "" {
			env.Set("PATH", fmt.Sprintf("%s:%s", path, env.Get("PATH")))
		}
		env.Set("LDFLAGS", fmt.Sprintf("%s -lrt", env.Get("LDFLAGS")))
		if err := run(ctx, "autoreconf", []string{"--install", "--force", "--verbose"}, env.Map()); err != nil {
			return err
		}
		confArgs := []string{fmt.Sprintf("--prefix=%v", libusbOutDir), "--disable-udev"}
		if host != "" {
			confArgs = append(confArgs, "--host="+host)
		}
		if err := run(ctx, "./configure", confArgs, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{fmt.Sprintf("-j%d", runtime.NumCPU())}, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"install"}, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"distclean"}, env.Map()); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, installLibusbFn, libusbOutDir, "Build and install libusb"); err != nil {
		return err
	}

	// Build and install libusb-compat.
	libusbCompatOutDir := filepath.Join(root, "environment", "cout", "libusb-compat-0.1.5", string(arch))
	installLibusbCompatFn := func() error {
		libusbCompatSrcDir := filepath.Join(root, "environment", "csrc", "libusb-compat-0.1.5")
		if err := ctx.Run().Function(runutil.Chdir(libusbCompatSrcDir)); err != nil {
			return err
		}
		env := envutil.NewSnapshotFromOS()
		if path != "" {
			env.Set("PATH", fmt.Sprintf("%s:%s", path, env.Get("PATH")))
		}
		env.Set("LDFLAGS", fmt.Sprintf("%s -L%s/lib", env.Get("LDFLAGS"), libusbOutDir))
		env.Set("PKG_CONFIG_PATH", fmt.Sprintf("%s:%s/lib/pkgconfig", env.Get("PKG_CONFIG_PATH"), libusbOutDir))
		if err := run(ctx, "autoreconf", []string{"--install", "--force", "--verbose"}, env.Map()); err != nil {
			return err
		}
		confArgs := []string{fmt.Sprintf("--prefix=%v", libusbCompatOutDir), "--disable-udev"}
		if host != "" {
			confArgs = append(confArgs, "--host="+host)
		}
		if err := run(ctx, "./configure", confArgs, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{fmt.Sprintf("-j%d", runtime.NumCPU())}, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"install"}, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"distclean"}, env.Map()); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, installLibusbCompatFn, libusbCompatOutDir, "Build and install libusb-compat"); err != nil {
		return err
	}

	// Build and install bluez.
	bluezOutDir := filepath.Join(root, "environment", "cout", "bluez-4.101", string(arch))
	installBluezFn := func() error {
		bluezSrcDir := filepath.Join(root, "environment", "csrc", "bluez-4.101")
		if err := ctx.Run().Function(runutil.Chdir(bluezSrcDir)); err != nil {
			return err
		}
		env := envutil.NewSnapshotFromOS()
		if path != "" {
			env.Set("PATH", fmt.Sprintf("%s:%s", path, env.Get("PATH")))
		}
		env.Set("CFLAGS", fmt.Sprintf("%s -I%s/include/dbus-1.0/dbus -I%s/include -I%s/include",
			env.Get("CFLAGS"), dbusOutDir, libusbOutDir, libusbCompatOutDir))
		env.Set("LDFLAGS", fmt.Sprintf("%s -L%s/lib -L%s/lib -L%s/lib -ldbus-1 -lusb-1.0 -lusb",
			env.Get("LDFLAGS"), dbusOutDir, libusbOutDir, libusbCompatOutDir))
		env.Set("LD_LIBRARY_PATH", fmt.Sprintf("%s:%s/lib:%s/lib:%s/lib:%s/lib",
			env.Get("LD_LIBRARY_PATH"), dbusOutDir, glibOutDir, libusbOutDir, libusbCompatOutDir))
		env.Set("PKG_CONFIG_PATH", fmt.Sprintf("%s:%s/lib/pkgconfig", env.Get("PKG_CONFIG_PATH"), glibOutDir))
		if err := run(ctx, "autoreconf", []string{"--install", "--force", "--verbose"}, env.Map()); err != nil {
			return err
		}
		confArgs := []string{
			fmt.Sprintf("--prefix=%v", bluezOutDir), "--enable-static",
			"--enable-alsa=false", "--disable-audio", "--enable-shared=false",
		}
		if host != "" {
			confArgs = append(confArgs, "--host="+host)
		}
		if err := run(ctx, "./configure", confArgs, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{fmt.Sprintf("-j%d", runtime.NumCPU())}, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"install"}, env.Map()); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"distclean"}, env.Map()); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, installBluezFn, bluezOutDir, "Build and install bluez"); err != nil {
		return err
	}

	return nil
}

// setupWebDarwin sets up the web profile for darwin.
func setupWebDarwin(ctx *util.Context) error {
	return setupWebHelper(ctx)
}

// setupWebLinux sets up the web profile for linux
func setupWebLinux(ctx *util.Context) error {
	// Install dependencies.
	pkgs := []string{"g++", "libc6-i386"}
	if err := installDeps(ctx, pkgs); err != nil {
		return err
	}

	return setupWebHelper(ctx)
}

// setupWebHelper sets up the web profile.
func setupWebHelper(ctx *util.Context) error {
	root, err := util.VeyronRoot()
	if err != nil {
		return err
	}

	// Build and install NodeJS.
	nodeOutDir := filepath.Join(root, "environment", "cout", "node")
	installNodeFn := func() error {
		nodeSrcDir := filepath.Join(root, "environment", "csrc", "node-v0.10.24")
		if err := ctx.Run().Function(runutil.Chdir(nodeSrcDir)); err != nil {
			return err
		}
		if err := run(ctx, "./configure", []string{fmt.Sprintf("--prefix=%v", nodeOutDir)}, nil); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{fmt.Sprintf("-j%d", runtime.NumCPU())}, nil); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"install"}, nil); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, installNodeFn, nodeOutDir, "Build and install NodeJS"); err != nil {
		return err
	}
	return nil
}

func unsetGoEnv(env *envutil.Snapshot) {
	env.Set("CGO_ENABLED", "")
	env.Set("CGO_CFLAGS", "")
	env.Set("CGO_CGO_LDFLAGS", "")
	env.Set("GOARCH", "")
	env.Set("GOBIN", "")
	env.Set("GOOS", "")
	env.Set("GOPATH", "")
	env.Set("GOROOT", "")
}
