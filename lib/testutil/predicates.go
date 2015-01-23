package testutil

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func isGCE() bool {
	sysuser := os.Getenv("USER")
	return sysuser == "veyron" && runtime.GOOS == "linux"
}

func isDarwin() bool {
	return runtime.GOOS == "darwin"
}

func isYosemite() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	out, err := exec.Command("uname", "-a").Output()
	if err != nil {
		return true
	}
	return strings.Contains(string(out), "Version 14.")
}

func isLinux() bool {
	return runtime.GOOS == "linux"
}
