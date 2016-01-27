// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"time"
)

var (
	errFailedRun      = fmt.Errorf("test failed.")
	fail              = []byte("FAIL")
	pass              = []byte("FAIL")
	bendroidPidPrefix = []byte("BENDROIDPID=")
	logPrefix         = []byte("/GoLog   : ")
	newline           = []byte("\n")
)

func (t *testrun) run() (time.Duration, error) {
	// TODO(mattr): adb should be downloaded by the android profile, and we should use
	// that version.
	if err := exec.Command("adb", "install", "-r", t.apk).Run(); err != nil {
		return 0, err
	}
	if err := exec.Command("adb", "logcat", "-c").Run(); err != nil {
		return 0, err
	}
	// TODO(mattr): Should we ensure the screen is on?
	// TODO(mattr): Should we try to adjust the cpu governor? (seems to require root).
	// Note that gomobile sets the stderr/stdout of the android app to send logs to GoLog:I
	// and GoLog:E respectively.  This is just hardcoded into the gomobile tool and may
	// be expected to change at some point.
	logs := exec.Command("adb", "logcat", "-v", "tag", "*:S", "GoLog:*")
	logr, err := logs.StdoutPipe()
	if err != nil {
		return 0, err
	}
	if err := logs.Start(); err != nil {
		return 0, err
	}
	// The package name t.AndroidPackage was set in the AndroidManifest that we generated
	// along with the code.  The GoNativeAcctivity is the activity that the gomobile tool
	// creates.
	cmd := exec.Command("adb", "shell", "am", "start", t.AndroidPackage+"/org.golang.app.GoNativeActivity")
	if err := cmd.Run(); err != nil {
		logs.Process.Kill()
		return 0, err
	}
	stdout := t.Env.Stdout
	stderr := t.Env.Stderr
	if !*verbose && len(*bench) == 0 {
		// If we're not in verbose mode, and not doing benchmarks,
		// we don't actually print stdout/stderr, we just capture it for
		// analysis.
		stdout = &bytes.Buffer{}
		stderr = &bytes.Buffer{}
	}

	start := time.Now()
	scanner := bufio.NewScanner(logr)
	passOrFail := false
	for scanner.Scan() {
		byts := scanner.Bytes()
		if len(byts) == 0 || !bytes.HasPrefix(byts[1:], logPrefix) {
			// Skip logs that don't have to do with our test.
			continue
		}
		level, msg := byts[0], byts[len(logPrefix)+1:]
		switch level {
		case 'I':
			switch {
			case bytes.Equal(msg, fail):
				passOrFail, err = true, errFailedRun
			case bytes.Equal(msg, pass):
				passOrFail = true
			}
			stdout.Write(msg)
			stdout.Write(newline)
		case 'E':
			switch {
			case bytes.HasPrefix(msg, bendroidPidPrefix):
				var pid int64
				pid, err = strconv.ParseInt(string(msg[len(bendroidPidPrefix):]), 10, 64)
				if err != nil {
					fmt.Fprint(t.Env.Stderr, "Could not parse pid from %q", string(msg))
					continue
				}
				go func() {
					wait(t.AndroidPackage, pid)
					logs.Process.Kill()
				}()
			default:
				stderr.Write(msg)
				stderr.Write(newline)
			}
		}
	}
	if !passOrFail {
		err = scanner.Err()
	}
	if err != nil && stdout != t.Env.Stdout {
		// If there was a test failure, then dump stderr/stdout if
		// we previously captured it.
		// Note that 'go test' writes both to stdout.
		io.Copy(t.Env.Stdout, stderr.(*bytes.Buffer))
		io.Copy(t.Env.Stdout, stdout.(*bytes.Buffer))
	}
	return time.Now().Sub(start), err
}

func wait(androidPackage string, pid int64) {
	pkg := []byte(androidPackage)
	for {
		cmd := exec.Command("adb", "shell", "cat", fmt.Sprintf("/proc/%d/cmdline", pid))
		out, err := cmd.Output()
		// I don't know why, but we get a lot of zero bytes at the end of the /proc output.
		if err != nil || !bytes.Equal(bytes.Trim(out, "\x00"), pkg) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}
