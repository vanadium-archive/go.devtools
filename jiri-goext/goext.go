// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $V23_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go .

package main

import (
	"path/filepath"
	"strings"

	"v.io/x/devtools/internal/project"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/lib/cmdline"
)

func init() {
	tool.InitializeRunFlags(&cmdGoExt.Flags)
}

// cmdGoExt represents the "v23 goext" command.
var cmdGoExt = &cmdline.Command{
	Name:     "goext",
	Short:    "Vanadium extensions of the go tool",
	Long:     "Vanadium extension of the go tool.",
	Children: []*cmdline.Command{cmdGoExtDistClean},
}

// cmdGoExtDistClean represents the "v23 goext distclean" command.
var cmdGoExtDistClean = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runGoExtDistClean),
	Name:   "distclean",
	Short:  "Restore the vanadium Go workspaces to their pristine state",
	Long: `
Unlike the 'go clean' command, which only removes object files for
packages in the source tree, the 'goext disclean' command removes all
object files from vanadium Go workspaces. This functionality is needed
to avoid accidental use of stale object files that correspond to
packages that no longer exist in the source tree.
`,
}

func runGoExtDistClean(cmdlineEnv *cmdline.Env, _ []string) error {
	ctx := tool.NewContextFromEnv(cmdlineEnv)
	root, err := project.V23Root()
	if err != nil {
		return err
	}
	env, err := util.VanadiumEnvironment(ctx)
	if err != nil {
		return err
	}
	failed := false

	for _, workspace := range env.GetTokens("GOPATH", ":") {
		if !strings.HasPrefix(workspace, root) {
			continue
		}
		for _, name := range []string{"bin", "pkg"} {
			dir := filepath.Join(workspace, name)
			if err := ctx.Run().RemoveAll(dir); err != nil {
				failed = true
			}
		}
	}
	if failed {
		return cmdline.ErrExitCode(2)
	}
	return nil
}

func main() {
	cmdline.Main(cmdGoExt)
}
