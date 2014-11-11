// Package util contains a variety of general purpose functions, such
// as the SelfUpdate() function, for writing tools.
package util

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"tools/lib/envutil"
)

const (
	rootEnv = "VEYRON_ROOT"
)

// CommonConfig holds configuration common to veyron tools.
type CommonConfig struct {
	// GoRepos identifies top-level VEYRON_ROOT directories that
	// contain a Go workspace.
	GoRepos []string `json:"go-repos"`
	// VDLRepos identifies top-level VEYRON_ROOT directories that
	// contain a VDL workspace.
	VDLRepos []string `json:"vdl-repos"`
	// SnapshotLabelTests maps snapshot labels to sets of tests
	// that determine whether a snapshot for the given label can
	// be created.
	SnapshotLabelTests map[string][]string `json:"snapshot-label-tests"`
	// ProjectTests maps veyron projects to sets of tests that
	// should be executed to test changes in the given project.
	ProjectTests map[string][]string `json:"project-tests"`
	// TestDependencies maps tests to sets of tests that the given
	// test depends on.
	TestDependencies map[string][]string `json:"test-dependencies"`
}

// LocalManifestFile returns the path to the local manifest.
func LocalManifestFile() (string, error) {
	root, err := VeyronRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".local_manifest"), nil
}

// LocalSnapshotDir returns the path to the local snapshots directory.
func LocalSnapshotDir() (string, error) {
	root, err := VeyronRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".snapshots"), nil
}

// RemoteManifestDir returns the path to the local manifest directory.
func RemoteManifestDir() (string, error) {
	root, err := VeyronRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".manifest", "v1"), nil
}

// RemoteManifestFile returns the path to the manifest file with the
// given relative path.
func RemoteManifestFile(name string) (string, error) {
	dir, err := RemoteManifestDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// ResolveManifestPath resolves the given path to an absolute path in
// the local filesystem. If the input is already an absolute path,
// this operation is a no-op. Otherwise, the relative path is rooted
// in the local manifest directory.
func ResolveManifestPath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}
	return RemoteManifestFile(path)
}

// ConfigDir returns the local path to the directory storing config
// files for the veyron tools.
func ConfigDir() (string, error) {
	root, err := VeyronRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "tools", "conf"), nil
}

// ConfigFile returns the local path to the config file identifed by
// the given name.
func ConfigFile(name string) (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".json"), nil
}

// TestScriptDir returns the local path to the test script directory.
func TestScriptDir() (string, error) {
	root, err := VeyronRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "scripts", "jenkins"), nil
}

// TestScriptFile returns the local path to the test script file
// identifed by the given name.
func TestScriptFile(name string) (string, error) {
	dir, err := TestScriptDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".sh"), nil
}

// LoadConfig loads the config identified by the given name.
func LoadConfig(name string, config interface{}) error {
	configPath, err := ConfigFile(name)
	if err != nil {
		return err
	}
	configBytes, err := ioutil.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("ReadFile(%v) failed: %v", configPath, err)
	}
	if err := json.Unmarshal(configBytes, config); err != nil {
		return fmt.Errorf("Unmarshal(%v) failed: %v", string(configBytes), err)
	}
	return nil
}

// VeyronEnvironment returns the environment variables setting for
// veyron. The util package captures the original state of the
// relevant environment variables when the tool is initialized, and
// every invocation of this function updates this original state
// according to the current config of the veyron tool.
func VeyronEnvironment(platform Platform) (*envutil.Snapshot, error) {
	env := envutil.NewSnapshotFromOS()
	root, err := VeyronRoot()
	if err != nil {
		return nil, err
	}
	var config CommonConfig
	if err := LoadConfig("common", &config); err != nil {
		return nil, err
	}
	setGoPath(env, root, &config)
	setVdlPath(env, root, &config)
	archCmd := exec.Command("uname", "-m")
	arch, err := archCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get host architecture: %v\n%v\n%s", err, strings.Join(archCmd.Args, " "))
	}
	if platform.OS == "linux" {
		if err := setBluetoothCgoEnv(env, root, strings.TrimSpace(string(arch))); err != nil {
			return nil, err
		}
	}
	switch {
	case platform.Arch == runtime.GOARCH && platform.OS == runtime.GOOS:
		// If setting up the environment for the host, we are done.
	case platform.Arch == "arm" && platform.OS == "linux":
		// Set up cross-compilation for arm / linux.
		setArmLinuxEnv := setArmEnv
		if platform.Environment == "android" {
			setArmLinuxEnv = setAndroidEnv
		}
		if err := setArmLinuxEnv(env, platform); err != nil {
			return nil, err
		}
	case platform.Arch == "386" && platform.OS == "nacl":
		// Set up cross-compilation for 386 / nacl.
		if err := setNaclEnv(env, platform); err != nil {
			return nil, err
		}
	default:
		return nil, UnsupportedPlatformErr{platform}
	}
	// If VEYRON_ENV_SETUP==none, revert all deltas to their
	// original base value. We can't just skip the above logic or
	// revert to the BaseMap completely, since we still need
	// DeltaMap to tell us which variables we care about.
	//
	// TODO(toddw): Remove this logic when Cos' old setup stops
	// depending on it.
	if env.Get("VEYRON_ENV_SETUP") == "none" {
		for key := range env.DeltaMap() {
			env.Set(key, env.BaseMap()[key])
		}
	}
	return env, nil
}

// VeyronRoot returns the root of the veyron universe.
func VeyronRoot() (string, error) {
	root := os.Getenv(rootEnv)
	if root == "" {
		return "", fmt.Errorf("%v is not set", rootEnv)
	}
	result, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("EvalSymlinks(%v) failed: %v", root)
	}
	return result, nil
}

// setAndroidEnv sets the environment variables used for android
// cross-compilation.
func setAndroidEnv(env *envutil.Snapshot, platform Platform) error {
	root, err := VeyronRoot()
	if err != nil {
		return err
	}
	// Set CC specific environment variables.
	env.Set("CC", filepath.Join(root, "environment", "android", "ndk-toolchain", "bin", "arm-linux-androideabi-gcc"))
	// Set Go specific environment variables.
	env.Set("GOARCH", platform.Arch)
	env.Set("GOARM", strings.TrimPrefix(platform.SubArch, "v"))
	env.Set("GOOS", platform.OS)
	if err := setJniCgoEnv(env, root, "android"); err != nil {
		return err
	}
	// Add the paths to veyron cross-compilation tools to the PATH.
	path := env.GetTokens("PATH", ":")
	path = append([]string{
		filepath.Join(root, "environment", "android", "go", "bin"),
	}, path...)
	env.SetTokens("PATH", path, ":")
	return nil
}

// setArmEnv sets the environment variables used for android
// cross-compilation.
func setArmEnv(env *envutil.Snapshot, platform Platform) error {
	root, err := VeyronRoot()
	if err != nil {
		return err
	}
	// Set Go specific environment variables.
	env.Set("GOARCH", platform.Arch)
	env.Set("GOARM", strings.TrimPrefix(platform.SubArch, "v"))
	env.Set("GOOS", platform.OS)

	// Add the paths to veyron cross-compilation tools to the PATH.
	path := env.GetTokens("PATH", ":")
	path = append([]string{
		filepath.Join(root, "environment", "cout", "xgcc", "cross_arm"),
		filepath.Join(root, "environment", "go", "linux", "arm", "go", "bin"),
	}, path...)
	env.SetTokens("PATH", path, ":")
	return nil
}

// setGoPath adds the paths to veyron Go workspaces to the GOPATH
// variable.
func setGoPath(env *envutil.Snapshot, root string, config *CommonConfig) {
	gopath := env.GetTokens("GOPATH", ":")
	// Append an entry to gopath for each veyron go repo.
	for _, repo := range config.GoRepos {
		gopath = append(gopath, filepath.Join(root, repo, "go"))
	}
	env.SetTokens("GOPATH", gopath, ":")
}

// setVdlPath adds the paths to veyron Go workspaces to the VDLPATH
// variable.
func setVdlPath(env *envutil.Snapshot, root string, config *CommonConfig) {
	vdlpath := env.GetTokens("VDLPATH", ":")
	// Append an entry to vdlpath for each veyron go repo.
	//
	// TODO(toddw): This logic will change when we pull vdl into a
	// separate repo.
	for _, repo := range config.VDLRepos {
		vdlpath = append(vdlpath, filepath.Join(root, repo, "go"))
	}
	env.SetTokens("VDLPATH", vdlpath, ":")
}

// setBluetoothCgoEnv sets the CGO_ENABLED variable and adds the
// bluetooth third-party C libraries veyron Go code depends on to the
// CGO_CFLAGS and CGO_LDFLAGS variables.
func setBluetoothCgoEnv(env *envutil.Snapshot, root, arch string) error {
	// Set the CGO_* variables for the veyron proximity component.
	env.Set("CGO_ENABLED", "1")
	libs := []string{
		"dbus-1.6.14",
		"expat-2.1.0",
		"bluez-4.101",
		"libusb-1.0.16-rc10",
		"libusb-compat-0.1.5",
	}
	cflags := env.GetTokens("CGO_CFLAGS", " ")
	ldflags := env.GetTokens("CGO_LDFLAGS", " ")
	for _, lib := range libs {
		dir := filepath.Join(root, "environment", "cout", lib, arch)
		if _, err := os.Stat(dir); err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("Stat(%v) failed: %v", dir, err)
			}
		} else {
			if lib == "dbus-1.6.14" {
				cflags = append(cflags, filepath.Join("-I"+dir, "include", "dbus-1.0", "dbus"))
			} else {
				cflags = append(cflags, filepath.Join("-I"+dir, "include"))
			}
			ldflags = append(ldflags, filepath.Join("-L"+dir, "lib"), "-Wl,-rpath", filepath.Join(dir, "lib"))
		}
	}
	env.SetTokens("CGO_CFLAGS", cflags, " ")
	env.SetTokens("CGO_LDFLAGS", ldflags, " ")
	return nil
}

// setJniCgoEnv sets the CGO_ENABLED variable and adds the JNI
// third-party C libraries veyron Go code depends on to the CGO_CFLAGS
// and CGO_LDFLAGS variables.
func setJniCgoEnv(env *envutil.Snapshot, root, arch string) error {
	env.Set("CGO_ENABLED", "1")
	cflags := env.GetTokens("CGO_CFLAGS", " ")
	ldflags := env.GetTokens("CGO_LDFLAGS", " ")
	dir := filepath.Join(root, "environment", "cout", "jni-wrapper-1.0", arch)
	if _, err := os.Stat(dir); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("Stat(%v) failed: %v", dir, err)
		}
	} else {
		cflags = append(cflags, filepath.Join("-I"+dir, "include"))
		ldflags = append(ldflags, filepath.Join("-L"+dir, "lib"), "-Wl,-rpath", filepath.Join(dir, "lib"))
	}
	env.SetTokens("CGO_CFLAGS", cflags, " ")
	env.SetTokens("CGO_LDFLAGS", ldflags, " ")
	return nil
}

// setNaclEnv sets the environment variables used for nacl
// cross-compilation.
func setNaclEnv(env *envutil.Snapshot, platform Platform) error {
	env.Set("GOARCH", platform.Arch)
	env.Set("GOOS", platform.OS)
	return nil
}
