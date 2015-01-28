// The following enables go generate to generate the doc.go file.
//go:generate go run $VANADIUM_ROOT/release/go/src/v.io/lib/cmdline/testdata/gendoc.go .
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"v.io/lib/cmdline"
	"v.io/tools/lib/util"
)

// TODO(jsimsa): Add tests by mocking out jenkins.

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

const (
	jenkinsHost = "http://veyron-jenkins:8001/jenkins"
)

var (
	// Global flags.
	flagColor   = flag.Bool("color", false, "Format output in color.")
	flagDryRun  = flag.Bool("n", false, "Show what commands will run, but do not execute them.")
	flagVerbose = flag.Bool("v", false, "Print verbose output.")
	// Command-specific flag.
	flagDescription string
)

func init() {
	cmdNodeCreate.Flags.StringVar(&flagDescription, "description", "", "Node description.")
}

func newContext(cmd *cmdline.Command) *util.Context {
	return util.NewContextFromCommand(cmd, *flagColor, *flagDryRun, *flagVerbose)
}

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
		Description:       flagDescription,
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
				map[string]string{
					"stapler-class": "hudson.slaves.EnvironmentVariablesNodeProperty$Entry",
					"key":           "TERM",
					"value":         "xterm-256color",
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
	Machines []machine `json:"computer"`
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

// runNodeCreate adds slave node(s) to Jenkins configuration.
func runNodeCreate(cmd *cmdline.Command, args []string) error {
	ctx := newContext(cmd)
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

// runNodeDelete removes slave node(s) from Jenkins configuration.
func runNodeDelete(cmd *cmdline.Command, args []string) error {
	ctx := newContext(cmd)
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
	return nil
}
