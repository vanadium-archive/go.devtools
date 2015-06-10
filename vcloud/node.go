// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"v.io/x/lib/cmdline"
)

var cmdNode = &cmdline.Command{
	Name:     "node",
	Short:    "Manage GCE nodes",
	Long:     "Manage GCE nodes.",
	Children: []*cmdline.Command{cmdNodeAuthorize, cmdNodeDeauthorize, cmdNodeCreate, cmdNodeDelete},
}

var cmdNodeAuthorize = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runNodeAuthorize),
	Name:   "authorize",
	Short:  "Authorize a user to login to a GCE node",
	Long: `
Authorizes a user to login to a GCE node (possibly as other user). For
instance, this mechanism is used to give Jenkins slave nodes access to
the GCE mirror of Vanadium repositories.
`,
	ArgsName: "<userA>@<hostA> [<userB>@]<hostB>",
	ArgsLong: `
<userA>@<hostA> [<userB>@]<hostB> authorizes userA to log into GCE
node hostB from GCE node hostA as user userB. The default value for
userB is userA.
`,
}

var cmdNodeDeauthorize = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runNodeDeauthorize),
	Name:   "deauthorize",
	Short:  "Deauthorize a user to login to a GCE node",
	Long: `
Deuthorizes a user to login to a GCE node (possibly as other
user). For instance, this mechanism is used to revoke access of give
Jenkins slave nodes to the GCE mirror of Vanadium repositories.
`,
	ArgsName: "<userA>@<hostA> [<userB>@]<hostB>",
	ArgsLong: `
<userA>@<hostA> [<userB>@]<hostB> deauthorizes userA to log into GCE
node hostB from GCE node hostA as user userB. The default value for
userB is userA.
`,
}

func parseUserAndHost(args []string) (string, string, string, string, error) {
	if got, want := len(args), 2; got != want {
		return "", "", "", "", fmt.Errorf("unexpected number of arguments: got %v, want %d", got, want)
	}

	parseFn := func(s string) (string, string, error) {
		tokens := strings.Split(s, "@")
		switch len(tokens) {
		case 1:
			return "", tokens[0], nil
		case 2:
			return tokens[0], tokens[1], nil
		default:
			return "", "", fmt.Errorf("unexpected length of %v: expected at most %d", tokens, 2)
		}
	}

	userA, hostA, err := parseFn(args[0])
	if err != nil {
		return "", "", "", "", err
	}
	if userA == "" {
		return "", "", "", "", fmt.Errorf("failed to parse user: %v", args[0])
	}
	userB, hostB, err := parseFn(args[1])
	if err != nil {
		return "", "", "", "", err
	}
	if userB == "" {
		userB = userA
	}

	return userA, hostA, userB, hostB, nil
}

// TODO(jsimsa): Add command-line flags for specifying the name of the
// SSH key file to use and whether to create one if it does not
// exist.
func runNodeAuthorize(env *cmdline.Env, args []string) error {
	userA, hostA, userB, hostB, err := parseUserAndHost(args)
	if err != nil {
		return env.UsageErrorf("%v", err)
	}

	// Copy the public SSH key for <userA> from <hostA> to the local
	// machine.
	ctx := newContext(env)
	tmpDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		return fmt.Errorf("TempDir() failed: %v", err)
	}
	defer ctx.Run().RemoveAll(tmpDir)
	allNodes, err := listAll(ctx, *flagDryRun)
	if err != nil {
		return err
	}
	nodeA, err := allNodes.MatchNames(hostA)
	if err != nil {
		return err
	}
	if err := nodeA.RunCopy(ctx, []string{fmt.Sprintf(":/home/%v/.ssh/id_rsa.pub", userA)}, tmpDir); err != nil {
		return err
	}

	// Append the key to the set of authorized keys of <userB> on
	// <hostB>.
	sshKeyFile := filepath.Join(tmpDir, "id_rsa.pub")
	bytes, err := ctx.Run().ReadFile(sshKeyFile)
	if err != nil {
		return fmt.Errorf("ReadFile(%v) failed: %v", sshKeyFile, err)
	}
	nodeB, err := allNodes.MatchNames(hostB)
	if err != nil {
		return err
	}
	echoCmd := []string{"echo", strings.TrimSpace(string(bytes)), ">>", fmt.Sprintf("/home/%v/.ssh/authorized_keys", userB)}
	if err := nodeB.RunCommand(ctx, userB, echoCmd); err != nil {
		return err
	}

	return nil
}

func runNodeDeauthorize(env *cmdline.Env, args []string) error {
	userA, hostA, userB, hostB, err := parseUserAndHost(args)
	if err != nil {
		return env.UsageErrorf("%v", err)
	}

	// Remove all keys for <userA>@<hostA> from the set of authorized
	// keys of <userB> on <hostB>.
	ctx := newContext(env)
	allNodes, err := listAll(ctx, *flagDryRun)
	if err != nil {
		return err
	}
	nodeB, err := allNodes.MatchNames(hostB)
	if err != nil {
		return err
	}
	authorizedKeysFile := fmt.Sprintf("/home/%v/.ssh/authorized_keys", userB)
	tmpKeysFile := authorizedKeysFile + ".tmp"
	grepCmd := []string{"grep", "-v", fmt.Sprintf("%v@%v", userA, hostA), authorizedKeysFile, ">", tmpKeysFile}
	if err := nodeB.RunCommand(ctx, userB, grepCmd); err != nil {
		return err
	}
	moveCmd := []string{"mv", tmpKeysFile, authorizedKeysFile}
	if err := nodeB.RunCommand(ctx, userB, moveCmd); err != nil {
		return err
	}

	return nil
}

var cmdNodeCreate = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runNodeCreate),
	Name:   "create",
	Short:  "Create GCE nodes",
	Long: `
Create GCE nodes. Runs 'gcloud compute instances create'.
`,
	ArgsName: "<names>",
	ArgsLong: "<names> is a list of names identifying nodes to be created.",
}

var cmdNodeDelete = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runNodeDelete),
	Name:   "delete",
	Short:  "Delete GCE nodes",
	Long: `
Delete GCE nodes. Runs 'gcloud compute instances delete'.
`,
	ArgsName: "<names>",
	ArgsLong: "<names> is a list of names identifying nodes to be deleted.",
}

func runNodeCreate(env *cmdline.Env, args []string) error {
	ctx := newContext(env)

	// Create the GCE node(s).
	createArgs := []string{
		"compute",
		"--project", *flagProject,
		"instances",
		"create",
	}
	createArgs = append(createArgs, args...)
	createArgs = append(createArgs,
		"--boot-disk-size", flagBootDiskSize,
		"--image", flagImage,
		"--machine-type", flagMachineType,
		"--zone", flagZone,
		"--scopes", "storage-full,logging-write",
	)
	if err := ctx.Run().Command("gcloud", createArgs...); err != nil {
		return err
	}

	// Create in-memory representation of node information.
	allNodes, err := listAll(ctx, *flagDryRun)
	if err != nil {
		return err
	}
	nodes, err := allNodes.MatchNames(strings.Join(args, ","))
	if err != nil {
		return err
	}

	// Wait for the SSH server on all nodes to start up.
	const numRetries = 10
	const retryPeriod = 5 * time.Second
	ready := false
	for i := 0; i < numRetries; i++ {
		if err := nodes.RunCommand(ctx, *flagUser, []string{"echo"}); err != nil {
			fmt.Fprintf(ctx.Stdout(), "attempt #%d to connect failed, will try again later\n", i+1)
			time.Sleep(retryPeriod)
			continue
		}
		ready = true
		break
	}
	if !ready {
		return fmt.Errorf("timed out waiting for nodes to start")
	}

	// Execute the setup script.
	if flagSetupScript != "" {
		if err := nodes.RunCopyAndRun(ctx, *flagUser, []string{flagSetupScript}, nil, ""); err != nil {
			return err
		}
	}

	return nil
}

func runNodeDelete(env *cmdline.Env, args []string) error {
	ctx := newContext(env)

	// Delete the GCE node(s).
	var in bytes.Buffer
	in.WriteString("Y\n") // answers the [Y/n] prompt
	opts := ctx.Run().Opts()
	opts.Stdin = &in
	deleteArgs := []string{
		"compute",
		"--project", *flagProject,
		"instances",
		"delete",
	}
	deleteArgs = append(deleteArgs, args...)
	deleteArgs = append(deleteArgs, "--zone", flagZone)
	if err := ctx.Run().CommandWithOpts(opts, "gcloud", deleteArgs...); err != nil {
		return err
	}

	return nil
}
