// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// package mojo_profile implements the mojo profile.
package mojo_profile

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"v.io/jiri/gitutil"
	"v.io/jiri/jiri"
	"v.io/jiri/profiles"
	"v.io/jiri/profiles/profilesmanager"
	"v.io/jiri/profiles/profilesutil"
	"v.io/x/lib/envvar"
)

const (
	// On every green Mojo build (defined as compiling and passing the mojo
	// tests) a set of build artifacts are published to publicly-readable
	// Google cloud storage buckets.
	mojoStorageBucket = "https://storage.googleapis.com/mojo"

	// The mojo devtools repo has tools for running, debugging and testing mojo apps.
	mojoDevtoolsRemote = "https://github.com/domokit/devtools"

	// The mojo_sdk repo is a mirror of github.com/domokit/mojo/mojo/public.
	// It is mirrored for easy consumption.
	mojoSdkRemote = "https://github.com/domokit/mojo_sdk.git"

	// The main mojo repo.  We should not need this, but currently the service
	// mojom files live here and are not mirrored anywhere else.
	// TODO(nlacasse): Once the service mojoms exist elsewhere, remove this and
	// get the service mojoms from wherever they are.
	mojoRemote = "https://github.com/domokit/mojo.git"
)

type versionSpec struct {
	// Version of android platform tools.  See http://tools.android.com/download.
	androidPlatformToolsVersion string

	// The names of the mojo services to install for all targets.
	serviceNames []string

	// The names of additional mojo services to install for android targets.
	serviceNamesAndroid []string

	// The names of additional mojo services to install for linux targets.
	serviceNamesLinux []string

	// The git SHA of the mojo artifacts, including the mojo shell and system
	// thunks to install on android targets.
	// The latest can be found in
	// https://storage.googleapis.com/mojo/shell/android-arm/LATEST.
	buildVersionAndroid string

	// The git SHA of the mojo artifacts, including the mojo shell and system
	// thunks to install on linux targets.
	// The latest can be found in
	// https://storage.googleapis.com/mojo/shell/linux-x64/LATEST.
	buildVersionLinux string

	// The git SHA, branch, or tag of the devtools repo to checkout.
	devtoolsVersion string

	// The git SHA of the mojo network service.  The latest can be found in
	// https://github.com/domokit/mojo/blob/master/mojo/public/tools/NETWORK_SERVICE_VERSION
	networkServiceVersion string

	// The git SHA, branch, or tag of the the mojo_sdk repo to checkout.
	sdkVersion string
}

func Register(installer, profile string) {
	m := &Manager{
		profileInstaller: installer,
		profileName:      profile,
		qualifiedName:    profiles.QualifiedProfileName(installer, profile),
		versionInfo: profiles.NewVersionInfo(profile, map[string]interface{}{
			"1": &versionSpec{
				serviceNames: []string{
					"authenticating_url_loader_interceptor.mojo",
					"dart_content_handler.mojo",
					"debugger.mojo",
					"kiosk_wm.mojo",
					"tracing.mojo",
				},
				serviceNamesAndroid: []string{},
				serviceNamesLinux: []string{
					"authentication.mojo",
				},
				buildVersionAndroid:         "e2cd09460972dab4d1766153e108457fe5bbaed5",
				buildVersionLinux:           "e2cd09460972dab4d1766153e108457fe5bbaed5",
				devtoolsVersion:             "a264dd5ebdb5508d4e7e432b0ee3dcf6b1fb7160",
				networkServiceVersion:       "0a814ed5512598e595c0ae7975a09d90a7a54e90",
				sdkVersion:                  "b3af6aeeea02c07e7ccb2c672a0ebcda0d6c42b4",
				androidPlatformToolsVersion: "2219198",
			},
			"2": &versionSpec{
				serviceNames: []string{
					"authenticating_url_loader_interceptor.mojo",
					"dart_content_handler.mojo",
					"debugger.mojo",
					"kiosk_wm.mojo",
					"tracing.mojo",
				},
				serviceNamesAndroid: []string{},
				serviceNamesLinux: []string{
					"authentication.mojo",
				},
				buildVersionAndroid:         "8e8ac2169f29ee1349f7ada64e2d98466d5f5205",
				buildVersionLinux:           "8e8ac2169f29ee1349f7ada64e2d98466d5f5205",
				devtoolsVersion:             "a264dd5ebdb5508d4e7e432b0ee3dcf6b1fb7160",
				networkServiceVersion:       "0a814ed5512598e595c0ae7975a09d90a7a54e90",
				sdkVersion:                  "8e3eb6e43c82af5b9c57870003138ea165209d81",
				androidPlatformToolsVersion: "2219198",
			},
			"3": &versionSpec{
				serviceNames: []string{
					"authenticating_url_loader_interceptor.mojo",
					"dart_content_handler.mojo",
					"debugger.mojo",
					"kiosk_wm.mojo",
					"tracing.mojo",
				},
				serviceNamesAndroid: []string{},
				serviceNamesLinux: []string{
					"authentication.mojo",
				},
				buildVersionAndroid:         "1608780b78ac467dcbf1761ebc0359739c3c6bbd",
				buildVersionLinux:           "1608780b78ac467dcbf1761ebc0359739c3c6bbd",
				devtoolsVersion:             "a264dd5ebdb5508d4e7e432b0ee3dcf6b1fb7160",
				networkServiceVersion:       "0a814ed5512598e595c0ae7975a09d90a7a54e90",
				sdkVersion:                  "8e3eb6e43c82af5b9c57870003138ea165209d81",
				androidPlatformToolsVersion: "2219198",
			},
			"4": &versionSpec{
				serviceNames: []string{
					"authenticating_url_loader_interceptor.mojo",
					"dart_content_handler.mojo",
					"debugger.mojo",
					"kiosk_wm.mojo",
					"tracing.mojo",
				},
				serviceNamesAndroid: []string{
					"shortcut.mojo",
				},
				serviceNamesLinux: []string{
					"authentication.mojo",
				},
				buildVersionAndroid:         "e93037e1a5b2d84d2df3be87579f86f73d842449",
				buildVersionLinux:           "e93037e1a5b2d84d2df3be87579f86f73d842449",
				devtoolsVersion:             "1185c2a2bb45c27cbcf281cb538bcb5ba4720fea",
				networkServiceVersion:       "0a814ed5512598e595c0ae7975a09d90a7a54e90",
				sdkVersion:                  "ab83ef213a4fb310e7de5d617046e9e4120efb75",
				androidPlatformToolsVersion: "2219198",
			},
			"5": &versionSpec{
				serviceNames: []string{
					"authenticating_url_loader_interceptor.mojo",
					"dart_content_handler.mojo",
					"debugger.mojo",
					"files.mojo",
					"kiosk_wm.mojo",
					"tracing.mojo",
				},
				serviceNamesAndroid: []string{
					"shortcut.mojo",
				},
				serviceNamesLinux: []string{
					"authentication.mojo",
				},
				buildVersionAndroid:         "25d53bf53b4040f29db984d4774483c5de9d2dc5",
				buildVersionLinux:           "a8adeba2e48f2fc5f8a89b39dc637dbab474bab7",
				devtoolsVersion:             "f71528bb1d9d9b9f874ce503b3cf3d7532283eb5",
				networkServiceVersion:       "0a814ed5512598e595c0ae7975a09d90a7a54e90",
				sdkVersion:                  "832aa6a651b4468c4d0c0025bca7605bc248f82b",
				androidPlatformToolsVersion: "2219198",
			},
			"6": &versionSpec{
				serviceNames: []string{
					"authenticating_url_loader_interceptor.mojo",
					"dart_content_handler.mojo",
					"debugger.mojo",
					"files.mojo",
					"kiosk_wm.mojo",
					"tracing.mojo",
				},
				serviceNamesAndroid: []string{
					"shortcut.mojo",
				},
				serviceNamesLinux: []string{
					"authentication.mojo",
				},
				buildVersionAndroid:         "630247e6afeaf9177ff50396988700e787c51440",
				buildVersionLinux:           "a49d2b24078041ffa78e7e8d9c72ffed213f7881",
				devtoolsVersion:             "5e3dadf261aa264d885b699cee874b3e81393ddc",
				networkServiceVersion:       "0a814ed5512598e595c0ae7975a09d90a7a54e90",
				sdkVersion:                  "309af0509fb2fd824e81e2e14a63e138e2eeb30c",
				androidPlatformToolsVersion: "2219198",
			},
		}, "6"),
	}
	profilesmanager.Register(m)
}

type Manager struct {
	profileInstaller, profileName, qualifiedName string
	root, mojoRoot                               jiri.RelPath
	mojoInstDir, androidPlatformToolsDir         jiri.RelPath
	devtoolsDir, sdkDir                          jiri.RelPath
	shellDir, systemThunksDir                    jiri.RelPath
	versionInfo                                  *profiles.VersionInfo
	spec                                         versionSpec
	buildVersion                                 string
	platform                                     string
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

func (m Manager) VersionInfo() *profiles.VersionInfo {
	return m.versionInfo
}

func (m Manager) Info() string {
	return `Downloads pre-built mojo binaries and other assets required for building mojo services.`
}

func (m *Manager) AddFlags(flags *flag.FlagSet, action profiles.Action) {
}

func (m *Manager) initForTarget(jirix *jiri.X, root jiri.RelPath, target *profiles.Target) error {
	if err := m.versionInfo.Lookup(target.Version(), &m.spec); err != nil {
		return err
	}

	// Turn "amd64" architecture string into "x64" to match mojo's usage.
	mojoArch := target.Arch()
	if mojoArch == "amd64" {
		mojoArch = "x64"
	}
	m.platform = target.OS() + "-" + mojoArch

	if m.platform != "linux-x64" && m.platform != "android-arm" {
		return fmt.Errorf("only amd64-linux and arm-android targets are supported for mojo profile")
	}

	m.buildVersion = m.spec.buildVersionLinux
	if m.platform == "android-arm" {
		m.buildVersion = m.spec.buildVersionAndroid
	}

	m.root = root
	m.mojoRoot = root.Join("mojo")

	// devtools and mojo sdk are not architecture-dependant, so they can go in
	// mojoRoot.
	m.devtoolsDir = m.mojoRoot.Join("devtools", m.spec.devtoolsVersion)
	m.sdkDir = m.mojoRoot.Join("mojo_sdk", m.spec.sdkVersion)

	m.mojoInstDir = m.mojoRoot.Join(target.TargetSpecificDirname())
	m.androidPlatformToolsDir = m.mojoInstDir.Join("platform-tools", m.spec.androidPlatformToolsVersion)
	m.shellDir = m.mojoInstDir.Join("mojo_shell", m.buildVersion)
	m.systemThunksDir = m.mojoInstDir.Join("system_thunks", m.buildVersion)

	if jirix.Verbose() {
		fmt.Fprintf(jirix.Stdout(), "Installation Directories for: %s\n", target)
		fmt.Fprintf(jirix.Stdout(), "mojo installation dir: %s\n", m.mojoInstDir)
		fmt.Fprintf(jirix.Stdout(), "devtools: %s\n", m.devtoolsDir)
		fmt.Fprintf(jirix.Stdout(), "sdk: %s\n", m.sdkDir)
		fmt.Fprintf(jirix.Stdout(), "shell: %s\n", m.shellDir)
		fmt.Fprintf(jirix.Stdout(), "system thunks: %s\n", m.systemThunksDir)
		fmt.Fprintf(jirix.Stdout(), "android platform tools: %s\n", m.androidPlatformToolsDir)
	}
	return nil
}

func (m *Manager) Install(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	if err := m.initForTarget(jirix, root, &target); err != nil {
		return err
	}

	seq := jirix.NewSeq()
	seq.MkdirAll(m.mojoInstDir.Abs(jirix), profilesutil.DefaultDirPerm).
		Call(func() error { return m.installMojoDevtools(jirix, m.devtoolsDir.Abs(jirix)) }, "install mojo devtools").
		Call(func() error { return m.installMojoSdk(jirix, m.sdkDir.Abs(jirix)) }, "install mojo SDK").
		Call(func() error { return m.installMojoShellAndServices(jirix, m.shellDir.Abs(jirix)) }, "install mojo shell and services").
		Call(func() error { return m.installMojoSystemThunks(jirix, m.systemThunksDir.Abs(jirix)) }, "install mojo system thunks")

	target.Env.Vars = envvar.MergeSlices(target.Env.Vars, []string{
		"CGO_CFLAGS=-I" + m.sdkDir.Join("src").Symbolic(),
		"CGO_CXXFLAGS=-I" + m.sdkDir.Join("src").Symbolic(),
		"CGO_LDFLAGS=-L" + m.systemThunksDir.Symbolic() + " -lsystem_thunks",
		"GOPATH=" + m.sdkDir.Symbolic() + ":" + m.sdkDir.Join("gen", "go").Symbolic(),
		"MOJO_DEVTOOLS=" + m.devtoolsDir.Symbolic(),
		"MOJO_SDK=" + m.sdkDir.Symbolic(),
		"MOJO_SHELL=" + m.shellDir.Join("mojo_shell").Symbolic(),
		"MOJO_SERVICES=" + m.shellDir.Symbolic(),
		"MOJO_SYSTEM_THUNKS=" + m.systemThunksDir.Join("libsystem_thunks.a").Symbolic(),
	})

	if m.platform == "android-arm" {
		seq.Call(func() error { return m.installAndroidPlatformTools(jirix, m.androidPlatformToolsDir.Abs(jirix)) }, "install android platform tools")
		target.Env.Vars = envvar.MergeSlices(target.Env.Vars, []string{
			"ANDROID_PLATFORM_TOOLS=" + m.androidPlatformToolsDir.Symbolic(),
			"MOJO_SHELL=" + m.shellDir.Join("MojoShell.apk").Symbolic(),
		})
	}

	if err := seq.Done(); err != nil {
		return err
	}

	target.InstallationDir = string(m.mojoInstDir)
	pdb.InstallProfile(m.profileInstaller, m.profileName, string(m.mojoRoot))
	return pdb.AddProfileTarget(m.profileInstaller, m.profileName, target)
}

// installAndroidPlatformTools installs the android platform tools in outDir.
func (m *Manager) installAndroidPlatformTools(jirix *jiri.X, outDir string) error {
	tmpDir, err := jirix.NewSeq().TempDir("", "")
	if err != nil {
		return err
	}
	defer jirix.NewSeq().RemoveAll(tmpDir).Done()

	fn := func() error {
		androidPlatformToolsZipFile := filepath.Join(tmpDir, "platform-tools.zip")
		return jirix.NewSeq().
			Call(func() error {
			return profilesutil.Fetch(jirix, androidPlatformToolsZipFile, androidPlatformToolsUrl(m.spec.androidPlatformToolsVersion))
		}, "fetch android platform tools").
			Call(func() error { return profilesutil.Unzip(jirix, androidPlatformToolsZipFile, tmpDir) }, "unzip android platform tools").
			MkdirAll(filepath.Dir(outDir), profilesutil.DefaultDirPerm).
			Rename(filepath.Join(tmpDir, "platform-tools"), outDir).
			Done()
	}
	return profilesutil.AtomicAction(jirix, fn, outDir, "Install Android Platform Tools")
}

// installMojoNetworkService installs network_services.mojo into outDir.
func (m *Manager) installMojoNetworkService(jirix *jiri.X, outDir string) error {
	tmpDir, err := jirix.NewSeq().TempDir("", "")
	if err != nil {
		return err
	}
	defer jirix.NewSeq().RemoveAll(tmpDir).Done()

	networkServiceUrl := mojoNetworkServiceUrl(m.platform, m.spec.networkServiceVersion)
	networkServiceZipFile := filepath.Join(tmpDir, "network_service.mojo.zip")
	tmpFile := filepath.Join(tmpDir, "network_service.mojo")
	outFile := filepath.Join(outDir, "network_service.mojo")

	return jirix.NewSeq().
		Call(func() error { return profilesutil.Fetch(jirix, networkServiceZipFile, networkServiceUrl) }, "fetch %s", networkServiceUrl).
		Call(func() error { return profilesutil.Unzip(jirix, networkServiceZipFile, tmpDir) }, "unzip network service").
		MkdirAll(filepath.Dir(outDir), profilesutil.DefaultDirPerm).
		Rename(tmpFile, outFile).
		Done()
}

// installMojoDevtools clones the mojo devtools repo into outDir.
func (m *Manager) installMojoDevtools(jirix *jiri.X, outDir string) error {
	fn := func() error {
		return jirix.NewSeq().
			MkdirAll(outDir, profilesutil.DefaultDirPerm).
			Pushd(outDir).
			Call(func() error { return gitutil.New(jirix.NewSeq()).CloneRecursive(mojoDevtoolsRemote, outDir) }, "git clone --recursive %s", mojoDevtoolsRemote).
			Call(func() error { return gitutil.New(jirix.NewSeq()).Reset(m.spec.devtoolsVersion) }, "git reset --hard %s", m.spec.devtoolsVersion).
			Popd().
			Done()
	}
	return profilesutil.AtomicAction(jirix, fn, outDir, "Install Mojo devtools")
}

// installMojoSdk clones the mojo_sdk repo into outDir/src/mojo/public.  It
// also generates .mojom.go files from the .mojom files.
func (m *Manager) installMojoSdk(jirix *jiri.X, outDir string) error {
	fn := func() error {
		seq := jirix.NewSeq()
		// TODO(nlacasse): At some point Mojo needs to change the structure of
		// their repo so that go packages occur with correct paths. Until then
		// we'll clone into src/mojo/public so that go import paths work.
		repoDst := filepath.Join(outDir, "src", "mojo", "public")
		seq.
			MkdirAll(repoDst, profilesutil.DefaultDirPerm).
			Pushd(repoDst).
			Call(func() error { return gitutil.New(jirix.NewSeq()).CloneRecursive(mojoSdkRemote, repoDst) }, "git clone --recursive %s", mojoSdkRemote).
			Call(func() error { return gitutil.New(jirix.NewSeq()).Reset(m.spec.sdkVersion) }, "git reset --hard %s", m.spec.sdkVersion).
			Popd()

		// Download the authentication and network service mojom files.
		// TODO(nlacasse): This is a HACK.  The service mojom files are not
		// published anywhere yet, so we get them from the main mojo repo,
		// which we should not need to do.  Once they are published someplace
		// else, get them from there.
		tmpMojoCheckout, err := jirix.NewSeq().TempDir("", "")
		if err != nil {
			return err
		}
		defer jirix.NewSeq().RemoveAll(tmpMojoCheckout).Done()

		seq.
			Pushd(tmpMojoCheckout).
			Call(func() error { return gitutil.New(jirix.NewSeq()).Clone(mojoRemote, tmpMojoCheckout) }, "git clone %s", mojoRemote).
			Call(func() error { return gitutil.New(jirix.NewSeq()).Reset(m.buildVersion) }, "git reset --hard %s", m.buildVersion).
			Popd()

		servicesSrc := filepath.Join(tmpMojoCheckout, "mojo", "services")
		servicesDst := filepath.Join(outDir, "src", "mojo", "services")
		seq.Rename(servicesSrc, servicesDst)

		// Find all .mojom files.
		var mojomFilesBuffer bytes.Buffer
		if err := jirix.NewSeq().Capture(&mojomFilesBuffer, nil).Last("find", outDir, "-name", "*.mojom"); err != nil {
			return err
		}
		mojomFiles := strings.Split(mojomFilesBuffer.String(), "\n")

		// Generate the mojom.go files from all mojom files.
		seq.Pushd(filepath.Join(outDir, "src"))
		// Fetch the mojom compiler
		compilerDir := filepath.Join(outDir, "src", "mojo", "public", "tools", "bindings", "mojom_parser", "bin")

		fetchCompiler := func(arch string) error {
			hash, err := ioutil.ReadFile(filepath.Join(compilerDir, arch, "mojom_parser.sha1"))
			if err != nil {
				return err
			}
			binary := mojoCompilerUrl(arch, string(hash))
			return profilesutil.Fetch(jirix, filepath.Join(compilerDir, arch, "mojom_parser"), binary)
		}
		seq.
			Call(func() error { return fetchCompiler("linux64") }, "fetch linux64 mojom compiler").
			Chmod(filepath.Join(compilerDir, "linux64", "mojom_parser"), 0755).
			Call(func() error { return fetchCompiler("mac64") }, "fetch mac64 mojom compiler").
			Chmod(filepath.Join(compilerDir, "mac64", "mojom_parser"), 0755)
		genMojomTool := filepath.Join(outDir, "src", "mojo", "public", "tools", "bindings", "mojom_bindings_generator.py")
		for _, mojomFile := range mojomFiles {
			trimmedFile := strings.TrimSpace(mojomFile)
			if trimmedFile == "" {
				continue
			}
			seq.Run(genMojomTool,
				"--use_bundled_pylibs",
				"-g", "go",
				"-o", filepath.Join("..", "gen"),
				"-I", ".",
				"-I", servicesDst,
				"--generate-type-info",
				trimmedFile)
		}
		seq.Popd()

		return seq.Done()
	}

	return profilesutil.AtomicAction(jirix, fn, outDir, "Clone Mojo SDK repository")
}

// installMojoShellAndServices installs the mojo shell and all services into outDir.
func (m *Manager) installMojoShellAndServices(jirix *jiri.X, outDir string) error {
	tmpDir, err := jirix.NewSeq().TempDir("", "")
	if err != nil {
		return err
	}
	defer jirix.NewSeq().RemoveAll(tmpDir).Done()

	fn := func() error {
		seq := jirix.NewSeq()
		seq.MkdirAll(outDir, profilesutil.DefaultDirPerm)

		// Install mojo shell.
		url := mojoShellUrl(m.platform, m.buildVersion)
		mojoShellZipFile := filepath.Join(tmpDir, "mojo_shell.zip")
		seq.
			Call(func() error { return profilesutil.Fetch(jirix, mojoShellZipFile, url) }, "fetch %s", url).
			Call(func() error { return profilesutil.Unzip(jirix, mojoShellZipFile, tmpDir) }, "unzip %s", mojoShellZipFile)

		files := []string{"mojo_shell", "mojo_shell_child"}
		if m.platform == "android-arm" {
			// On android, mojo shell is called "MojoShell.apk".
			files = []string{"MojoShell.apk"}
		}
		for _, file := range files {
			tmpFile := filepath.Join(tmpDir, file)
			outFile := filepath.Join(outDir, file)
			seq.Rename(tmpFile, outFile)
		}

		// Install the network services.
		seq.Call(func() error {
			return m.installMojoNetworkService(jirix, outDir)
		}, "install mojo network service")

		// Install all other services.
		serviceNames := m.spec.serviceNames
		if m.platform == "android-arm" {
			serviceNames = append(serviceNames, m.spec.serviceNamesAndroid...)
		}
		if m.platform == "linux-x64" {
			serviceNames = append(serviceNames, m.spec.serviceNamesLinux...)
		}
		for _, serviceName := range serviceNames {
			outFile := filepath.Join(outDir, serviceName)
			serviceUrl := mojoServiceUrl(m.platform, serviceName, m.buildVersion)
			seq.Call(func() error { return profilesutil.Fetch(jirix, outFile, serviceUrl) }, "fetch %s", serviceUrl)
		}
		return seq.Done()
	}

	return profilesutil.AtomicAction(jirix, fn, outDir, "install mojo_shell")
}

// installMojoSystemThunks installs the mojo system thunks lib into outDir.
func (m *Manager) installMojoSystemThunks(jirix *jiri.X, outDir string) error {
	fn := func() error {
		outFile := filepath.Join(outDir, "libsystem_thunks.a")
		return jirix.NewSeq().MkdirAll(outDir, profilesutil.DefaultDirPerm).
			Call(func() error {
			return profilesutil.Fetch(jirix, outFile, mojoSystemThunksUrl(m.platform, m.buildVersion))
		}, "fetch mojo system thunks").Done()
	}
	return profilesutil.AtomicAction(jirix, fn, outDir, "Download Mojo system thunks")
}

func (m *Manager) Uninstall(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	// TODO(nlacasse): What should we do with all the installed artifacts?
	// They could be used by other profile versions, so deleting them does not
	// make sense.  Should we check that they are only used by this profile
	// before deleting?
	pdb.RemoveProfileTarget(m.profileInstaller, m.profileName, target)
	return nil
}

// androidPlatformToolsUrl returns the url of the android platform tools zip
// file for the given version.
func androidPlatformToolsUrl(version string) string {
	return fmt.Sprintf("http://tools.android.com/download/sdk-repo-linux-platform-tools-%s.zip", version)
}

// mojoNetworkServiceUrl returns the url for the network service for the given
// platform and git revision.
func mojoNetworkServiceUrl(platform, gitRevision string) string {
	return mojoStorageBucket + fmt.Sprintf("/network_service/%s/%s/network_service.mojo.zip", gitRevision, platform)
}

// mojoServiceUrl returns the url for the service for the given platform, name,
// and git revision.
func mojoServiceUrl(platform, name, gitRevision string) string {
	return mojoStorageBucket + fmt.Sprintf("/services/%s/%s/%s", platform, gitRevision, name)
}

// mojoShellUrl returns the url for the mojo shell binary given platform and
// git revision.
func mojoShellUrl(platform, gitRevision string) string {
	return mojoStorageBucket + fmt.Sprintf("/shell/%s/%s.zip", gitRevision, platform)
}

// mojoSystemThunksUrl returns the url for the mojo system thunks binary for the
// given platform and git revision.
func mojoSystemThunksUrl(platform, gitRevision string) string {
	return mojoStorageBucket + fmt.Sprintf("/system_thunks/%s/%s/libsystem_thunks.a", platform, gitRevision)
}

func mojoCompilerUrl(platform string, sha1 string) string {
	return mojoStorageBucket + fmt.Sprintf("/mojom_parser/%s/%s", platform, sha1)
}
