// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"v.io/jiri/jiri"
	"v.io/jiri/runutil"
	"v.io/x/lib/cmdline"
)

var (
	displayFlag            string
	resolutionFlag         string
	screenshotIntervalFlag string
	screenshotNameFlag     string
	urlFlag                string
)

func init() {
	cmdCollect.Flags.StringVar(&displayFlag, "display", ":1365", "The value of DISPLAY environment variable for Xvfb.")
	cmdCollect.Flags.StringVar(&resolutionFlag, "resolution", "1920x1080x24", "The resolution string for Xvfb.")
	cmdCollect.Flags.StringVar(&screenshotIntervalFlag, "interval", "5s", "The interval between screenshots.")
	cmdCollect.Flags.StringVar(&screenshotNameFlag, "name", "", "The name of the screenshot file.")
	cmdCollect.Flags.StringVar(&urlFlag, "url", "", "The url to take screenshots for.")
}

var cmdCollect = &cmdline.Command{
	Name:  "collect",
	Short: "Takes screenshots of a given URL in Chrome and stores them in the given export dir",
	Long: `
The collect commands takes screenshots of a given URL in Chrome and stores them
in the given export dir.

To use this command, the following programs need to be installed:
Google Chrome,Xvfb, and Fluxbox.
`,
	Runner: cmdline.RunnerFunc(runCollect),
}

func runCollect(env *cmdline.Env, args []string) error {
	jirix, err := jiri.NewX(env)
	if err != nil {
		return err
	}
	if err := checkPreRequisites(); err != nil {
		return err
	}

	// A tmp dir to store screenshots.
	tmpDir, err := jirix.NewSeq().TempDir("", "vkiosk")
	if err != nil {
		return err
	}
	fmt.Fprintf(jirix.Stdout(), "Tmp screenshot dir: %s\n", tmpDir)

	fmt.Fprintf(jirix.Stdout(), "Starting Xvfb at DISPLAY=%s with resolution %s...\n", displayFlag, resolutionFlag)
	p, err := startXvfb(jirix)
	if err != nil {
		return err
	}

	// Set up cleanup function to remove tmp screenshot dir and kill Xvfb.
	// When Xvfb is killed, the Chrome running in it will also be killed
	// automatically.
	cleanupFn := func() {
		// Ignore all errors.
		jirix.NewSeq().RemoveAll(tmpDir)
		p.Kill()
	}
	defer cleanupFn()

	// Trap SIGTERM and SIGINT signal.
	go func() {
		sigchan := make(chan os.Signal, 1)
		signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM)
		<-sigchan
		cleanupFn()
		os.Exit(0)
	}()

	fmt.Fprintf(jirix.Stdout(), "Starting Chrome for %q in Xvfb...\n", urlFlag)
	if err := startChrome(jirix); err != nil {
		return err
	}

	fmt.Fprintf(jirix.Stdout(), "Starting Fluxbox in Xvfb...\n")
	if err := startFluxbox(jirix); err != nil {
		return err
	}

	if err := takeScreenshots(jirix, tmpDir); err != nil {
		return err
	}
	return nil
}

func checkPreRequisites() error {
	programs := []string{"Xvfb", "fluxbox", "google-chrome", "scrot"}
	notInstalled := []string{}
	for _, p := range programs {
		if _, err := exec.LookPath(p); err != nil {
			notInstalled = append(notInstalled, p)
		}
	}
	if len(notInstalled) != 0 {
		return fmt.Errorf("%q not found in PATH", strings.Join(notInstalled, ","))
	}
	return nil
}

// startXvfb starts Xvfb at the given display and screen resolution.
// It provides an virtual X11 environment for Chrome to run in.
func startXvfb(jirix *jiri.X) (*runutil.Handle, error) {
	args := []string{
		displayFlag,
		"-screen",
		"0",
		resolutionFlag,
		"-ac",
	}
	cmd, err := jirix.NewSeq().Start("Xvfb", args...)
	if err != nil {
		return nil, err
	}
	// Wait a little bit to make sure it finishes initialization.
	time.Sleep(time.Second * 5)
	return cmd, nil
}

// startFluxbox starts a light weight windows manager Fluxbox.
// Without it, Chrome can't be maximized or run in kiosk mode properly.
func startFluxbox(jirix *jiri.X) error {
	args := []string{
		"-display",
		displayFlag,
	}
	if _, err := jirix.NewSeq().Start("fluxbox", args...); err != nil {
		return err
	}
	return nil
}

// startChrome starts Chrome in a kiosk mode in the given X11 environment.
func startChrome(jirix *jiri.X) error {
	args := []string{
		"--kiosk",
		urlFlag,
	}
	env := map[string]string{"DISPLAY": displayFlag}
	if _, err := jirix.NewSeq().Env(env).Start("google-chrome", args...); err != nil {
		return err
	}
	return nil
}

// takeScreenshot takes screenshots periodically, and stores them in the given
// export dir.
func takeScreenshots(jirix *jiri.X, tmpDir string) error {
	d, err := time.ParseDuration(screenshotIntervalFlag)
	if err != nil {
		return fmt.Errorf("ParseDuration(%s) failed: %v", screenshotIntervalFlag, err)
	}
	screenshotFile := filepath.Join(tmpDir, screenshotNameFlag)
	scrotArgs := []string{
		screenshotFile,
	}
	s := jirix.NewSeq()
	env := map[string]string{"DISPLAY": displayFlag}
	gsutilArgs := []string{
		"-q",
		"cp",
		screenshotFile,
		exportDirFlag + "/" + screenshotNameFlag,
	}
	ticker := time.NewTicker(d)
	for range ticker.C {
		// Use "scrot" command to take screenshots.
		fmt.Fprintf(jirix.Stdout(), "[%s]: take screenshot to %q...\n", nowTimestamp(), screenshotFile)
		if err := s.Env(env).Last("scrot", scrotArgs...); err != nil {
			fmt.Fprintf(jirix.Stderr(), "%v\n", err)
			continue
		}

		// Store the screenshots to export dir.
		fmt.Fprintf(jirix.Stdout(), "[%s]: copying screenshot to %s...\n", nowTimestamp(), exportDirFlag)
		if strings.HasPrefix(exportDirFlag, "gs://") {
			if err := s.Last("gsutil", gsutilArgs...); err != nil {
				fmt.Fprintf(jirix.Stderr(), "%v\n", err)
			}
		} else {
			if err := s.Rename(screenshotFile, filepath.Join(exportDirFlag, screenshotNameFlag)).Done(); err != nil {
				fmt.Fprintf(jirix.Stderr(), "%v\n", err)
			}
		}
	}
	return nil
}

func nowTimestamp() string {
	return time.Now().Format("20060102 15:04:05")
}
