// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"sort"
	"strings"

	"v.io/x/devtools/internal/tool"
	"v.io/x/lib/cmdline"
)

// checkFunctions is a map from check names to the corresponding check functions.
var checkFunctions = map[string]func(*tool.Context) error{
	"service-latency": checkServiceLatency,
}

// cmdCheck represents the "check" command of the vmon tool.
var cmdCheck = &cmdline.Command{
	Name:  "check",
	Short: "Manage checks whose results are used in GCM for alerting and graphing",
	Long:  "Manage checks whose results are used in GCM for alerting and graphing.",
	Children: []*cmdline.Command{
		cmdCheckList,
		cmdCheckRun,
	},
}

// cmdCheckList represents the "vmon check list" command.
var cmdCheckList = &cmdline.Command{
	Run:   runCheckList,
	Name:  "list",
	Short: "List known checks",
	Long:  "List known checks.",
}

func runCheckList(command *cmdline.Command, _ []string) error {
	checks := []string{}
	for name := range checkFunctions {
		checks = append(checks, name)
	}
	sort.Strings(checks)
	for _, check := range checks {
		fmt.Fprintf(command.Stdout(), "%v\n", check)
	}
	return nil
}

// cmdCheckRun represents the "vmon check run" command.
var cmdCheckRun = &cmdline.Command{
	Run:      runCheckRun,
	Name:     "run",
	Short:    "Run the given checks",
	Long:     "Run the given checks.",
	ArgsName: "<names>",
	ArgsLong: "<names> is a list of names identifying the checks to run. Available: " + strings.Join(knownCheckNames(), ", "),
}

func runCheckRun(command *cmdline.Command, args []string) error {
	// Check args.
	for _, arg := range args {
		if _, ok := checkFunctions[arg]; !ok {
			return command.UsageErrorf("check %v does not exist", arg)
		}
	}
	if len(args) == 0 {
		return command.UsageErrorf("no checks provided")
	}

	// Run checks.
	hasError := false
	ctx := tool.NewContextFromCommand(command, tool.ContextOpts{
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
