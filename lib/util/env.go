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
)

const (
	rootEnv = "VEYRON_ROOT"
)

var (
	baseEnv map[string]string
)

type Config struct {
	GoRepos []string
}

func init() {
	// Initialize the baseEnv map with values of the environment
	// variables relevant to veyron.
	baseEnv = map[string]string{}
	vars := []string{
		"PATH",
		"CGO_ENABLED",
		"CGO_CFLAGS",
		"CGO_LDFLAGS",
		"GOPATH",
		"VDLPATH",
	}
	for _, v := range vars {
		baseEnv[v] = os.Getenv(v)
	}
}

// Config returns the config for veyron tools.
func VeyronConfig() (*Config, error) {
	root, err := VeyronRoot()
	if err != nil {
		return nil, err
	}
	confPath := filepath.Join(root, "tools", "conf", "veyron")
	confBytes, err := ioutil.ReadFile(confPath)
	if err != nil {
		return nil, fmt.Errorf("ReadFile(%v) failed: %v", confPath, err)
	}
	var conf Config
	if err := json.Unmarshal(confBytes, &conf); err != nil {
		return nil, fmt.Errorf("Unmarshal(%v) failed: %v", string(confBytes), err)
	}
	return &conf, nil
}

// SetupVeyronEnvironment sets up the environment variables used by
// veyron for the given platform.
func SetupVeyronEnvironment(platform Platform) error {
	if os.Getenv("VEYRON_ENV_SETUP") == "none" {
		return nil
	}
	env, err := VeyronEnvironment(platform)
	if err != nil {
		return err
	}
	for key, value := range env {
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("Setenv(%v, %v) failed: %v", key, value, err)
		}
	}
	return nil
}

// VeyronEnvironment returns the environment variables setting for
// veyron. The util package captures the original state of the
// relevant environment variables when the tool is initialized, and
// every invocation of this function updates this original state
// according to the current config of the veyron tool.
func VeyronEnvironment(platform Platform) (map[string]string, error) {
	root, err := VeyronRoot()
	if err != nil {
		return nil, err
	}
	conf, err := VeyronConfig()
	if err != nil {
		return nil, err
	}
	env := map[string]string{}
	setGoPath(env, root, conf)
	setVdlPath(env, root, conf)
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
	if platform.Arch == runtime.GOARCH && platform.OS == runtime.GOOS {
		// If setting up the environment for the host, we are done.
		return env, nil
	}
	// Otherwise, set up the cross-compilation environment.
	if platform.Arch == "arm" && platform.OS == "linux" {
		if platform.Environment == "android" {
			if err := setAndroidEnv(platform, env); err != nil {
				return nil, err
			}
			return env, nil
		} else {
			if err := setArmEnv(platform, env); err != nil {
				return nil, err
			}
			return env, nil
		}
	}
	return nil, UnsupportedErr{platform}
}

// VeyronRoot returns the root of the veyron universe.
func VeyronRoot() (string, error) {
	root := os.Getenv(rootEnv)
	if root == "" {
		return "", fmt.Errorf("%v is not set", rootEnv)
	}
	return root, nil
}

// getEnvTokens fetches a value for the environment variable identified by
// the given key from the given map, or if it does not exist there,
// from the baseEnv map. The value is then tokenized using the given
// separator and returned as a slice of string tokens.
func getEnvTokens(env map[string]string, key, separator string) []string {
	tokens, ok := env[key]
	if !ok {
		tokens = baseEnv[key]
	}
	result := []string{}
	for _, token := range strings.Split(tokens, separator) {
		if token != "" {
			result = append(result, token)
		}
	}
	return result
}

// setAndroidEnv sets the environment variables used for android
// cross-compilation.
func setAndroidEnv(platform Platform, env map[string]string) error {
	root, err := VeyronRoot()
	if err != nil {
		return err
	}
	// Set CC specific environment variables.
	env["CC"] = filepath.Join(root, "environment", "android", "ndk-toolchain", "bin", "arm-linux-androideabi-gcc")
	// Set Go specific environment variables.
	env["GOARCH"] = platform.Arch
	env["GOARM"] = strings.TrimPrefix(platform.SubArch, "v")
	env["GOOS"] = platform.OS
	env["GOROOT"] = filepath.Join(root, "environment", "android", "go")
	if err := setJniCgoEnv(env, root, "arm"); err != nil {
		return err
	}
	// Add the paths to veyron cross-compilation tools to the PATH.
	path := getEnvTokens(env, "PATH", ":")
	path = append([]string{
		filepath.Join(root, "environment", "android", "go", "bin"),
	}, path...)
	env["PATH"] = strings.Join(path, ":")
	return nil
}

// setArmEnv sets the environment variables used for android
// cross-compilation.
func setArmEnv(platform Platform, env map[string]string) error {
	root, err := VeyronRoot()
	if err != nil {
		return err
	}
	// Set Go specific environment variables.
	env["GOARCH"] = platform.Arch
	env["GOARM"] = strings.TrimPrefix(platform.SubArch, "v")
	env["GOOS"] = platform.OS
	env["GOROOT"] = filepath.Join(root, "environment", "go", "linux", "arm", "go")
	// Add the paths to veyron cross-compilation tools to the PATH.
	path := getEnvTokens(env, "PATH", ":")
	path = append([]string{
		filepath.Join(root, "environment", "cout", "xgcc", "cross_arm"),
		filepath.Join(root, "environment", "go", "linux", "arm", "go", "bin"),
	}, path...)
	env["PATH"] = strings.Join(path, ":")
	return nil
}

// setGoPath adds the paths to veyron Go workspaces to the GOPATH
// variable.
func setGoPath(env map[string]string, root string, conf *Config) {
	gopath := getEnvTokens(env, "GOPATH", ":")
	// Append an entry to gopath for each veyron go repo.
	for _, repo := range conf.GoRepos {
		gopath = append(gopath, filepath.Join(root, repo, "go"))
	}
	env["GOPATH"] = strings.Join(gopath, ":")
}

// setVdlPath adds the paths to veyron Go workspaces to the VDLPATH
// variable.
func setVdlPath(env map[string]string, root string, conf *Config) {
	vdlpath := getEnvTokens(env, "VDLPATH", ":")
	// Append an entry to vdlpath for each veyron go repo.
	//
	// TODO(toddw): This logic will change when we pull vdl into a
	// separate repo.
	for _, repo := range conf.GoRepos {
		vdlpath = append(vdlpath, filepath.Join(root, repo, "go"))
	}
	env["VDLPATH"] = strings.Join(vdlpath, ":")
}

// setBluetoothCgoEnv sets the CGO_ENABLED variable and adds the
// bluetooth third-party C libraries veyron Go code depends on to the
// CGO_CFLAGS and CGO_LDFLAGS variables.
func setBluetoothCgoEnv(env map[string]string, root, arch string) error {
	// Set the CGO_* variables for the veyron proximity component.
	env["CGO_ENABLED"] = "1"
	libs := []string{
		"dbus-1.6.14",
		"expat-2.1.0",
		"bluez-4.101",
		"libusb-1.0.16-rc10",
		"libusb-compat-0.1.5",
	}
	cflags := getEnvTokens(env, "CGO_CFLAGS", " ")
	ldflags := getEnvTokens(env, "CGO_LDFLAGS", " ")
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
	env["CGO_CFLAGS"] = strings.Join(cflags, " ")
	env["CGO_LDFLAGS"] = strings.Join(ldflags, " ")
	return nil
}

// setJniCgoEnv sets the CGO_ENABLED variable and adds the JNI
// third-party C libraries veyron Go code depends on to the CGO_CFLAGS
// and CGO_LDFLAGS variables.
func setJniCgoEnv(env map[string]string, root, arch string) error {
	env["CGO_ENABLED"] = "1"
	cflags := getEnvTokens(env, "CGO_CFLAGS", " ")
	ldflags := getEnvTokens(env, "CGO_LDFLAGS", " ")
	dir := filepath.Join(root, "environment", "cout", "jni-wrapper-1.0", arch)
	if _, err := os.Stat(dir); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("Stat(%v) failed: %v", dir, err)
		}
	} else {
		cflags = append(cflags, filepath.Join("-I"+dir, "include"))
		ldflags = append(ldflags, filepath.Join("-L"+dir, "lib"), "-Wl,-rpath", filepath.Join(dir, "lib"))
	}
	env["CGO_CFLAGS"] = strings.Join(cflags, " ")
	env["CGO_LDFLAGS"] = strings.Join(ldflags, " ")
	return nil
}
