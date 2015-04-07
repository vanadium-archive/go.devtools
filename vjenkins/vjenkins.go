// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $V23_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go .
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"v.io/x/devtools/internal/tool"
	"v.io/x/lib/cmdline"
)

// TODO(jsimsa): Add tests by mocking out jenkins.
//
// TODO(jsimsa): Create a tools/lib/gcutil package that encapsulates
// the interaction with GCE and use it here and in the vcloud tool.
func main() {
	os.Exit(cmdVJenkins.Main())
}

var cmdVJenkins = &cmdline.Command{
	Name:     "vjenkins",
	Short:    "Vanadium command-line utility for interacting with Jenkins",
	Long:     "Vanadium command-line utility for interacting with Jenkins.",
	Children: []*cmdline.Command{cmdNode},
}

var cmdNode = &cmdline.Command{
	Name:     "node",
	Short:    "Manage Jenkins slave nodes",
	Long:     "Manage Jenkins slave nodes.",
	Children: []*cmdline.Command{cmdNodeCreate, cmdNodeDelete},
}

var cmdNodeCreate = &cmdline.Command{
	Run:   runNodeCreate,
	Name:  "create",
	Short: "Create Jenkins slave nodes",
	Long: `
Create Jenkins nodes. Uses the Jenkins REST API to create new slave nodes.
`,
	ArgsName: "<names>",
	ArgsLong: "<names> is a list of names identifying nodes to be created.",
}

var cmdNodeDelete = &cmdline.Command{
	Run:   runNodeDelete,
	Name:  "delete",
	Short: "Delete Jenkins slave nodes",
	Long: `
Delete Jenkins nodes. Uses the Jenkins REST API to delete existing slave nodes.
`,
	ArgsName: "<names>",
	ArgsLong: "<names> is a list of names identifying nodes to be deleted.",
}

var (
	// Global flags.
	flagColor   = flag.Bool("color", false, "Format output in color.")
	flagDryRun  = flag.Bool("n", false, "Show what commands will run, but do not execute them.")
	flagVerbose = flag.Bool("v", false, "Print verbose output.")
	// Command-specific flags.
	flagCredentialsId string
	flagDescription   string
	flagJenkinsHost   string
	flagProject       string
	flagZone          string

	ipAddressRE = regexp.MustCompile(`^(\S*)\s*(\S*)\s(\S*)\s(\S*)\s(\S*)\s(\S*)$`)
)

func init() {
	cmdVJenkins.Flags.StringVar(&flagJenkinsHost, "jenkins", "http://localhost:8080/jenkins", "The host of the Jenkins master.")
	cmdNodeCreate.Flags.StringVar(&flagCredentialsId, "credentials_id", "73f76f53-8332-4259-bc08-d6f0b8521a5b", "The credentials ID used to connect the master to the node.")
	cmdNodeCreate.Flags.StringVar(&flagDescription, "description", "", "Node description.")
	cmdNodeCreate.Flags.StringVar(&flagZone, "zone", "us-central1-f", "GCE zone of the machine.")
	cmdNodeCreate.Flags.StringVar(&flagProject, "project", "vanadium-internal", "GCE project of the machine.")
}

func newContext(cmd *cmdline.Command) *tool.Context {
	return tool.NewContextFromCommand(cmd, tool.ContextOpts{
		Color:   flagColor,
		DryRun:  flagDryRun,
		Verbose: flagVerbose,
	})
}

// lookupIPAddress looks up the IP address for the given GCE node.
func lookupIPAddress(ctx *tool.Context, node string) (string, error) {
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	if err := ctx.Run().CommandWithOpts(opts, "gcloud", "compute", "instances",
		"--project", flagProject,
		"list", "--zones", flagZone, "-r", node); err != nil {
		return "", err
	}
	// The expected output is two lines, the first one is a header and
	// the second one is a node description.
	output := strings.TrimSpace(out.String())
	lines := strings.Split(output, "\n")
	if got, want := len(lines), 2; got != want {
		return "", fmt.Errorf("unexpected length of %v: got %v, want %v", lines, got, want)
	}
	// Parse the node information.
	matches := ipAddressRE.FindStringSubmatch(lines[1])
	if got, want := len(matches), 7; got != want {
		return "", fmt.Errorf("unexpected length of %v: got %v, want %v", matches, got, want)
	}
	// The external IP address is the fifth column.
	return matches[5], nil
}

// runNodeCreate adds slave node(s) to Jenkins configuration.
func runNodeCreate(cmd *cmdline.Command, args []string) error {
	ctx := newContext(cmd)
	jenkins, err := ctx.Jenkins(flagJenkinsHost)
	if err != nil {
		return err
	}

	for _, name := range args {
		ipAddress, err := lookupIPAddress(ctx, name)
		if err != nil {
			return err
		}
		fmt.Println(ipAddress)
		if err := jenkins.AddNodeToJenkins(name, ipAddress, flagDescription, flagCredentialsId); err != nil {
			return err
		}
	}
	return nil
}

// runNodeDelete removes slave node(s) from Jenkins configuration.
func runNodeDelete(cmd *cmdline.Command, args []string) error {
	ctx := newContext(cmd)
	jenkins, err := ctx.Jenkins(flagJenkinsHost)
	if err != nil {
		return err
	}

	for _, node := range args {
		// Wait for the node to become idle.
		const numRetries = 60
		const retryPeriod = time.Minute
		for i := 0; i < numRetries; i++ {
			if ok, err := jenkins.IsNodeIdle(node); err != nil {
				return err
			} else if ok {
				break
			}
			time.Sleep(retryPeriod)
		}
		err := jenkins.RemoveNodeFromJenkins(node)
		if err != nil {
			return err
		}
	}
	return nil
}
