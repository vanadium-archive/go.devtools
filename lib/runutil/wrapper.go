package runutil

import (
	"fmt"
	"os"
)

// Chdir returns a closure for os.Chdir() that can be passed to the
// Function() method.
func Chdir(dir string) (func() error, string) {
	return func() error { return os.Chdir(dir) }, fmt.Sprintf("cd %v", dir)
}

// MkdirAll returns a closure for os.MkdirAll() that can be passed to
// the Function() method.
func MkdirAll(dir string, mode os.FileMode) (func() error, string) {
	return func() error { return os.MkdirAll(dir, mode) }, fmt.Sprintf("mkdir -p %v", dir)
}

// RemoveAll returns a closure for os.RemoveAll() that can be passed
// to the Function() method.
func RemoveAll(dir string) (func() error, string) {
	return func() error { return os.RemoveAll(dir) }, fmt.Sprintf("rm -rf %v", dir)
}

// Rename returns a closure for os.Rename() that can be passed to the
// Function() method.
func Rename(src, dst string) (func() error, string) {
	return func() error { return os.Rename(src, dst) }, fmt.Sprintf("mv %v %v", src, dst)
}
