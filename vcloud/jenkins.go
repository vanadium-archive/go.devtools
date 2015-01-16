package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"v.io/lib/cmdline"
)

var cmdNode = &cmdline.Command{
	Name:     "node",
	Short:    "Manage GCE jenkins slave nodes",
	Long:     "Manage GCE jenkins slave nodes.",
	Children: []*cmdline.Command{cmdNodeCreate, cmdNodeDelete},
}

var cmdNodeCreate = &cmdline.Command{
	Run:   runNodeCreate,
	Name:  "create",
	Short: "Create GCE jenkins slave nodes",
	Long: `
Create GCE jenkins slave nodes.  Runs 'gcloud compute instances create'.
`,
	ArgsName: "<names>",
	ArgsLong: "<names> is a list of names identifying nodes to be created.",
}

var cmdNodeDelete = &cmdline.Command{
	Run:   runNodeDelete,
	Name:  "delete",
	Short: "Delete GCE jenkins slave nodes",
	Long: `
Delete GCE jenkins slave nodes.  Runs 'gcloud compute instances delete'.
`,
	ArgsName: "<names>",
	ArgsLong: "<names> is a list of names identifying nodes to be deleted.",
}

const (
	numRetries  = 10
	retryPeriod = 5 * time.Second
)

func runNodeCreate(cmd *cmdline.Command, args []string) error {
	run := newRun(cmd)

	// Create the nodes.
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
		"--scopes", "storage-full",
	)
	if err := run.Command("gcloud", createArgs...); err != nil {
		return err
	}

	// Create in-memory representation of node information.
	allNodes, err := listAll(run, *flagDryRun)
	if err != nil {
		return err
	}
	nodes, err := allNodes.MatchNames(strings.Join(args, ","))
	if err != nil {
		return err
	}

	// Wait for the SSH server on all nodes to start up.
	ready := false
	for i := 0; i < numRetries; i++ {
		if err := nodes.RunCommand(run, "veyron", []string{"echo"}); err != nil {
			fmt.Fprintf(run.Opts().Stdout, "attempt #%d to connect failed, will try again later\n", i+1)
			time.Sleep(retryPeriod)
			continue
		}
		ready = true
		break
	}
	if !ready {
		return fmt.Errorf("timed out waiting for nodes to start")
	}

	// Execute the jenkins setup script.
	curlCmd := []string{"curl", "-u", "vanadium:D6HT]P,LrJ7e", "https://dev.v.io/noproxy/jenkins-setup.sh", "|", "bash"}
	if err := nodes.RunCommand(run, "veyron", curlCmd); err != nil {
		return err
	}

	// Copy the SSH keys from the newly created nodes to the local
	// machine.
	tmpDir, err := run.TempDir("", "")
	if err != nil {
		return fmt.Errorf("TempDir() failed: %v", err)
	}
	defer run.RemoveAll(tmpDir)
	if err := nodes.RunCopy(run, []string{":/home/veyron/.ssh/id_rsa.pub"}, tmpDir); err != nil {
		return err
	}

	// Append the keys to the authorized keys on the GCE git
	// mirror.
	sshKeys := []string{}
	for _, node := range nodes {
		var sshKeyDir string
		if len(nodes) == 1 {
			sshKeyDir = tmpDir
		} else {
			sshKeyDir = filepath.Join(tmpDir, node.Name)
		}
		sshKeyFile := filepath.Join(sshKeyDir, "id_rsa.pub")
		bytes, err := run.ReadFile(sshKeyFile)
		if err != nil {
			return fmt.Errorf("ReadFile(%v) failed: %v", sshKeyFile, err)
		}
		sshKeys = append(sshKeys, string(bytes))
	}
	gitMirror, err := allNodes.MatchNames("^git-mirror$")
	if err != nil {
		return err
	}
	echoCmd := []string{"echo", strings.Join(sshKeys, "\n"), ">>", "/home/git/.ssh/authorized_keys"}
	if err := gitMirror.RunCommand(run, "git", echoCmd); err != nil {
		return err
	}

	return nil
}

func runNodeDelete(cmd *cmdline.Command, args []string) error {
	run := newRun(cmd)

	// Delete the node.
	var in bytes.Buffer
	in.WriteString("Y\n") // answers the [Y/n] prompt
	opts := run.Opts()
	opts.Stdin = &in
	deleteArgs := []string{
		"compute",
		"--project", *flagProject,
		"instances",
		"delete",
	}
	deleteArgs = append(deleteArgs, args...)
	deleteArgs = append(deleteArgs, "--zone", flagZone)
	if err := run.CommandWithOpts(opts, "gcloud", deleteArgs...); err != nil {
		return err
	}

	return nil
}
