// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"sort"
	"strings"

	"v.io/x/devtools/internal/tool"
	"v.io/x/lib/cmdline2"
)

// checkFunctions is a map from check names to the corresponding check functions.
var checkFunctions = map[string]func(*tool.Context) error{
	"service-latency":  checkServiceLatency,
	"service-counters": checkServiceCounters,
	"service-qps":      checkAllServiceQPS,
	"gce-instance":     checkGCEInstances,
	"rpc-load-test":    checkRPCLoadTest,
}

// cmdCheck represents the "check" command of the vmon tool.
var cmdCheck = &cmdline2.Command{
	Name:  "check",
	Short: "Manage checks whose results are used in GCM for alerting and graphing",
	Long:  "Manage checks whose results are used in GCM for alerting and graphing.",
	Children: []*cmdline2.Command{
		cmdCheckList,
		cmdCheckRun,
	},
}

// cmdCheckList represents the "vmon check list" command.
var cmdCheckList = &cmdline2.Command{
	Runner: cmdline2.RunnerFunc(runCheckList),
	Name:   "list",
	Short:  "List known checks",
	Long:   "List known checks.",
}

func runCheckList(env *cmdline2.Env, _ []string) error {
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
var cmdCheckRun = &cmdline2.Command{
	Runner:   cmdline2.RunnerFunc(runCheckRun),
	Name:     "run",
	Short:    "Run the given checks",
	Long:     "Run the given checks.",
	ArgsName: "<names>",
	ArgsLong: "<names> is a list of names identifying the checks to run. Available: " + strings.Join(knownCheckNames(), ", "),
}

func runCheckRun(env *cmdline2.Env, args []string) error {
	// Check args.
	for _, arg := range args {
		if _, ok := checkFunctions[arg]; !ok {
			return env.UsageErrorf("check %v does not exist", arg)
		}
	}
	if len(args) == 0 {
		return env.UsageErrorf("no checks provided")
	}

	// Run checks.
	hasError := false
	ctx := tool.NewContextFromEnv(env, tool.ContextOpts{
		Color:   &colorFlag,
		Verbose: &verboseFlag,
	})
	for _, check := range args {
		// We already checked the given checks all exist.
		checkFn, _ := checkFunctions[check]
		fmt.Fprintf(ctx.Stdout(), "##### Running check %q #####\n", check)
		err := checkFn(ctx)
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
