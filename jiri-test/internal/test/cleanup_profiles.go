// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bytes"
	"fmt"
	"strings"
	"sync"

	"v.io/jiri/tool"
)

var cleanupOnce sync.Once
var cleanupError error

// sanitizeProfiles is the entry point for ensuring profiles are in a sane
// state prior to running tests. There is often a need to get the current
// profiles installation into a sane state, either because of previous bugs,
// or to prepare for a subsequent change. This function is the entry point for that.
func cleanupProfiles(ctx *tool.Context) error {
	cleanupOnce.Do(func() { cleanupError = cleanupProfilesImpl(ctx) })
	return cleanupError
}

func cleanupProfilesImpl(ctx *tool.Context) error {
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out

	cmds := []string{"list"}
	cleanup := []string{"cleanup --ensure-specific-versions-are-set --gc"}
	fmt.Fprintf(ctx.Stdout(), "cleanupProfiles: commands: %s\n", cleanup)
	cmds = append(cmds, cleanup...)
	cmds = append(cmds, "list")
	removals := []string{"uninstall --all-targets nacl"}
	fmt.Fprintf(ctx.Stdout(), "cleanupProfiles: remove: %s\n", removals)
	cmds = append(cmds, removals...)
	cmds = append(cmds, "list")
	for _, args := range cmds {
		clargs := append([]string{"v23-profile"}, strings.Split(args, " ")...)
		err := ctx.Run().CommandWithOpts(opts, "jiri", clargs...)
		fmt.Fprintf(ctx.Stdout(), "jiri %v: %v [[\n", strings.Join(clargs, " "), err)
		fmt.Fprintf(ctx.Stdout(), "%s]]\n", out.String())
		out.Reset()
	}
	return nil
}

func displayProfiles(ctx *tool.Context, msg string) {
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	fmt.Fprintf(ctx.Stdout(), "%s: installed profiles:\n", msg)
	err := ctx.Run().CommandWithOpts(opts, "jiri", "v23-profile", "list", "--v")
	if err != nil {
		fmt.Fprintf(ctx.Stdout(), " %v\n", err)
		return
	}
	fmt.Fprintf(ctx.Stdout(), "\n%s\n", out.String())
	out.Reset()
	fmt.Fprintf(ctx.Stdout(), "recreate profiles with:\n")
	err = ctx.Run().CommandWithOpts(opts, "jiri", "v23-profile", "list", "--info", "Target.Command")
	if err != nil {
		fmt.Fprintf(ctx.Stdout(), " %v\n", err)
		return
	}
	fmt.Fprintf(ctx.Stdout(), "\n%s\n", out.String())
}
