// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"v.io/jiri"
	"v.io/jiri/collect"
	"v.io/jiri/retry"
	"v.io/jiri/runutil"
	"v.io/jiri/util"
	"v.io/x/devtools/internal/test"
)

const (
	bucket                    = "gs://vanadium-release"
	checkManifestRetries      = 5
	checkManifestRetryPeriod  = 10 * time.Second
	hostNameStaging           = "dev.staging.v.io"
	hostNameProduction        = "dev.v.io"
	adminRole                 = "identity/role/vprod/admin"
	publisherRole             = "identity/role/vprod/publisher"
	manifestEnvVar            = "SNAPSHOT_MANIFEST"
	mounttableWaitRetries     = 30
	mounttableWaitRetryPeriod = 10 * time.Second
	propertiesFile            = ".release_candidate_properties"
	rcTimeFormat              = "2006-01-02.15:04"
	snapshotName              = "rc"
	testsEnvVar               = "TESTS"
)

var (
	defaultReleaseTestTimeout = time.Minute * 5
	manifestRE                = regexp.MustCompile(`^devmgr/.*<manifest snapshotpath="manifest/(.*)">`)

	toolsPackages = []string{
		"v.io/x/ref/services/agent/gcreds/",
		"v.io/x/ref/services/agent/vbecome/",
		"v.io/x/ref/services/debug/debug/",
		"v.io/x/ref/services/device/device/",
		"v.io/x/devtools/vbinary/",
	}

	serviceBinaries = []string{
		"applicationd",
		"binaryd",
		"deviced",
		"groupsd",
		"identityd",
		"mounttabled",
		"xproxyd",
		"xproxyd:vlab-xproxyd",
		"roled",
	}

	nonMounttableApps = []string{
		"devmgr/apps/applicationd",
		"devmgr/apps/binaryd",
		"devmgr/apps/groupsd",
		"devmgr/apps/identityd",
		"devmgr/apps/roled",
		"devmgr/apps/xproxyd",
		"devmgr/apps/VLabXProxy",
	}
)

type step struct {
	msg string
	fn  func() error
}

type updater struct {
	jirix               *jiri.X
	hostname            string
	oauthBlesserService string
}

func newUpdater(jirix *jiri.X, hostname string) *updater {
	return &updater{
		jirix:    jirix,
		hostname: hostname,
	}
}

// buildBinaries builds binaries for the given package pattern.
func (u *updater) buildBinaries(pkgs ...string) error {
	s := u.jirix.NewSeq()
	args := []string{
		"jiri",
		"go",
		"install",
		"-tags=leveldb",
	}
	args = append(args, pkgs...)
	u.outputCmd(args)
	return s.Last(args[0], args[1:]...)
}

// extractRCTimestamp extracts release candidate timestamp from the manifest
// path stored in the <manifestEnvVar> environment variable.
func (u *updater) extractRCTimestamp() (string, error) {
	manifestPath := os.Getenv(manifestEnvVar)
	if manifestPath == "" {
		return "", fmt.Errorf("Environment variable %q not set", manifestEnvVar)
	}
	return filepath.Base(manifestPath), nil
}

// uploadVanadiumBinaries uploads binaries to the specific timestamp dir in the
// vanadium-release storage bucket.
func (u *updater) uploadVanadiumBinaries(rcTimestamp string) error {
	s := u.jirix.NewSeq()
	tmpDir, err := s.TempDir("", "")
	if err != nil {
		return err
	}
	defer u.jirix.NewSeq().RemoveAll(tmpDir)
	doneFile := filepath.Join(tmpDir, ".done")

	gsutilUploadArgs := []string{
		"-q", "-m", "cp", "-r",
		filepath.Join(u.jirix.Root, "release", "go", "bin"),
		fmt.Sprintf("%s/%s", bucket, rcTimestamp),
	}
	gsutilDoneArgs := []string{"-q", "cp", doneFile, fmt.Sprintf("%s/%s", bucket, rcTimestamp)}

	return s.
		Run("gsutil", gsutilUploadArgs...).
		WriteFile(doneFile, nil, os.FileMode(0600)).
		Last("gsutil", gsutilDoneArgs...)
}

// downloadReleaseBinaries uses the "vbinary" tool to download current release
// binaries.
func (u *updater) downloadReleaseBinaries(binDir string) error {
	s := u.jirix.NewSeq()
	args := []string{
		u.bin("vbinary"),
		"--release",
		"download",
		"--output-dir=" + binDir,
	}
	u.outputCmd(args)
	return s.Last(args[0], args[1:]...)
}

// checkReleaseCandidateStatus checks whether the "latest" file in
// gs://vanadium-release was updated today. If so, it means that the staging
// services have been updated successfully today, and it will return the
// content of the file.
func (u *updater) checkReleaseCandidateStatus() (string, error) {
	s := u.jirix.NewSeq()
	args := []string{
		"cat",
		fmt.Sprintf("%s/latest", bucket),
	}
	var out bytes.Buffer
	if err := s.Capture(&out, nil).Last("gsutil", args...); err != nil {
		return "", err
	}
	t, err := time.Parse(rcTimeFormat, out.String())
	if err != nil {
		return "", fmt.Errorf("Parse(%s, %s) failed: %v", rcTimeFormat, out.String(), err)
	}
	now := time.Now()
	if t.Year() != now.Year() || t.Month() != now.Month() || t.Day() != now.Day() {
		return "", fmt.Errorf("Release candidate (%v) not done for today", t)
	}
	fmt.Fprintf(u.jirix.Stdout(), "Snapshot timestamp: %s\n", out.String())
	return out.String(), nil
}

// publishBinaries publishes binaries from the given location.
// If the location is empty, it will use $JIRI_ROOT/release/go/bin.
func (u *updater) publishBinaries(binDir string) error {
	s := u.jirix.NewSeq()
	args := u.publisherCmd(
		u.bin("device"),
		u.globalMtFlag(),
		"publish",
		"--goos=linux",
		"--goarch=amd64",
	)
	if binDir != "" {
		args = append(args, fmt.Sprintf("--from=%s", binDir))
	}
	args = append(args, serviceBinaries...)
	u.outputCmd(args)
	return s.Timeout(defaultReleaseTestTimeout).Last(args[0], args[1:]...)
}

// updateInstallations updates installations of all apps.
func (u *updater) updateInstallations() error {
	return u.runDeviceUpdate("devmgr/apps/*/*")
}

// updateMounttable updates mounttable instance.
// It will return when mounttable is ready or times out.
func (u *updater) updateMounttable() error {
	if err := u.runDeviceUpdate("devmgr/apps/mounttabled/*/*"); err != nil {
		return err
	}
	return u.waitForMounttable(fmt.Sprintf("/ns.%s:8101", u.hostname), `.+`)
}

// updateNonMounttableInstances updates instances of all apps.
func (u *updater) updateNonMounttableInstances() error {
	for _, app := range nonMounttableApps {
		if err := u.runDeviceUpdate(fmt.Sprintf("%s/*/*", app)); err != nil {
			return err
		}
	}
	return nil
}

// updateDeviceManager updates device manager itself.
func (u *updater) updateDeviceManager() error {
	return u.runDeviceUpdate("devmgr/device")
}

// checkManifestTimestamps checks the timestamp part of the manifest for all apps
// againest the given expected timestamp.
func (u *updater) checkManifestTimestamps(statsPrefix, expectedTimestamp string, expectedNumMatches int) error {
	fmt.Fprintf(u.jirix.Stdout(), "Expected timestamp: %s\nExpected number of results: %d\n------\n", expectedTimestamp, expectedNumMatches)
	s := u.jirix.NewSeq()
	args := u.adminCmd(
		u.bin("debug"),
		u.localMtFlag(),
		"stats",
		"read",
		fmt.Sprintf("%s/stats/system/metadata/build.Manifest", statsPrefix),
	)
	u.outputCmd(args)
	checkFn := func() error {
		var out bytes.Buffer
		if err := s.Capture(&out, nil).Timeout(defaultReleaseTestTimeout).Last(args[0], args[1:]...); err != nil {
			return err
		}
		statsOutput := out.String()
		numMatches := 0
		for _, line := range strings.Split(statsOutput, "\n") {
			matches := manifestRE.FindStringSubmatch(line)
			if len(matches) != 2 {
				continue
			}
			snapshotPath := matches[1]
			timestamp := filepath.Base(snapshotPath)
			numMatches++
			fmt.Fprintf(u.jirix.Stdout(), "%d: %s\n", numMatches, line)
			if timestamp != expectedTimestamp {
				return fmt.Errorf("failed to verify manifest timestamp of #%d. Got %s, want %s", numMatches, timestamp, expectedTimestamp)
			}
		}
		if numMatches != expectedNumMatches {
			return fmt.Errorf("wrong number of matches: want %d, got %d", expectedNumMatches, numMatches)
		}
		return nil
	}
	return retry.Function(u.jirix.Context, checkFn, retry.AttemptsOpt(checkManifestRetries), retry.IntervalOpt(checkManifestRetryPeriod))
}

// updateLatestFile updates the "latest" file in Google Storage bucket to the
// given release candidate timestamp.
func (u *updater) updateLatestFile(rcTimestamp string) error {
	s := u.jirix.NewSeq()
	tmpDir, err := s.TempDir("", "")
	if err != nil {
		return err
	}
	defer u.jirix.NewSeq().RemoveAll(tmpDir)
	latestFile := filepath.Join(tmpDir, "latest")
	args := []string{"-q", "cp", latestFile, fmt.Sprintf("%s/latest", bucket)}
	return s.WriteFile(latestFile, []byte(rcTimestamp), os.FileMode(0600)).
		Last("gsutil", args...)
}

// checkServices runs "jiri test run vanadium-prod-services-test".
func (u *updater) checkServices() error {
	s := u.jirix.NewSeq()
	args := []string{
		"jiri-test",
		"run",
		u.globalMtFlag(),
		fmt.Sprintf("--blessings-root=%s", u.hostname),
		"vanadium-prod-services-test",
	}
	u.outputCmd(args)
	return s.Timeout(defaultReleaseTestTimeout).Last(args[0], args[1:]...)
}

func (u *updater) runDeviceUpdate(target string) error {
	s := u.jirix.NewSeq()
	args := u.adminCmd(
		u.bin("device"),
		u.localMtFlag(),
		"update",
		target,
	)
	u.outputCmd(args)
	return s.Timeout(defaultReleaseTestTimeout).Last(args[0], args[1:]...)
}

// waitForMounttable waits for the given mounttable to be up and checks output
// against outputRegexp (timeout: 5 minutes).
func (u *updater) waitForMounttable(mounttableRoot, outputRegexp string) error {
	fmt.Fprintf(u.jirix.Stdout(), "Waiting for mounttable to be up...\n")
	s := u.jirix.NewSeq()
	args := u.adminCmd(
		u.bin("debug"),
		"glob",
		mounttableRoot+"/*",
	)
	up := false
	outputRE := regexp.MustCompile(outputRegexp)
	for i := 0; i < mounttableWaitRetries; i++ {
		var out bytes.Buffer
		err := s.Capture(&out, nil).Last(args[0], args[1:]...)
		if err != nil || !outputRE.MatchString(out.String()) {
			time.Sleep(mounttableWaitRetryPeriod)
			continue
		} else {
			up = true
			break
		}
	}
	if !up {
		return fmt.Errorf("mounttable %q not up after 5 minute", mounttableRoot)
	}
	return nil
}

func (u *updater) roleCmd(role string, cmd []string) []string {
	return append([]string{
		u.bin("gcreds"),
		fmt.Sprintf("--oauth-blesser=https://%s/auth/google/bless", u.hostname),
		u.bin("vbecome"),
		fmt.Sprintf("--role=/ns.%s:8101/%s", u.hostname, role),
	}, cmd...)
}

func (u *updater) adminCmd(cmd ...string) []string {
	return u.roleCmd(adminRole, cmd)
}

func (u *updater) publisherCmd(cmd ...string) []string {
	return u.roleCmd(publisherRole, cmd)
}

func (u *updater) globalMtFlag() string {
	return fmt.Sprintf("--v23.namespace.root=/ns.%s:8101", u.hostname)
}

func (u *updater) localMtFlag() string {
	return fmt.Sprintf("--v23.namespace.root=/ns.%s:8151", u.hostname)
}

func (u *updater) bin(name string) string {
	return filepath.Join(u.jirix.Root, "release", "go", "bin", name)
}

func (u *updater) outputCmd(args []string) {
	fmt.Fprintf(u.jirix.Stdout(), "Running:\n%s\n", strings.Join(args, " \\\n  "))
}

// vanadiumReleaseCandidate updates binaries of staging cloud services and run tests for them.
func vanadiumReleaseCandidate(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	cleanup, err := initTest(jirix, testName, []string{"v23:base"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Extract release candidate timestamp from env var.
	u := newUpdater(jirix, hostNameStaging)
	rcTimestamp, err := u.extractRCTimestamp()
	if err != nil {
		return nil, newInternalError(err, "Extract release candidate timestamp")
	}
	fmt.Fprintf(u.jirix.Stdout(), "Timestamp: %s\n", rcTimestamp)

	steps := []step{
		step{
			msg: "Prepare binaries",
			fn: func() error {
				if err := u.buildBinaries("v.io/..."); err != nil {
					return err
				}
				return u.uploadVanadiumBinaries(rcTimestamp)
			},
		},
	}
	steps = append(steps, genCommonSteps(u, "", rcTimestamp)...)
	steps = append(steps,
		step{
			msg: "Update the 'latest' file",
			fn:  func() error { return u.updateLatestFile(rcTimestamp) },
		})
	for _, step := range steps {
		if result, err := invoker(jirix, step.msg, step.fn); result != nil || err != nil {
			return result, err
		}
	}
	return &test.Result{Status: test.Passed}, nil
}

// vanadiumReleaseProduction updates binaries of production cloud services and runs tests for them.
func vanadiumReleaseProduction(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	cleanup, err := initTest(jirix, testName, []string{"v23:base"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	u := newUpdater(jirix, hostNameProduction)
	// Temp dir to hold release binaries.
	binDir, err := u.jirix.NewSeq().TempDir("", "")
	if err != nil {
		return nil, newInternalError(err, "TempDir")
	}
	defer u.jirix.NewSeq().RemoveAll(binDir)

	// Make sure we got a release candidate today.
	rcTimestamp := ""
	if result, err := invoker(jirix, "Check release candidate status", func() error {
		rcTimestamp, err = u.checkReleaseCandidateStatus()
		return err
	}); result != nil || err != nil {
		return result, err
	}

	steps := []step{
		step{
			msg: "Prepare tools",
			fn:  func() error { return u.buildBinaries(toolsPackages...) },
		},
		step{
			msg: "Download release binaries",
			fn:  func() error { return u.downloadReleaseBinaries(binDir) },
		},
	}
	steps = append(steps, genCommonSteps(u, binDir, rcTimestamp)...)
	for _, step := range steps {
		if result, err := invoker(jirix, step.msg, step.fn); result != nil || err != nil {
			return result, err
		}
	}
	return &test.Result{Status: test.Passed}, nil
}

// invoker invokes the given function and returns test.Result and/or
// errors based on function's results.
func invoker(jirix *jiri.X, msg string, fn func() error) (*test.Result, error) {
	fmt.Fprintf(jirix.Stdout(), banner(msg))
	if err := fn(); err != nil {
		fmt.Fprintf(jirix.Stderr(), "%s\n", err.Error())
		test.Fail(jirix.Context, "\n\n")
		if runutil.IsTimeout(err) {
			return &test.Result{
				Status:       test.TimedOut,
				TimeoutValue: defaultReleaseTestTimeout,
			}, nil
		}
		return nil, newInternalError(err, msg)
	}
	test.Pass(jirix.Context, "\n\n")
	return nil, nil
}

// banner generates a banner like this:
// ##############
// # banner msg #
// ##############
func banner(msg string) string {
	s := strings.Repeat("#", len(msg)+4)
	return fmt.Sprintf("%s\n# %s #\n%s\n", s, msg, s)
}

// vanadiumReleaseCandidateSnapshot takes a snapshot of the current JIRI_ROOT and
// writes the symlink target (the relative path to JIRI_ROOT) of that snapshot
// in the form of "<manifestEnvVar>=<symlinkTarget>" to
// "JIRI_ROOT/<snapshotManifestFile>".
func vanadiumReleaseCandidateSnapshot(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	cleanup, err := initTest(jirix, testName, nil)
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// TODO(nlacasse): Are we going to continue storing snapshots here?  Maybe
	// we need some configuation to tell us where these should be, so we don't
	// need to hard-code this path.
	manifestDir := filepath.Join(jirix.Root, "manifest")
	snapshotDir := filepath.Join(manifestDir, "snapshot")

	// Take snapshot.
	args := []string{
		"snapshot",
		"--dir=" + snapshotDir,
		"create",
		"--push-remote",
		// TODO(jingjin): change this to use "date-rc<n>" format when the function is ready.
		"--time-format=" + rcTimeFormat,
		snapshotName,
	}
	s := jirix.NewSeq()
	if err := s.Last("jiri", args...); err != nil {
		return nil, newInternalError(err, "Snapshot")
	}

	// Get the symlink target of the newly created snapshot manifest.
	symlink := filepath.Join(snapshotDir, snapshotName)
	target, err := filepath.EvalSymlinks(symlink)
	if err != nil {
		return nil, newInternalError(fmt.Errorf("EvalSymlinks(%s) failed: %v", symlink, err), "Resolve Snapshot Symlink")
	}

	// Get manifest file's relative path to the root manifest dir.
	relativePath := strings.TrimPrefix(target, manifestDir+string(filepath.Separator))

	// Get all the tests to run.
	config, err := util.LoadConfig(jirix)
	if err != nil {
		return nil, newInternalError(err, "LoadConfig")
	}
	tests := config.GroupTests([]string{"go", "java", "javascript", "projects", "third_party-go"})
	testsWithParts := []string{}
	// Append the part suffix to tests that have multiple parts specified in the config file.
	for _, test := range tests {
		if parts := config.TestParts(test); parts != nil {
			for i := 0; i <= len(parts); i++ {
				testsWithParts = append(testsWithParts, fmt.Sprintf("%s-part%d", test, i))
			}
		} else {
			testsWithParts = append(testsWithParts, test)
		}
	}
	sort.Strings(testsWithParts)

	// Write to the properties file.
	content := fmt.Sprintf("%s=%s\n%s=%s", manifestEnvVar, relativePath, testsEnvVar, strings.Join(testsWithParts, " "))
	if err := s.WriteFile(filepath.Join(jirix.Root, propertiesFile), []byte(content), os.FileMode(0644)).Done(); err != nil {
		return nil, newInternalError(err, "Record Properties")
	}

	return &test.Result{Status: test.Passed}, nil
}

func genCommonSteps(u *updater, binDir, rcTimestamp string) []step {
	return []step{
		step{
			msg: "Publish binaries",
			fn:  func() error { return u.publishBinaries(binDir) },
		},
		step{
			msg: "Update installations",
			fn:  u.updateInstallations,
		},
		step{
			msg: "Update mounttable",
			fn:  u.updateMounttable,
		},
		step{
			msg: "Update non-mounttable apps",
			fn:  u.updateNonMounttableInstances,
		},
		step{
			msg: "Check manifest timestamps of all apps",
			fn:  func() error { return u.checkManifestTimestamps("devmgr/apps/*/*/*", rcTimestamp, 8) },
		},
		step{
			msg: "Update device manager",
			fn:  u.updateDeviceManager,
		},
		step{
			msg: "Check manifest timestamps of device manager",
			fn:  func() error { return u.checkManifestTimestamps("devmgr/__debug", rcTimestamp, 1) },
		},
		step{
			msg: "Health check",
			fn:  u.checkServices,
		},
	}
}
