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

	"v.io/jiri/tool"
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
	ctx := tool.NewContextFromEnv(env)
	if err := checkPreRequisites(); err != nil {
		return err
	}

	// A tmp dir to store screenshots.
	tmpDir, err := ctx.Run().TempDir("", "vkiosk")
	if err != nil {
		return err
	}
	fmt.Fprintf(ctx.Stdout(), "Tmp screenshot dir: %s\n", tmpDir)

	fmt.Fprintf(ctx.Stdout(), "Starting Xvfb at DISPLAY=%s with resolution %s...\n", displayFlag, resolutionFlag)
	p, err := startXvfb(ctx)
	if err != nil {
		return err
	}

	// Set up cleanup function to remove tmp screenshot dir and kill Xvfb.
	// When Xvfb is killed, the Chrome running in it will also be killed
	// automatically.
	cleanupFn := func() {
		// Ignore all errors.
		os.RemoveAll(tmpDir)
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

	fmt.Fprintf(ctx.Stdout(), "Starting Chrome for %q in Xvfb...\n", urlFlag)
	if err := startChrome(ctx); err != nil {
		return err
	}

	fmt.Fprintf(ctx.Stdout(), "Starting Fluxbox in Xvfb...\n")
	if err := startFluxbox(ctx); err != nil {
		return err
	}

	if err := takeScreenshots(ctx, tmpDir); err != nil {
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
func startXvfb(ctx *tool.Context) (*os.Process, error) {
	args := []string{
		displayFlag,
		"-screen",
		"0",
		resolutionFlag,
		"-ac",
	}
	cmd, err := ctx.Start().Command("Xvfb", args...)
	if err != nil {
		return nil, err
	}
	// Wait a little bit to make sure it finishes initialization.
	time.Sleep(time.Second * 5)
	return cmd.Process, nil
}

// startFluxbox starts a light weight windows manager Fluxbox.
// Without it, Chrome can't be maximized or run in kiosk mode properly.
func startFluxbox(ctx *tool.Context) error {
	args := []string{
		"-display",
		displayFlag,
	}
	if _, err := ctx.Start().Command("fluxbox", args...); err != nil {
		return err
	}
	return nil
}

// startChrome starts Chrome in a kiosk mode in the given X11 environment.
func startChrome(ctx *tool.Context) error {
	args := []string{
		"--kiosk",
		urlFlag,
	}
	opts := ctx.Start().Opts()
	opts.Env = map[string]string{"DISPLAY": displayFlag}
	if _, err := ctx.Start().CommandWithOpts(opts, "google-chrome", args...); err != nil {
		return err
	}
	return nil
}

// takeScreenshot takes screenshots periodically, and stores them in the given
// export dir.
func takeScreenshots(ctx *tool.Context, tmpDir string) error {
	d, err := time.ParseDuration(screenshotIntervalFlag)
	if err != nil {
		return fmt.Errorf("ParseDuration(%s) failed: %v", screenshotIntervalFlag, err)
	}
	screenshotFile := filepath.Join(tmpDir, screenshotNameFlag)
	scrotArgs := []string{
		screenshotFile,
	}
	scrotOpts := ctx.Run().Opts()
	scrotOpts.Env = map[string]string{"DISPLAY": displayFlag}
	gsutilArgs := []string{
		"-q",
		"cp",
		screenshotFile,
		exportDirFlag + "/" + screenshotNameFlag,
	}
	ticker := time.NewTicker(d)
	for range ticker.C {
		// Use "scrot" command to take screenshots.
		fmt.Fprintf(ctx.Stdout(), "[%s]: take screenshot to %q...\n", nowTimestamp(), screenshotFile)
		if err := ctx.Run().CommandWithOpts(scrotOpts, "scrot", scrotArgs...); err != nil {
			fmt.Fprintf(ctx.Stderr(), "%v\n", err)
			continue
		}

		// Store the screenshots to export dir.
		fmt.Fprintf(ctx.Stdout(), "[%s]: copying screenshot to %s...\n", nowTimestamp(), exportDirFlag)
		if strings.HasPrefix(exportDirFlag, "gs://") {
			if err := ctx.Run().Command("gsutil", gsutilArgs...); err != nil {
				fmt.Fprintf(ctx.Stderr(), "%v\n", err)
			}
		} else {
			if err := ctx.Run().Rename(screenshotFile, filepath.Join(exportDirFlag, screenshotNameFlag)); err != nil {
				fmt.Fprintf(ctx.Stderr(), "%v\n", err)
			}
		}
	}
	return nil
}

func nowTimestamp() string {
	return time.Now().Format("20060102 15:04:05")
}
