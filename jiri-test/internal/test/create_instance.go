// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"v.io/jiri"
	"v.io/jiri/collect"
	"v.io/jiri/runutil"
	"v.io/x/devtools/internal/test"
)

const (
	scriptEnvVar       = "CREATE_INSTANCE_SCRIPT"
	projectEnvVar      = "CREATE_INSTANCE_PROJECT_ID"
	testInstancePrefix = "create-instance-test"
	testInstanceZone   = "us-central1-c"
)

var (
	defaultCreateInstanceTimeout = time.Minute * 10
	defaultCheckInstanceTimeout  = time.Minute * 5
	testInstanceProject          = os.Getenv(projectEnvVar)
)

type instance struct {
	Name              string
	Zone              string
	NetworkInterfaces []struct {
		AccessConfigs []struct {
			NatIP string
		}
	}
}

// vanadiumCreateInstanceTest creates a test instance using the
// create_instance.sh script (specified in the CREATE_INSTANCE_SCRIPT
// environment variable) and run prod service test and load test againest it.
func vanadiumCreateInstanceTest(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	if testInstanceProject == "" {
		return nil, newInternalError(fmt.Errorf("project not defined in %s environment variable", projectEnvVar), "Env")
	}

	// Check CREATE_INSTANCE_SCRIPT environment variable.
	script := os.Getenv(scriptEnvVar)
	if script == "" {
		return nil, newInternalError(fmt.Errorf("script not defined in %s environment variable", scriptEnvVar), "Env")
	}

	cleanup, err := initTest(jirix, testName, []string{"v23:base"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Clean up test instances possibly left by previous test runs.
	if err := cleanupTestInstances(jirix); err != nil {
		return nil, newInternalError(err, "Delete old test instances")
	}

	// Run script.
	printBanner(jirix, fmt.Sprintf("Running instance creation script: %s", script))
	instanceName := fmt.Sprintf("%s-%s", testInstancePrefix, time.Now().Format("20060102150405"))
	defer collect.Error(func() error {
		// TODO(caprita): Commenting this out so we can debug failures.
		// return cleanupTestInstances(jirix)
		return nil
	}, &e)
	go func() {
		sigchan := make(chan os.Signal, 1)
		signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
		<-sigchan
		if err := cleanupTestInstances(jirix); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}
		os.Exit(0)
	}()
	if err := runScript(jirix, script, instanceName); err != nil {
		if runutil.IsTimeout(err) {
			return &test.Result{
				Status:       test.TimedOut,
				TimeoutValue: defaultCreateInstanceTimeout,
			}, nil
		}
		return nil, newInternalError(err, err.Error())
	}

	// Check the test instance.
	printBanner(jirix, "Checking test instance")
	if err := checkTestInstance(jirix, instanceName); err != nil {
		if runutil.IsTimeout(err) {
			return &test.Result{
				Status:       test.TimedOut,
				TimeoutValue: defaultCheckInstanceTimeout,
			}, nil
		}
		return nil, newInternalError(err, err.Error())
	}

	return &test.Result{Status: test.Passed}, nil
}

func cleanupTestInstances(jirix *jiri.X) error {
	printBanner(jirix, "Cleaning up test instances")

	// List all test instances.
	instances, err := listInstances(jirix, testInstancePrefix+".*")
	if err != nil {
		return err
	}

	// Delete them.
	for _, instance := range instances {
		if err := deleteInstance(jirix, instance.Name, instance.Zone); err != nil {
			fmt.Fprintf(jirix.Stderr(), "%v", err)
		}
	}
	return nil
}

func listInstances(jirix *jiri.X, instanceRegEx string) ([]instance, error) {
	var out bytes.Buffer
	args := []string{
		"-q",
		"compute",
		"instances",
		"list",
		"--project",
		testInstanceProject,
		fmt.Sprintf("--regexp=%s", instanceRegEx),
		"--format=json",
	}
	if err := jirix.NewSeq().Capture(&out, nil).Last("gcloud", args...); err != nil {
		return nil, err
	}
	instances := []instance{}
	if err := json.Unmarshal(out.Bytes(), &instances); err != nil {
		return nil, fmt.Errorf("Unmarshal() failed: %v", err)
	}
	return instances, nil
}

func deleteInstance(jirix *jiri.X, instanceName, instanceZone string) error {
	args := []string{
		"-q",
		"compute",
		"instances",
		"delete",
		"--project",
		testInstanceProject,
		"--zone",
		instanceZone,
		instanceName,
	}
	if err := jirix.NewSeq().Last("gcloud", args...); err != nil {
		return err
	}
	return nil
}

func runScript(jirix *jiri.X, script, instanceName string) error {
	s := jirix.NewSeq()
	// Build all binaries.
	args := []string{"go", "install", "v.io/..."}
	return s.Capture(jirix.Stdout(), jirix.Stderr()).Run("jiri", args...).
		Timeout(defaultCreateInstanceTimeout).Last(script, instanceName)
}

func checkTestInstance(jirix *jiri.X, instanceName string) error {
	instances, err := listInstances(jirix, instanceName)
	if err != nil {
		return err
	}
	if len(instances) == 0 {
		return fmt.Errorf("no matching instance for %q", instanceName)
	}
	externalIP := instances[0].NetworkInterfaces[0].AccessConfigs[0].NatIP
	suites := testAllProdServices(jirix, "", fmt.Sprintf("/%s:8101", externalIP))
	allPassed := true
	for _, suite := range suites {
		allPassed = allPassed && (suite.Failures == 0)
	}
	if !allPassed {
		return fmt.Errorf("some checks failed")
	}
	return nil
}

func printBanner(jirix *jiri.X, msg string) {
	fmt.Fprintf(jirix.Stdout(), "##### %s #####\n", msg)
}
