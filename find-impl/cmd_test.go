package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestFindImpl(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := os.Setenv("GOPATH", os.Getenv("GOPATH")+":"+filepath.Join(cwd, "testdata")); err != nil {
		t.Fatalf("%v", err)
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("go", "run", "main.go", "cmd.go", "--interface=test.Interface", "test")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%v\n%v", stderr.String(), err)
	}
	if got, want := stdout.String(), "test.Implementation in "+filepath.Join(cwd, "testdata", "src", "test", "test.go")+"\n"; got != want {
		t.Fatalf("unexpected output:got\n%v\nwant\n%v", got, want)
	}
}
