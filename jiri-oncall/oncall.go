// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go -env=CMDLINE_PREFIX=jiri .

package main

import (
	"fmt"
	"time"

	"v.io/jiri/jiri"
	"v.io/jiri/tool"
	"v.io/jiri/util"
	"v.io/x/lib/cmdline"
)

func init() {
	tool.InitializeRunFlags(&cmdOncall.Flags)
}

// cmdOncall represents the "jiri oncall" command.
var cmdOncall = &cmdline.Command{
	Runner: jiri.RunnerFunc(runOncall),
	Name:   "oncall",
	Short:  "Manage vanadium oncall schedule",
	Long: `
Manage vanadium oncall schedule. If no subcommand is given, it shows the LDAP
of the current oncall.
`,
	Children: []*cmdline.Command{cmdOncallList},
}

// cmdOncallList represents the "jiri oncall list" command.
var cmdOncallList = &cmdline.Command{
	Runner: jiri.RunnerFunc(runOncallList),
	Name:   "list",
	Short:  "List available oncall schedule",
	Long:   "List available oncall schedule.",
}

func runOncall(jirix *jiri.X, _ []string) error {
	shift, err := util.Oncall(jirix, time.Now())
	if err != nil {
		return err
	}
	fmt.Fprintf(jirix.Stdout(), "%s,%s\n", shift.Primary, shift.Secondary)
	return nil
}

func runOncallList(jirix *jiri.X, _ []string) error {
	rotation, err := util.LoadOncallRotation(jirix)
	if err != nil {
		return err
	}
	// Print the schedule with the current oncall marked.
	layout := "Jan 2, 2006 3:04:05 PM"
	now := time.Now().Unix()
	foundOncall := false
	for i, shift := range rotation.Shifts {
		prefix := "   "
		if !foundOncall && i < len(rotation.Shifts)-1 {
			nextDate := rotation.Shifts[i+1].Date
			nextTimestamp, err := time.Parse(layout, nextDate)
			if err != nil {
				fmt.Fprintf(jirix.Stderr(), "Parse(%q, %v) failed: %v", layout, nextDate, err)
				continue
			}
			if now < nextTimestamp.Unix() {
				prefix = "-> "
				foundOncall = true
			}
		}
		if i == len(rotation.Shifts)-1 && !foundOncall {
			prefix = "-> "
		}
		fmt.Fprintf(jirix.Stdout(), "%s%25s: %s\n", prefix, shift.Date, shift.Primary)
	}
	return nil
}

func main() {
	cmdline.Main(cmdOncall)
}
