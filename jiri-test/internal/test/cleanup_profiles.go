// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bytes"
	"fmt"
	"strings"
	"sync"

	"v.io/jiri/jiri"
)

var cleanupOnce sync.Once
var cleanupError error

// cleanupProfiles is the entry point for ensuring profiles are in a sane
// state prior to running tests. There is often a need to get the current
// profiles installation into a sane state, either because of previous bugs,
// or to prepare for a subsequent change. This function is the entry point for that.
func cleanupProfiles(jirix *jiri.X) error {
	cleanupOnce.Do(func() { cleanupError = cleanupProfilesImpl(jirix) })
	return cleanupError
}

func cleanupProfilesImpl(jirix *jiri.X) error {
	cmds := []string{"list"}
	cleanup := []string{"cleanup --gc"}
	fmt.Fprintf(jirix.Stdout(), "cleanupProfiles: commands: %s\n", cleanup)
	cmds = append(cmds, cleanup...)
	cmds = append(cmds, "list")
	removals := []string{"cleanup -rm-all"}
	if isCI() {
		fmt.Fprintf(jirix.Stdout(), "cleanupProfiles: remove: %s\n", removals)
		if len(removals) > 0 {
			cmds = append(cmds, removals...)
			cmds = append(cmds, "list")
		}
	} else {
		fmt.Fprintf(jirix.Stdout(), "cleanupProfiles: skipping removals when not on CI\n")
	}
	s := jirix.NewSeq()
	for _, args := range cmds {
		var out bytes.Buffer
		clargs := append([]string{"v23-profile"}, strings.Split(args, " ")...)
		err := s.Capture(&out, &out).Last("jiri", clargs...)
		fmt.Fprintf(jirix.Stdout(), "jiri %v: %v [[\n", strings.Join(clargs, " "), err)
		fmt.Fprintf(jirix.Stdout(), "%s]]\n", out.String())
	}
	return nil
}

func displayProfiles(jirix *jiri.X, msg string) {
	var out bytes.Buffer
	s := jirix.NewSeq()
	fmt.Fprintf(jirix.Stdout(), "%s: installed profiles:\n", msg)
	err := s.Capture(&out, &out).Last("jiri", "v23-profile", "list", "--v")
	if err != nil {
		fmt.Fprintf(jirix.Stdout(), " %v\n", err)
		return
	}
	fmt.Fprintf(jirix.Stdout(), "\n%s\n", out.String())
	out.Reset()
	fmt.Fprintf(jirix.Stdout(), "recreate profiles with:\n")
	err = s.Capture(&out, &out).Last("jiri", "v23-profile", "list", "--info", "Target.Command")
	if err != nil {
		fmt.Fprintf(jirix.Stdout(), " %v\n", err)
		return
	}
	fmt.Fprintf(jirix.Stdout(), "\n%s\n", out.String())
}
