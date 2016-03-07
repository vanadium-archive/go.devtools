// Copyright 2016 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go . -help

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"v.io/x/lib/cmdline"
)

var (
	flagVerbose bool
	flagWork    bool
	cmdBendroid = &cmdline.Command{
		Name:  "bendroid",
		Short: "Execute go tests and benchmarks on android devices.",
		Long: `
bendroid executes Go tests and benchmarks on an android device.

It requires that 'adb' (the Android Debug Bridge:
http://developer.android.com/tools/help/adb.html) be available in PATH.

Sample usage:
  GOARCH=arm GOOS=android go test -exec bendroid crypto/sha256
Or, alternatively:
  GOARCH=arm GOOS=android go test -c cryto/sha256
  bendroid ./sha256.test

Additionally, bendroid outputs a preamble of the form:
  BENDROID_<variable_name>=<value>
that describe the characteristics of the connected android device.

WARNING: As of March 2016, bendroid is unable to ensure synchronization between
what the executed program prints on standard output and standard error. This
should hopefully be resolved with the next release of adb.
`,
		ArgsName: "<filename to execute> [<flags provided to 'go test'>]",
		ArgsLong: "See 'go help run' for details",
		Runner:   cmdline.RunnerFunc(bendroid),
	}
)

func bendroid(env *cmdline.Env, inargs []string) error {
	if len(inargs) == 0 {
		return env.UsageErrorf("Require at least one argument, the path of the binary to copy to and execute on the device")
	}
	if err := describeDevice(); err != nil {
		return err
	}
	var (
		dir       = fmt.Sprintf("/data/local/tmp/bendroid/run-%d-%d", os.Getpid(), time.Now().UnixNano())
		hostbin   = inargs[0]
		devicebin = path.Join(dir, filepath.Base(hostbin))
		args      = append([]string{devicebin}, inargs[1:]...)
	)
	if err := adb("shell", "mkdir", "-p", dir); err != nil {
		return fmt.Errorf("failed to create scratch space in %v on the android device: %v", dir, err)
	}
	defer cleanup(dir)
	if err := adb("push", hostbin, devicebin); err != nil {
		return fmt.Errorf("failed to push binary to the android device: %v", err)
	}
	if err := adbshell(dir, args...); err != nil {
		return err
	}
	return nil
}

func adb(args ...string) error {
	buf := new(bytes.Buffer)
	cmd := exec.Command("adb", args...)
	if flagVerbose {
		fmt.Println(cmd.Args)
	}
	cmd.Stdout = buf
	cmd.Stderr = buf
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, buf.String())
		return fmt.Errorf("%v failed: %v", cmd.Args, err)
	}
	return nil
}

// adbshell is variant of the adb function that prints out stdout and stderr
// instead of hiding them while executing 'adb shell <args...>'.
//
// As of March 2016, there were a few issues with 'adb shell':
// (1) It does not differentiate between stdout and stderr of the program
//     executed on the device. Instead, stdout and stderr of the program
//     executed on the device are combined into stdout of the adb command.
// (2) It does not preserve the exit status of the command run on the device.
//     adb shell false && echo SUCCESS
//     writes SUCCESS to the terminal.
//
// Both of these issues seem to be addressed in the next release of the Android
// platform core (Android N?), which includes these changes:
// https://android.googlesource.com/platform/system/core/+/0955c66b226db7a7f34613f834f7b0a145fd407d
// and
// https://android.googlesource.com/platform/system/core/+/606835ae5c4b9519009cdff8b1c33169cff32cb1
// However, neither are mentioned in https://code.google.com/p/android/issues/detail?id=3254
//
// Till those changes are more broadly available (or at least a release with
// them has happened), workarounds are needed.
//
// https://github.com/facebook/fb-adb claims to address these issues and could
// be used instead.  However, in my experimentation, fb-adb (version 1.4.4)
// sometimes ends up writing what should have gone to stdout to stderr, when
// running a binary built via "go test -c".
//
// Thus, we use 'adb shell' with the following workarounds:
// (1) Write stdout to a local file on the device and read from there.
//     This means that stdout and stderr may be out of sync and we live with that.
// (2) Extract the exit status by executing:
//     adb shell "<cmd>; echo $? >EXITCODE"
//     and reading the EXITCODE from the device.
func adbshell(dir string, args ...string) error {
	// Redirect stdout to a file and write out EXIT_STATUS.
	var (
		stdout   = path.Join(dir, "STDOUT")
		exitcode = path.Join(dir, "EXITCODE")
		hack     = fmt.Sprintf("%s >%v; echo -n $? >%v", strings.Join(args, " "), stdout, exitcode)
	)
	fmt.Fprintln(os.Stderr, "WARNING: Standard output and standard error may not be in sync, see https://v.io/i/1225 for details")
	cmd := exec.Command("adb", "shell", hack)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if flagVerbose {
		fmt.Printf("[adb shell '%v']\n", hack)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("adb shell '%v' failed: %v", hack, err)
	}
	stop := make(chan struct{})
	done := make(chan error)
	go func() {
		done <- stdoutHack(stdout, stop)
	}()
	if err := cmd.Wait(); err != nil {
		close(stop)
		return fmt.Errorf("adb shell '%v' failed: %v", hack, err)
	}
	close(stop)
	if err := <-done; err != nil { // Wait for STDOUT to be flushed
		return fmt.Errorf("failed to read %v from device: %v", stdout, err)
	}
	return exitCodeHack(exitcode)
}

// stdoutHack repeatedly invokes "adb shell tail file" and dumps the output
// to os.Stdout.
func stdoutHack(file string, stop <-chan struct{}) error {
	var (
		nlines = 1
		done   = false
	)
	for !done {
		select {
		case <-stop:
			done = true
			// One more "adb shell tail" to catch any residue
			// before returning.
		default:
		}
		cmd := exec.Command("adb", "shell", "tail", "-n", fmt.Sprint("+", nlines), file)
		out, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		if err := cmd.Start(); err != nil {
			return err
		}
		// Use bufio.Reader instead of bufio.Scanner so that we
		// can determine if the last line read was a full line
		// or an incomplete line because we reached EOF.
		// (bufio.Scanner.Err hides io.EOF)
		reader := bufio.NewReader(out)
		oldlines := nlines
		for {
			line, err := reader.ReadSlice('\n')
			if err != nil {
				break
			}
			os.Stdout.Write(line)
			nlines++
		}
		if err := cmd.Wait(); err != nil {
			return err
		}
		if nlines == oldlines && !done {
			// No new lines read in this round, take a break
			time.Sleep(100 * time.Millisecond)
		}
	}
	return nil
}

func exitCodeHack(devicefile string) error {
	hostfile, err := mktemp("bendroid-exit-code")
	if err != nil {
		return err
	}
	defer os.Remove(hostfile)
	if err := adb("pull", devicefile, hostfile); err != nil {
		return fmt.Errorf("failed to determine exit status of process on device: %v", err)
	}
	f, err := os.Open(hostfile)
	if err != nil {
		return fmt.Errorf("failed to extract exit code, unable to open local file %v: %v", hostfile, err)
	}
	defer f.Close()
	byts, err := ioutil.ReadAll(f)
	if err != nil {
		return fmt.Errorf("failed to extract exit code, unable to read local file %v: %v", hostfile, err)
	}
	if code := string(byts); code != "0" {
		return fmt.Errorf("exited with exit status %v", code)
	}
	return nil
}

// describeDevice prints CPU and OS information of the connected android device
// to standard output.
func describeDevice() error {
	hostfile, err := mktemp("bendroid-describe-device")
	if err != nil {
		return err
	}
	defer os.Remove(hostfile)
	const devicefile = "/system/build.prop"
	if err := adb("pull", devicefile, hostfile); err != nil {
		return fmt.Errorf("failed to read %v on device: %v", devicefile, err)
	}
	f, err := os.Open(hostfile)
	if err != nil {
		return fmt.Errorf("failed to read device description: %v", err)
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	var osver struct {
		Release, SDK, BuildID, Incremental string
	}
	var cpu struct {
		Architecture       string
		Brand, Model, Name string
	}
	buildprops := []struct {
		key   []byte
		field *string
	}{
		{[]byte("ro.build.version.release="), &osver.Release},
		{[]byte("ro.build.version.sdk="), &osver.SDK},
		{[]byte("ro.build.id="), &osver.BuildID},
		{[]byte("ro.build.version.incremental="), &osver.Incremental},
		{[]byte("ro.product.cpu.abilist="), &cpu.Architecture},
		{[]byte("ro.product.brand="), &cpu.Brand},
		{[]byte("ro.product.model="), &cpu.Model},
		{[]byte("ro.product.name="), &cpu.Name},
	}
	for s.Scan() {
		byts := s.Bytes()
		for i, p := range buildprops {
			if bytes.HasPrefix(byts, p.key) {
				*p.field = string(bytes.TrimPrefix(byts, p.key))
				buildprops[i] = buildprops[len(buildprops)-1]
				buildprops = buildprops[:len(buildprops)-1]
				break
			}
		}
	}
	if len(buildprops) > 0 {
		return fmt.Errorf("did not find %s in %v on device, required to complete description of the device", buildprops[0], devicefile)
	}
	fmt.Printf("BENDROIDCPU_ARCHITECTURE=%v\n", cpu.Architecture)
	fmt.Printf("BENDROIDCPU_DESCRIPTION=%v\n", strings.Join([]string{cpu.Brand, cpu.Model, cpu.Name}, " "))
	fmt.Printf("BENDROIDOS_VERSION=%v (Build %v Release %v SDK %v)\n", osver.Release, osver.BuildID, osver.Incremental, osver.SDK)
	return nil
}

func cleanup(dir string) {
	if flagWork {
		fmt.Println("Build and run artifacts left in", dir, "on the device")
		return
	}
	adb("shell", "rm", "-r", dir)
}

func mktemp(pattern string) (string, error) {
	f, err := ioutil.TempFile("", pattern)
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file in host: %v", err)
	}
	defer f.Close()
	return f.Name(), nil
}

func main() {
	cmdline.HideGlobalFlagsExcept()
	cmdBendroid.Flags.BoolVar(&flagVerbose, "bendroid.v", false, "verbose output: print adb commands executed by bendroid")
	cmdBendroid.Flags.BoolVar(&flagWork, "bendroid.work", false, "print the name of the directory on the device where all data is copied and do not erase it")
	cmdline.Main(cmdBendroid)
}
