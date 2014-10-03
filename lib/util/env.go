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

type configType struct {
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

// ArmEnvironment returns the environment variables setting for arm
// cross-compilation. The util package captures the original state of
// the relevent environment variables when the tool is initialized,
// and every invocation of this function updates this original state
// according to the current configuration of the veyron tool.
func ArmEnvironment() (map[string]string, error) {
	root, err := VeyronRoot()
	if err != nil {
		return nil, err
	}
	env := map[string]string{}
	setEnvArmPath(env, root)
	env["GOARCH"] = "arm"
	env["GOARM"] = "6"
	return env, nil
}

// SetupVeyronEnvironment sets up the environment variables used by
// the veyron setup.
func SetupVeyronEnvironment() error {
	return setupEnvironment(VeyronEnvironment)
}

// SetupArmEnvironment sets up the environment variables used by the
// veyron setup for arm cross-compilation.
func SetupArmEnvironment() error {
	return setupEnvironment(ArmEnvironment)
}

// VeyronEnvironment returns the environment variables setting for
// veyron. The util package captures the original state of the
// relevent environment variables when the tool is initialized, and
// every invocation of this function updates this original state
// according to the current configuration of the veyron tool.
func VeyronEnvironment() (map[string]string, error) {
	root, err := VeyronRoot()
	if err != nil {
		return nil, err
	}
	configPath := filepath.Join(root, "tools", "conf", "veyron")
	configBytes, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("ReadFile(%v) failed: %v", configPath, err)
	}
	var config configType
	if err := json.Unmarshal(configBytes, &config); err != nil {
		return nil, fmt.Errorf("Unmarshal(%v) failed: %v", string(configBytes), err)
	}
	env := map[string]string{}
	setEnvGoPath(env, root, config)
	setEnvVdlPath(env, root, config)
	if err := setEnvCgo(env, root); err != nil {
		return nil, err
	}
	return env, nil
}

// VeyronRoot returns the root of the veyron universe.
func VeyronRoot() (string, error) {
	root := os.Getenv(rootEnv)
	if root == "" {
		return "", fmt.Errorf("%v is not set", rootEnv)
	}
	return root, nil
}

// parseTokens separates the given string into tokens using the given
// separator and returns a slice of all non-empty tokens.
func parseTokens(tokens, separator string) []string {
	result := []string{}
	for _, token := range strings.Split(tokens, separator) {
		if token != "" {
			result = append(result, token)
		}
	}
	return result
}

// setEnvArmPath adds the paths to veyron cross-compilation tools
// to the PATH variable. The util package captures the original state
// of the PATH environment variable when the tool is initialized, and
// every invocation of this function updates this original state
// according to the current configuration of the veyron tool.
func setEnvArmPath(env map[string]string, root string) {
	path := parseTokens(baseEnv["PATH"], ":")
	// Make sure that the cross-compilation version of gcc tools and go
	// is used.
	path = append([]string{
		filepath.Join(root, "environment", "cout", "xgcc", "cross_arm"),
		filepath.Join(root, "environment", "go", "linux", "arm", "go", "bin"),
	}, path...)
	env["PATH"] = strings.Join(path, ":")
}

// setEnvGoPath adds the paths to veyron Go workspaces to the GOPATH
// variable. The util package captures the original state of the
// GOPATH environment variable when the tool is initialized, and every
// invocation of this function updates this original state according
// to the current configuration of the veyron tool.
func setEnvGoPath(env map[string]string, root string, config configType) {
	gopath := parseTokens(baseEnv["GOPATH"], ":")
	// Append an entry to gopath for each veyron go repo.
	for _, repo := range config.GoRepos {
		gopath = append(gopath, filepath.Join(root, repo, "go"))
	}
	env["GOPATH"] = strings.Join(gopath, ":")
}

// setEnvVdlPath adds the paths to veyron Go workspaces to the VDLPATH
// variable. The util package captures the original state of the
// VDLPATH environment variable when the tool is initialized, and every
// invocation of this function updates this original state according
// to the current configuration of the veyron tool.
func setEnvVdlPath(env map[string]string, root string, config configType) {
	vdlpath := parseTokens(baseEnv["VDLPATH"], ":")
	// Append an entry to vdlpath for each veyron go repo.
	//
	// TODO(toddw): This logic will change when we pull vdl into a
	// separate repo.
	for _, repo := range config.GoRepos {
		vdlpath = append(vdlpath, filepath.Join(root, repo, "go"))
	}
	env["VDLPATH"] = strings.Join(vdlpath, ":")
}

// setEnvCgo sets the CGO_ENABLED variable and adds the third-party C
// libraries veyron Go code depends on to the CGO_CFLAGS and
// CGO_LDFLAGS variables. The util package captures the original state
// of the CGO_CFLAGS and CGO_LDFLAGS environment variables when the
// tool is initialized, and every invocation of this function updates
// this original state according to the current configuration of the
// veyron tool.
func setEnvCgo(env map[string]string, root string) error {
	// Set the CGO_* variables for the veyron proximity component.
	if runtime.GOOS == "linux" {
		env["CGO_ENABLED"] = "1"
		libs := []string{
			"dbus-1.6.14",
			"expat-2.1.0",
			"bluez-4.101",
			"libusb-1.0.16-rc10",
			"libusb-compat-0.1.5",
		}
		archCmd := exec.Command("uname", "-m")
		arch, err := archCmd.Output()
		if err != nil {
			return fmt.Errorf("failed to get host architecture: %v\n%v\n%s", err, strings.Join(archCmd.Args, " "))
		}
		cflags := parseTokens(baseEnv["CGO_CFLAGS"], " ")
		ldflags := parseTokens(baseEnv["CGO_LDFLAGS"], " ")
		for _, lib := range libs {
			dir := filepath.Join(root, "environment", "cout", lib, strings.TrimSpace(string(arch)))
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
	}
	return nil
}

// setupEnvironment updates the current environment with the values
// produced by the given function. Developers that wish to do the
// environment variable setup themselves, should set the
// VEYRON_ENV_SETUP environment variable to "none".
func setupEnvironment(fn func() (map[string]string, error)) error {
	if os.Getenv("VEYRON_ENV_SETUP") == "none" {
		return nil
	}
	environment, err := fn()
	if err != nil {
		return err
	}
	for key, value := range environment {
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("Setenv(%v, %v) failed: %v", key, value, err)
		}
	}
	return nil
}
