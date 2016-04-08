// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"sort"
	"strings"

	cloudmonitoring "google.golang.org/api/monitoring/v3"

	"v.io/jiri/tool"
	"v.io/v23/context"
	"v.io/x/devtools/internal/monitoring"
	"v.io/x/lib/cmdline"
	"v.io/x/ref/lib/v23cmd"
)

// checkFunctions is a map from check names to the corresponding check functions.
var checkFunctions = map[string]func(*context.T, *tool.Context, *cloudmonitoring.Service) error{
	"jenkins":                   checkJenkins,
	"service-latency":           checkServiceLatency,
	"service-permethod-latency": checkServicePerMethodLatency,
	"service-counters":          checkServiceCounters,
	"service-metadata":          checkServiceMetadata,
	"service-qps":               checkServiceQPS,
	"gce-instance":              checkGCEInstances,
	"rpc-load-test":             checkRPCLoadTest,
}

// cmdCheck represents the "check" command of the vmon tool.
var cmdCheck = &cmdline.Command{
	Name:  "check",
	Short: "Manage checks used for alerting and graphing",
	Long:  "Manage checks whose results are used in GCM for alerting and graphing.",
	Children: []*cmdline.Command{
		cmdCheckList,
		cmdCheckRun,
	},
}

// cmdCheckList represents the "vmon check list" command.
var cmdCheckList = &cmdline.Command{
	Runner: v23cmd.RunnerFunc(runCheckList),
	Name:   "list",
	Short:  "List known checks",
	Long:   "List known checks.",
}

func runCheckList(_ *context.T, env *cmdline.Env, _ []string) error {
	checks := []string{}
	for name := range checkFunctions {
		checks = append(checks, name)
	}
	sort.Strings(checks)
	for _, check := range checks {
		fmt.Fprintf(env.Stdout, "%v\n", check)
	}
	return nil
}

// cmdCheckRun represents the "vmon check run" command.
var cmdCheckRun = &cmdline.Command{
	Runner:   v23cmd.RunnerFunc(runCheckRun),
	Name:     "run",
	Short:    "Run the given checks",
	Long:     "Run the given checks.",
	ArgsName: "<names>",
	ArgsLong: "<names> is a list of names identifying the checks to run. Available: " + strings.Join(knownCheckNames(), ", "),
}

func runCheckRun(v23ctx *context.T, env *cmdline.Env, args []string) error {
	// Check args.
	for _, arg := range args {
		if _, ok := checkFunctions[arg]; !ok {
			return env.UsageErrorf("check %v does not exist", arg)
		}
	}
	if len(args) == 0 {
		return env.UsageErrorf("no checks provided")
	}
	ctx := tool.NewContextFromEnv(env)

	// Authenticate monitoring APIs.
	s, err := monitoring.Authenticate(keyFileFlag)
	if err != nil {
		return err
	}

	// Run checks.
	hasError := false
	for _, check := range args {
		// We already checked the given checks all exist.
		checkFn, _ := checkFunctions[check]
		fmt.Fprintf(ctx.Stdout(), "##### Running check %q #####\n", check)
		err := checkFn(v23ctx, ctx, s)
		if err != nil {
			fmt.Fprintf(ctx.Stderr(), "%v\n", err)
			fmt.Fprintf(ctx.Stdout(), "##### FAIL #####\n")
			hasError = true
		} else {
			fmt.Fprintf(ctx.Stdout(), "##### PASS #####\n")
		}
	}
	if hasError {
		return fmt.Errorf("Failed to run some checks.")
	}

	return nil
}

func knownCheckNames() []string {
	names := []string{}
	for n := range checkFunctions {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
