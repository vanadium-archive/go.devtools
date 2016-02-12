// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"v.io/jiri/collect"
	"v.io/jiri/jiri"
	"v.io/jiri/runutil"
	"v.io/jiri/util"
	"v.io/x/devtools/internal/test"
)

const (
	bucket                  = "gs://vanadium-release"
	localMountTable         = "/ns.dev.staging.v.io:8151"
	globalMountTable        = "/ns.dev.staging.v.io:8101"
	oauthBlesserService     = "https://dev.staging.v.io/auth/google/bless"
	adminRole               = "identity/role/vprod/admin"
	publisherRole           = "identity/role/vprod/publisher"
	manifestEnvVar          = "SNAPSHOT_MANIFEST"
	numRetries              = 30
	objNameForDeviceManager = "devices/vanadium-cell-master/devmgr/device"
	propertiesFile          = ".release_candidate_properties"
	retryPeriod             = 10 * time.Second
	stagingBlessingsRoot    = "dev.staging.v.io" // TODO(jingjin): use a better name and update prod.go.
	snapshotName            = "rc"
	testsEnvVar             = "TESTS"
)

var (
	defaultReleaseTestTimeout = time.Minute * 5
	manifestRE                = regexp.MustCompile(`.*<manifest snapshotpath="(.*)">`)

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

	deviceManagerApplications = []string{
		"devmgr/apps/applicationd",
		"devmgr/apps/binaryd",
		"devmgr/apps/groupsd",
		"devmgr/apps/identityd",
		"devmgr/apps/roled",
		"devmgr/apps/xproxyd",
		"devmgr/apps/VLabXProxy",
	}
)

// vanadiumReleaseCandidate updates binaries of staging cloud services and run tests for them.
func vanadiumReleaseCandidate(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	cleanup, err := initTest(jirix, testName, []string{"base", "java"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	type step struct {
		msg string
		fn  func() error
	}
	rcTimestamp := ""
	steps := []step{
		step{
			msg: "Extract release candidate path\n",
			fn: func() error {
				rcPath, err := extractRCPath()
				if err != nil {
					return err
				}
				rcTimestamp = filepath.Base(rcPath)
				return nil
			},
		},
		step{
			msg: "Prepare binaries\n",
			fn:  func() error { return prepareBinaries(jirix, rcTimestamp) },
		},
		step{
			msg: "Update services\n",
			fn:  func() error { return updateServices(jirix) },
		},
		step{
			msg: "Check services\n",
			fn: func() error {
				// Wait 5 minutes.
				fmt.Fprintf(jirix.Stdout(), "Wait for 5 minutes before checking services...\n")
				time.Sleep(time.Minute * 5)

				return checkServices(jirix)
			},
		},
		step{
			msg: "Update the 'latest' file\n",
			fn:  func() error { return updateLatestFile(jirix, rcTimestamp) },
		},
	}
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
	if err := fn(); err != nil {
		test.Fail(jirix.Context, msg)
		if runutil.IsTimeout(err) {
			return &test.Result{
				Status:       test.TimedOut,
				TimeoutValue: defaultReleaseTestTimeout,
			}, nil
		}
		fmt.Fprintf(jirix.Stderr(), "%s\n", err.Error())
		return nil, newInternalError(err, msg)
	}
	test.Pass(jirix.Context, msg)
	return nil, nil
}

// extractRCPath extracts release candidate path from the manifest path stored
// in the <manifestEnvVar> environment variable.
func extractRCPath() (string, error) {
	manifestPath := os.Getenv(manifestEnvVar)
	if manifestPath == "" {
		return "", fmt.Errorf("Environment variable %q not set", manifestEnvVar)
	}
	return manifestPath, nil
}

// prepareBinaries builds all vanadium binaries and uploads them to Google Storage bucket.
func prepareBinaries(jirix *jiri.X, rcTimestamp string) error {
	s := jirix.NewSeq()

	// Build and upload binaries.
	//
	// The "leveldb" tag is needed to compile the levelDB-based storage
	// engine for the groups service. See v.io/i/632 for more details.

	// Upload the .done file to signal that all binaries have been
	// successfully uploaded.
	tmpDir, err := s.TempDir("", "")
	if err != nil {
		return err
	}
	defer jirix.NewSeq().RemoveAll(tmpDir)
	doneFile := filepath.Join(tmpDir, ".done")

	jiriArgs := []string{"go", "install", "-tags=leveldb", "v.io/..."}
	gsutilUploadArgs := []string{
		"-q", "-m", "cp", "-r",
		filepath.Join(jirix.Root, "release", "go", "bin"),
		fmt.Sprintf("%s/%s", bucket, rcTimestamp),
	}
	gsutilDoneArgs := []string{"-q", "cp", doneFile, fmt.Sprintf("%s/%s", bucket, rcTimestamp)}

	return s.Run("jiri", jiriArgs...).
		Run("gsutil", gsutilUploadArgs...).
		WriteFile(doneFile, nil, os.FileMode(0600)).
		Last("gsutil", gsutilDoneArgs...)
}

func roleCmd(jirix *jiri.X, role string, cmd []string) []string {
	return append([]string{
		filepath.Join(jirix.Root, "release", "go", "bin", "gcreds"),
		"--oauth-blesser=" + oauthBlesserService,
		filepath.Join(jirix.Root, "release", "go", "bin", "vbecome"),
		"--role=" + globalMountTable + "/" + role,
	}, cmd...)
}

func adminCmd(jirix *jiri.X, cmd []string) []string {
	return roleCmd(jirix, adminRole, cmd)
}

func publisherCmd(jirix *jiri.X, cmd []string) []string {
	return roleCmd(jirix, publisherRole, cmd)
}

// updateServices pushes services' binaries to the applications and binaries
// services and tells the device manager to update all its app.
func updateServices(jirix *jiri.X) (e error) {
	debugBin := filepath.Join(jirix.Root, "release", "go", "bin", "debug")
	deviceBin := filepath.Join(jirix.Root, "release", "go", "bin", "device")
	nsArg := fmt.Sprintf("--v23.namespace.root=%s", globalMountTable)

	s := jirix.NewSeq()

	// Push all binaries.
	{
		fmt.Fprintln(jirix.Stdout(), "\n\n### Pushing binaries ###")
		args := publisherCmd(jirix, []string{
			deviceBin,
			nsArg,
			"publish",
			"--goos=linux",
			"--goarch=amd64",
		})
		args = append(args, serviceBinaries...)
		msg := "Push binaries\n"
		if err := s.Timeout(defaultReleaseTestTimeout).Last(args[0], args[1:]...); err != nil {
			test.Fail(jirix.Context, msg)
			return err
		}
		test.Pass(jirix.Context, msg)
	}

	// A helper function to update a single app.
	updateAppFn := func(appName string) error {
		args := adminCmd(jirix, []string{
			deviceBin,
			fmt.Sprintf("--v23.namespace.root=%s", localMountTable),
			"update",
			"-parallelism=BYKIND",
			appName + "/...",
		})
		msg := fmt.Sprintf("Update %q\n", appName)
		if err := s.Timeout(defaultReleaseTestTimeout).Last(args[0], args[1:]...); err != nil {
			test.Fail(jirix.Context, msg)
			return err
		}
		test.Pass(jirix.Context, msg)
		return nil
	}

	// A helper function to check a single app's manifest snapshotpath.
	expectedManifestPath := os.Getenv(manifestEnvVar)
	checkManifestPathFn := func(appName string) error {
		msg := fmt.Sprintf("Verify manifest snapshotpath for %q\n", appName)
		args := adminCmd(jirix, []string{
			debugBin,
			fmt.Sprintf("--v23.namespace.root=%s", localMountTable),
			"stats",
			"read",
			fmt.Sprintf("%s/*/*/stats/system/metadata/build.Manifest", appName),
		})
		var out bytes.Buffer
		stdout := io.MultiWriter(jirix.Stdout(), &out)
		if err := s.Capture(stdout, nil).Timeout(defaultReleaseTestTimeout).Last(args[0], args[1:]...); err != nil {
			test.Fail(jirix.Context, msg)
			return err
		}
		statsOutput := out.String()
		matches := manifestRE.FindStringSubmatch(statsOutput)
		if matches == nil || (matches[1] != expectedManifestPath) {
			test.Fail(jirix.Context, msg)
			return fmt.Errorf("failed to verify manifest path %q.\nCurrent manifest:\n%s",
				expectedManifestPath, statsOutput)
		}
		test.Pass(jirix.Context, msg)
		return nil
	}

	// Update services except for device manager and mounttable.
	{
		fmt.Fprintln(jirix.Stdout(), "\n\n### Updating services other than device manager and mounttable ###")
		for _, app := range deviceManagerApplications {
			if err := updateAppFn(app); err != nil {
				return err
			}
			if err := checkManifestPathFn(app); err != nil {
				return err
			}
		}
	}

	// Update Device Manager.
	{
		fmt.Fprintln(jirix.Stdout(), "\n\n### Updating device manager ###")
		args := adminCmd(jirix, []string{
			deviceBin,
			fmt.Sprintf("--v23.namespace.root=%s", globalMountTable),
			"update",
			objNameForDeviceManager,
		})
		if err := s.Timeout(defaultReleaseTestTimeout).Last(args[0], args[1:]...); err != nil {
			return err
		}
		if err := waitForMounttable(jirix, localMountTable, `.*8151/devmgr.*`); err != nil {
			return err
		}
		// TODO(jingjin): check build time for device manager.
	}

	// Update mounttable last.
	{
		fmt.Fprintln(jirix.Stdout(), "\n\n### Updating mounttable ###")
		mounttableName := "devmgr/apps/mounttabled"
		if err := updateAppFn(mounttableName); err != nil {
			return err
		}
		if err := waitForMounttable(jirix, globalMountTable, `.+`); err != nil {
			return err
		}
		if err := checkManifestPathFn(mounttableName); err != nil {
			return err
		}
	}
	return nil
}

// waitForMounttable waits for the given mounttable to be up and optionally
// checks output against outputRegexp if it is not empty.
// (timeout: 5 minutes)
func waitForMounttable(jirix *jiri.X, mounttableRoot, outputRegexp string) error {
	s := jirix.NewSeq()
	debugBin := filepath.Join(jirix.Root, "release", "go", "bin", "debug")
	args := adminCmd(jirix, []string{
		debugBin,
		"glob",
		mounttableRoot + "/*",
	})
	up := false
	outputRE := regexp.MustCompile(outputRegexp)
	for i := 0; i < numRetries; i++ {
		var out bytes.Buffer
		stdout := io.MultiWriter(jirix.Stdout(), &out)
		err := s.Capture(stdout, nil).Last(args[0], args[1:]...)
		if err != nil || !outputRE.MatchString(out.String()) {
			time.Sleep(retryPeriod)
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

// checkServices runs "jiri test run vanadium-prod-services-test" against
// staging.
func checkServices(jirix *jiri.X) error {
	s := jirix.NewSeq()
	args := []string{
		"test",
		"run",
		fmt.Sprintf("--v23.namespace.root=%s", globalMountTable),
		fmt.Sprintf("--blessings-root=%s", stagingBlessingsRoot),
		"vanadium-prod-services-test",
	}
	if err := s.Timeout(defaultReleaseTestTimeout).Last("jiri", args...); err != nil {
		return err
	}
	return nil
}

// updateLatestFile updates the "latest" file in Google Storage bucket to the
// given release candidate timestamp.
func updateLatestFile(jirix *jiri.X, rcTimestamp string) error {
	s := jirix.NewSeq()
	tmpDir, err := s.TempDir("", "")
	if err != nil {
		return err
	}
	defer jirix.NewSeq().RemoveAll(tmpDir)
	latestFile := filepath.Join(tmpDir, "latest")
	args := []string{"-q", "cp", latestFile, fmt.Sprintf("%s/latest", bucket)}
	return s.WriteFile(latestFile, []byte(rcTimestamp), os.FileMode(0600)).
		Last("gsutil", args...)
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
		"--time-format=2006-01-02.15:04",
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
