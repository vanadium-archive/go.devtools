package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"v.io/lib/cmdline"
	"v.io/tools/lib/util"
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
	jenkinsHost = "http://veyron-jenkins:8001/jenkins"
)

// createRequest represents a request to create a new machine in
// Jenkins configuration.
type createRequest struct {
	Name              string            `json:"name"`
	Description       string            `json:"nodeDescription"`
	NumExecutors      int               `json:"numExecutors"`
	RemoteFS          string            `json:"remoteFS"`
	Labels            string            `json:"labelString"`
	Mode              string            `json:"mode"`
	Type              string            `json:"type"`
	RetentionStrategy map[string]string `json:"retentionStrategy"`
	NodeProperties    nodeProperties    `json:"nodeProperties"`
	Launcher          map[string]string `json:"launcher"`
}

// nodeProperties enumerates the environment variable settings for
// Jenkins configuration.
type nodeProperties struct {
	Class       string              `json:"stapler-class"`
	Environment []map[string]string `json:"env"`
}

// addNodeToJenkins sends an HTTP request to Jenkins that prompts it
// to add a new machine to its configuration.
//
// NOTE: Jenkins REST API is not documented anywhere and the
// particular HTTP request used to add a new machine to Jenkins
// configuration has been crafted using trial and error.
func addNodeToJenkins(ctx *util.Context, node string) (*http.Response, error) {
	jenkins := ctx.Jenkins(jenkinsHost)
	request := createRequest{
		Name:              node,
		Description:       flagMachineType,
		NumExecutors:      1,
		RemoteFS:          "/home/veyron/jenkins",
		Labels:            fmt.Sprintf("%s linux-slave", node),
		Mode:              "EXCLUSIVE",
		Type:              "hudson.slaves.DumbSlave$DescriptorImpl",
		RetentionStrategy: map[string]string{"stapler-class": "hudson.slaves.RetentionStrategy$Always"},
		NodeProperties: nodeProperties{
			Class: "hudson.slaves.EnvironmentVariablesNodeProperty",
			Environment: []map[string]string{
				map[string]string{
					"stapler-class": "hudson.slaves.EnvironmentVariablesNodeProperty$Entry",
					"key":           "GOROOT",
					"value":         "$HOME/go",
				},
				map[string]string{
					"stapler-class": "hudson.slaves.EnvironmentVariablesNodeProperty$Entry",
					"key":           "PATH",
					"value":         "$HOME/go/bin:$PATH",
				},
			},
		},
		Launcher: map[string]string{
			"stapler-class": "hudson.plugins.sshslaves.SSHLauncher",
			"host":          node,
			// The following ID has been retrieved from
			// Jenkins configuration backup.
			"credentialsId": "73f76f53-8332-4259-bc08-d6f0b8521a5b",
		},
	}
	bytes, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("Marshal(%v) failed: %v", request, err)
	}
	values := url.Values{
		"name": {node},
		"type": {"hudson.slaves.DumbSlave$DescriptorImpl"},
		"json": {string(bytes)},
	}
	return jenkins.Invoke("GET", "computer/doCreateItem", values)
}

// machines stores information about Jenkins machines.
type machines struct {
	Machines []machine `json:"machine"`
}

// machine stores information about a Jenkins machine.
type machine struct {
	Name string `json:"displayName"`
	Idle bool   `json:"idle"`
}

// isNodeIdle checks if a Jenkins node is idle
func isNodeIdle(ctx *util.Context, node string) (bool, error) {
	jenkins := ctx.Jenkins(jenkinsHost)
	res, err := jenkins.Invoke("GET", "computer/api/json", url.Values{})
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	r := bufio.NewReader(res.Body)
	machines := machines{}
	if err := json.NewDecoder(r).Decode(&machines); err != nil {
		return false, fmt.Errorf("Decode() failed: %v", err)
	}
	for _, machine := range machines.Machines {
		if machine.Name == node {
			return machine.Idle, nil
		}
	}
	return false, fmt.Errorf("node %v not found", node)
}

// removeNodeFromJenkins sends an HTTP request to Jenkins that prompts
// it to remove an existing machine from its configuration.
func removeNodeFromJenkins(ctx *util.Context, node string) (*http.Response, error) {
	jenkins := ctx.Jenkins(jenkinsHost)
	return jenkins.Invoke("POST", fmt.Sprintf("computer/%s/doDelete", node), url.Values{})
}

func runNodeCreate(cmd *cmdline.Command, args []string) error {
	ctx := newContext(cmd)

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
		"--scopes", "storage-full",
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
		if err := nodes.RunCommand(ctx, "veyron", []string{"echo"}); err != nil {
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

	// Execute the jenkins setup script.
	curlCmd := []string{"curl", "-u", "vanadium:D6HT]P,LrJ7e", "https://dev.v.io/noproxy/jenkins-setup.sh", "|", "bash"}
	if err := nodes.RunCommand(ctx, "veyron", curlCmd); err != nil {
		return err
	}

	// Copy the SSH keys from the newly created nodes to the local
	// machine.
	tmpDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		return fmt.Errorf("TempDir() failed: %v", err)
	}
	defer ctx.Run().RemoveAll(tmpDir)
	if err := nodes.RunCopy(ctx, []string{":/home/veyron/.ssh/id_rsa.pub"}, tmpDir); err != nil {
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
		bytes, err := ctx.Run().ReadFile(sshKeyFile)
		if err != nil {
			return fmt.Errorf("ReadFile(%v) failed: %v", sshKeyFile, err)
		}
		sshKeys = append(sshKeys, string(bytes))
	}
	gitMirror, err := allNodes.MatchNames("git-mirror")
	if err != nil {
		return err
	}
	echoCmd := []string{"echo", strings.Join(sshKeys, "\n"), ">>", "/home/git/.ssh/authorized_keys"}
	if err := gitMirror.RunCommand(ctx, "git", echoCmd); err != nil {
		return err
	}

	// Add the slave node(s) to Jenkins configuration.
	for _, node := range args {
		response, err := addNodeToJenkins(ctx, node)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			return fmt.Errorf("HTTP request returned %d", response.StatusCode)
		}
	}
	return nil
}

func runNodeDelete(cmd *cmdline.Command, args []string) error {
	ctx := newContext(cmd)

	// Remove the slave node(s) from Jenkins configuration.
	for _, node := range args {
		// Wait for the node to become idle.
		const numRetries = 60
		const retryPeriod = time.Minute
		for i := 0; i < numRetries; i++ {
			if ok, err := isNodeIdle(ctx, node); err != nil {
				return err
			} else if ok {
				break
			}
			time.Sleep(retryPeriod)
		}
		response, err := removeNodeFromJenkins(ctx, node)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			return fmt.Errorf("HTTP request returned %d", response.StatusCode)
		}
	}

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
