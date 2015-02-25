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

	"v.io/tools/lib/envutil"
)

const (
	rootEnv = "VANADIUM_ROOT"
)

// LocalManifestFile returns the path to the local manifest.
func LocalManifestFile() (string, error) {
	root, err := VanadiumRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".local_manifest"), nil
}

// LocalSnapshotDir returns the path to the local snapshots directory.
func LocalSnapshotDir() (string, error) {
	root, err := VanadiumRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".snapshots"), nil
}

// RemoteManifestDir returns the path to the local manifest directory.
func RemoteManifestDir() (string, error) {
	root, err := VanadiumRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".manifest", "v2"), nil
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
// files for the vanadium tools.
func ConfigDir() (string, error) {
	root, err := VanadiumRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "release", "go", "src", "v.io", "tools", "conf"), nil
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

// VanadiumEnvironment returns the environment variables setting for
// vanadium. The util package captures the original state of the
// relevant environment variables when the tool is initialized, and
// every invocation of this function updates this original state
// according to the current config of the v23 tool.
func VanadiumEnvironment(platform Platform) (*envutil.Snapshot, error) {
	env := envutil.NewSnapshotFromOS()
	root, err := VanadiumRoot()
	if err != nil {
		return nil, err
	}
	var config Config
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
	if platform.OS == "darwin" || platform.OS == "linux" {
		if err := setSyncbaseCgoEnv(env, root, platform.OS); err != nil {
			return nil, err
		}
	}
	switch {
	case platform.Arch == runtime.GOARCH && platform.OS == runtime.GOOS:
		// If setting up the environment for the host, we are done.
	case platform.Arch == "arm" && platform.OS == "linux":
		// Set up cross-compilation for arm / linux.
		if err := setArmEnv(env, platform); err != nil {
			return nil, err
		}
	case platform.Arch == "arm" && platform.OS == "android":
		// Set up cross-compilation for arm / android.
		if err := setAndroidEnv(env, platform); err != nil {
			return nil, err
		}
	case (platform.Arch == "386" || platform.Arch == "amd64p32") && platform.OS == "nacl":
		// Set up cross-compilation nacl.
		if err := setNaclEnv(env, platform); err != nil {
			return nil, err
		}
	default:
		return nil, UnsupportedPlatformErr{platform}
	}
	return env, nil
}

// VanadiumGitRepoHost returns the URL that hosts vanadium git
// repositories.
func VanadiumGitRepoHost() string {
	return "https://vanadium.googlesource.com/"
}

// VanadiumRoot returns the root of the vanadium universe.
func VanadiumRoot() (string, error) {
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
	root, err := VanadiumRoot()
	if err != nil {
		return err
	}
	// Set Go specific environment variables.
	env.Set("CGO_ENABLED", "1")
	env.Set("GOOS", platform.OS)
	env.Set("GOARCH", platform.Arch)
	env.Set("GOARM", strings.TrimPrefix(platform.SubArch, "v"))
	if err := setJniCgoEnv(env, root, platform.OS); err != nil {
		return err
	}
	// Add the paths to vanadium cross-compilation tools to the PATH.
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
	root, err := VanadiumRoot()
	if err != nil {
		return err
	}
	// Set Go specific environment variables.
	env.Set("GOARCH", platform.Arch)
	env.Set("GOARM", strings.TrimPrefix(platform.SubArch, "v"))
	env.Set("GOOS", platform.OS)

	// Add the paths to vanadium cross-compilation tools to the PATH.
	path := env.GetTokens("PATH", ":")
	path = append([]string{
		filepath.Join(root, "environment", "cout", "xgcc", "cross_arm"),
		filepath.Join(root, "environment", "go", "linux", "arm", "go", "bin"),
	}, path...)
	env.SetTokens("PATH", path, ":")
	return nil
}

// setGoPath adds the paths to vanadium Go workspaces to the GOPATH
// variable.
func setGoPath(env *envutil.Snapshot, root string, config *Config) {
	gopath := env.GetTokens("GOPATH", ":")
	// Append an entry to gopath for each vanadium go workspace.
	for _, repo := range config.GoWorkspaces() {
		gopath = append(gopath, filepath.Join(root, repo, "go"))
	}
	env.SetTokens("GOPATH", gopath, ":")
}

// setVdlPath adds the paths to vanadium VDL workspaces to the VDLPATH
// variable.
func setVdlPath(env *envutil.Snapshot, root string, config *Config) {
	vdlpath := env.GetTokens("VDLPATH", ":")
	// Append an entry to vdlpath for each vanadium vdl workspace.
	//
	// TODO(toddw): This logic will change when we pull vdl into a
	// separate repo.
	for _, repo := range config.VDLWorkspaces() {
		vdlpath = append(vdlpath, filepath.Join(root, repo, "go"))
	}
	env.SetTokens("VDLPATH", vdlpath, ":")
}

// setBluetoothCgoEnv sets the CGO_ENABLED variable and adds the
// bluetooth third-party C libraries vanadium Go code depends on to the
// CGO_CFLAGS and CGO_LDFLAGS variables.
func setBluetoothCgoEnv(env *envutil.Snapshot, root, arch string) error {
	// Set the CGO_* variables for the vanadium proximity component.
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
// third-party C libraries vanadium Go code depends on to the CGO_CFLAGS
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

// setSyncbaseCgoEnv sets the CGO_ENABLED variable and adds the LevelDB
// third-party C++ libraries vanadium Go code depends on to the CGO_CFLAGS and
// CGO_LDFLAGS variables.
func setSyncbaseCgoEnv(env *envutil.Snapshot, root, arch string) error {
	// Set the CGO_* variables for the vanadium syncbase component.
	env.Set("CGO_ENABLED", "1")
	cflags := env.GetTokens("CGO_CFLAGS", " ")
	ldflags := env.GetTokens("CGO_LDFLAGS", " ")
	dir := filepath.Join(root, "third_party", "cout", "leveldb")
	if _, err := os.Stat(dir); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("Stat(%v) failed: %v", dir, err)
		}
	} else {
		cflags = append(cflags, filepath.Join("-I"+dir, "include"))
		ldflags = append(ldflags, filepath.Join("-L"+dir, "lib"))
		if arch == "linux" {
			ldflags = append(ldflags, "-Wl,-rpath", filepath.Join(dir, "lib"))
		}
	}
	env.SetTokens("CGO_CFLAGS", cflags, " ")
	env.SetTokens("CGO_LDFLAGS", ldflags, " ")
	return nil
}

// BuildCopRotationPath returns the path to the build cop rotation file.
func BuildCopRotationPath() (string, error) {
	root, err := VanadiumRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "release", "go", "src", "v.io", "tools", "conf", "buildcop.xml"), nil
}
