// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"v.io/jiri/collect"
	"v.io/jiri/jiri"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/xunit"
)

const (
	testStressNodeName            = "stress"
	testStressNumServerNodes      = 3
	testStressNumClientNodes      = 6
	testStressNumWorkersPerClient = 8
	testStressMaxChunkCnt         = 100
	testStressMaxPayloadSize      = 10000
	testStressDuration            = 1 * time.Hour

	testLoadNodeName       = "load"
	testLoadNumServerNodes = 1
	testLoadNumClientNodes = 1
	testLoadCPUs           = testLoadNumServerNodes
	testLoadPayloadSize    = 1000
	testLoadDuration       = 15 * time.Minute

	loadStatsOutputFile = "load_stats.json"

	serverPort          = 10000
	serverMaxUpTime     = 2 * time.Hour
	waitTimeForServerUp = 1 * time.Minute

	gceProject           = "vanadium-internal"
	gceZone              = "us-central1-f"
	gceServerMachineType = "n1-highcpu-8"
	gceClientMachineType = "n1-highcpu-4"
	gceNodePrefix        = "tmpnode-rpc"

	vcloudPkg = "v.io/x/devtools/vcloud"
	serverPkg = "v.io/x/ref/runtime/internal/rpc/stress/stressd"
	clientPkg = "v.io/x/ref/runtime/internal/rpc/stress/stress"
)

var (
	binPath = filepath.Join("release", "go", "bin")
)

// vanadiumGoRPCStress runs an RPC stress test with multiple GCE instances.
func vanadiumGoRPCStress(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	return runRPCTest(jirix, testName, testStressNodeName, testStressNumServerNodes, testStressNumClientNodes, runStressTest)
}

// vanadiumGoRPCLoad runs an RPC load test with multiple GCE instances.
func vanadiumGoRPCLoad(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	return runRPCTest(jirix, testName, testLoadNodeName, testLoadNumServerNodes, testLoadNumClientNodes, runLoadTest)
}

func runRPCTest(jirix *jiri.X, testName, nodeName string, numServerNodes, numClientNodes int, testFunc func(*jiri.X, string) (*test.Result, error)) (_ *test.Result, e error) {
	cleanup, err := initTest(jirix, testName, []string{"base"})
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Install binaries.
	if err := jirix.Run().Command("jiri", "go", "install", vcloudPkg, serverPkg, clientPkg); err != nil {
		return nil, internalTestError{err, "Install Binaries"}
	}

	// Cleanup old nodes if any.
	fmt.Fprint(jirix.Stdout(), "Deleting old nodes...\n")
	if err := deleteNodes(jirix, nodeName, numServerNodes, numClientNodes); err != nil {
		fmt.Fprintf(jirix.Stdout(), "IGNORED: %v\n", err)
	}

	// Create nodes.
	fmt.Fprint(jirix.Stdout(), "Creating nodes...\n")
	if err := createNodes(jirix, nodeName, numServerNodes, numClientNodes); err != nil {
		return nil, internalTestError{err, "Create Nodes"}
	}

	// Start servers.
	fmt.Fprint(jirix.Stdout(), "Starting servers...\n")
	serverDone, err := startServers(jirix, nodeName, numServerNodes)
	if err != nil {
		return nil, internalTestError{err, "Start Servers"}
	}

	// Run the test.
	fmt.Fprint(jirix.Stdout(), "Running test...\n")
	result, err := testFunc(jirix, testName)
	if err != nil {
		return nil, internalTestError{err, "Run Test"}
	}

	// Stop servers.
	fmt.Fprint(jirix.Stdout(), "Stopping servers...\n")
	if err := stopServers(jirix, nodeName, numServerNodes); err != nil {
		return nil, internalTestError{err, "Stop Servers"}
	}
	if err := <-serverDone; err != nil {
		return nil, internalTestError{err, "Stop Servers"}
	}

	// Delete nodes.
	fmt.Fprint(jirix.Stdout(), "Deleting nodes...\n")
	if err := deleteNodes(jirix, nodeName, numServerNodes, numClientNodes); err != nil {
		return nil, internalTestError{err, "Delete Nodes"}
	}
	return result, nil
}

func serverNodeName(nodeName string, n int) string {
	return fmt.Sprintf("%s-%s-server-%02d", gceNodePrefix, nodeName, n)
}

func clientNodeName(nodeName string, n int) string {
	return fmt.Sprintf("%s-%s-client-%02d", gceNodePrefix, nodeName, n)
}

func createNodes(jirix *jiri.X, nodeName string, numServerNodes, numClientNodes int) error {
	cmd := filepath.Join(jirix.Root, binPath, "vcloud")
	args := []string{
		"node", "create",
		"-project", gceProject,
		"-zone", gceZone,
	}
	serverArgs := append(args, "-machine-type", gceServerMachineType)
	for n := 0; n < numServerNodes; n++ {
		serverArgs = append(serverArgs, serverNodeName(nodeName, n))
	}
	if err := jirix.Run().Command(cmd, serverArgs...); err != nil {
		return err
	}
	clientArgs := append(args, "-machine-type", gceClientMachineType)
	for n := 0; n < numClientNodes; n++ {
		clientArgs = append(clientArgs, clientNodeName(nodeName, n))
	}
	return jirix.Run().Command(cmd, clientArgs...)
}

func deleteNodes(jirix *jiri.X, nodeName string, numServerNodes, numClientNodes int) error {
	cmd := filepath.Join(jirix.Root, binPath, "vcloud")
	args := []string{
		"node", "delete",
		"-project", gceProject,
		"-zone", gceZone,
	}
	for n := 0; n < numServerNodes; n++ {
		args = append(args, serverNodeName(nodeName, n))
	}
	for n := 0; n < numClientNodes; n++ {
		args = append(args, clientNodeName(nodeName, n))
	}
	return jirix.Run().Command(cmd, args...)
}

func startServers(jirix *jiri.X, nodeName string, numServerNodes int) (<-chan error, error) {
	var servers []string
	for n := 0; n < numServerNodes; n++ {
		servers = append(servers, serverNodeName(nodeName, n))
	}
	cmd := filepath.Join(jirix.Root, binPath, "vcloud")
	args := []string{
		"run",
		"-failfast",
		"-project", gceProject,
		strings.Join(servers, ","),
		filepath.Join(jirix.Root, binPath, "stressd"),
		"++",
		"./stressd",
		"-v23.tcp.address", fmt.Sprintf(":%d", serverPort),
		"-duration", serverMaxUpTime.String(),
	}

	done := make(chan error)
	go func() {
		done <- jirix.Run().Command(cmd, args...)
	}()

	// Wait until for a few minute while servers are brought up.
	timeout := time.After(waitTimeForServerUp)
	select {
	case err := <-done:
		if err != nil {
			return nil, err
		}
		close(done)
	case <-timeout:
	}
	return done, nil
}

func stopServers(jirix *jiri.X, nodeName string, numServerNodes int) error {
	cmd := filepath.Join(jirix.Root, binPath, "vcloud")
	args := []string{
		"run",
		"-failfast",
		"-project", gceProject,
		clientNodeName(nodeName, 0),
		filepath.Join(jirix.Root, binPath, "stress"),
		"++",
		"./stress", "stop",
	}
	for n := 0; n < numServerNodes; n++ {
		args = append(args, fmt.Sprintf("/%s:%d", serverNodeName(nodeName, n), serverPort))
	}
	return jirix.Run().Command(cmd, args...)
}

func runStressTest(jirix *jiri.X, testName string) (*test.Result, error) {
	var servers, clients []string
	for n := 0; n < testStressNumServerNodes; n++ {
		servers = append(servers, fmt.Sprintf("/%s:%d", serverNodeName(testStressNodeName, n), serverPort))
	}
	for n := 0; n < testStressNumClientNodes; n++ {
		clients = append(clients, clientNodeName(testStressNodeName, n))
	}

	var out bytes.Buffer
	opts := jirix.Run().Opts()
	opts.Stdout = io.MultiWriter(opts.Stdout, &out)
	opts.Stderr = io.MultiWriter(opts.Stderr, &out)
	cmd := filepath.Join(jirix.Root, binPath, "vcloud")
	args := []string{
		"run",
		"-failfast",
		"-project", gceProject,
		strings.Join(clients, ","),
		filepath.Join(jirix.Root, binPath, "stress"),
		"++",
		"./stress", "stress",
		"-workers", strconv.Itoa(testStressNumWorkersPerClient),
		"-max-chunk-count", strconv.Itoa(testStressMaxChunkCnt),
		"-max-payload-size", strconv.Itoa(testStressMaxPayloadSize),
		"-duration", testStressDuration.String(),
		"-format", "json",
	}
	args = append(args, servers...)
	if err := jirix.Run().CommandWithOpts(opts, cmd, args...); err != nil {
		return nil, err
	}

	// Get the stats from the servers and stop them.
	args = []string{
		"run",
		"-failfast",
		"-project", gceProject,
		clients[0],
		filepath.Join(jirix.Root, binPath, "stress"),
		"++",
		"./stress", "stats",
		"-format", "json",
	}
	args = append(args, servers...)
	if err := jirix.Run().CommandWithOpts(opts, cmd, args...); err != nil {
		return nil, err
	}

	// Read the stats.
	cStats, sStats, err := readStressStats(out.String())
	if err != nil {
		if err := xunit.CreateFailureReport(jirix.Context, testName, "StressTest", "ReadStats", "Failure", err.Error()); err != nil {
			return nil, err
		}
		return &test.Result{Status: test.Failed}, nil
	}
	fmt.Fprint(jirix.Stdout(), "\nRESULT:\n")
	writeStressStats(jirix.Stdout(), "Client Stats:", cStats)
	writeStressStats(jirix.Stdout(), "Server Stats:", sStats)
	fmt.Fprint(jirix.Stdout(), "\n")

	// Verify the stats.
	sStats.BytesRecv, sStats.BytesSent = sStats.BytesSent, sStats.BytesRecv
	if !reflect.DeepEqual(cStats, sStats) {
		output := fmt.Sprintf("%+v != %+v", cStats, sStats)
		if err := xunit.CreateFailureReport(jirix.Context, testName, "StressTest", "VerifyStats", "Mismatched", output); err != nil {
			return nil, err
		}
		return &test.Result{Status: test.Failed}, nil
	}
	return &test.Result{Status: test.Passed}, nil
}

type stressStats struct {
	SumCount       uint64
	SumStreamCount uint64
	BytesRecv      uint64
	BytesSent      uint64
}

func readStressStats(out string) (*stressStats, *stressStats, error) {
	re := regexp.MustCompile(`client stats:({.*})`)
	cStats, err := readStressStatsHelper(re, out, testStressNumClientNodes)
	if err != nil {
		return nil, nil, err
	}
	re = regexp.MustCompile(`server stats\(.*\):({.*})`)
	sStats, err := readStressStatsHelper(re, out, testStressNumServerNodes)
	if err != nil {
		return nil, nil, err
	}
	return cStats, sStats, nil
}

func readStressStatsHelper(re *regexp.Regexp, out string, numStats int) (*stressStats, error) {
	matches := re.FindAllSubmatch([]byte(out), -1)
	if len(matches) != numStats {
		return nil, fmt.Errorf("invalid number of stats: %d != %qd", len(matches), numStats)
	}
	var merged stressStats
	for _, match := range matches {
		if len(match) != 2 {
			return nil, fmt.Errorf("invalid stats: %q", match)
		}
		var stats stressStats
		if err := json.Unmarshal(match[1], &stats); err != nil {
			return nil, fmt.Errorf("invalid stats: %q", match)
		}
		if stats.SumCount == 0 || stats.SumStreamCount == 0 {
			// Although clients choose servers and RPC methods randomly, we report
			// this as a failure since it is very unlikely.
			return nil, fmt.Errorf("zero count: %q", match)
		}
		merged.SumCount += stats.SumCount
		merged.SumStreamCount += stats.SumStreamCount
		merged.BytesRecv += stats.BytesRecv
		merged.BytesSent += stats.BytesSent
	}
	return &merged, nil
}

func writeStressStats(w io.Writer, title string, stats *stressStats) {
	fmt.Fprintf(w, "%s\n", title)
	fmt.Fprintf(w, "\tNumber of non-streaming RPCs:\t%d\n", stats.SumCount)
	fmt.Fprintf(w, "\tNumber of streaming RPCs:\t%d\n", stats.SumStreamCount)
	fmt.Fprintf(w, "\tNumber of bytes received:\t%d\n", stats.BytesRecv)
	fmt.Fprintf(w, "\tNumber of bytes sent:\t\t%d\n", stats.BytesSent)
}

func runLoadTest(jirix *jiri.X, testName string) (*test.Result, error) {
	var servers, clients []string
	for n := 0; n < testLoadNumServerNodes; n++ {
		servers = append(servers, fmt.Sprintf("/%s:%d", serverNodeName(testLoadNodeName, n), serverPort))
	}
	for n := 0; n < testLoadNumClientNodes; n++ {
		clients = append(clients, clientNodeName(testLoadNodeName, n))
	}

	var out bytes.Buffer
	opts := jirix.Run().Opts()
	opts.Stdout = io.MultiWriter(opts.Stdout, &out)
	opts.Stderr = io.MultiWriter(opts.Stderr, &out)
	cmd := filepath.Join(jirix.Root, binPath, "vcloud")
	args := []string{
		"run",
		"-failfast",
		"-project", gceProject,
		strings.Join(clients, ","),
		filepath.Join(jirix.Root, binPath, "stress"),
		"++",
		"./stress", "load",
		"-cpu", strconv.Itoa(testLoadCPUs),
		"-payload-size", strconv.Itoa(testLoadPayloadSize),
		"-duration", testLoadDuration.String(),
		"-format", "json",
	}
	args = append(args, servers...)
	if err := jirix.Run().CommandWithOpts(opts, cmd, args...); err != nil {
		return nil, err
	}

	// Read the stats.
	stats, err := readLoadStats(out.String(), testLoadNumClientNodes)
	if err != nil {
		if err := xunit.CreateFailureReport(jirix.Context, testName, "LoadTest", "ReadStats", "Failure", err.Error()); err != nil {
			return nil, err
		}
		return &test.Result{Status: test.Failed}, nil
	}

	fmt.Fprint(jirix.Stdout(), "\nRESULT:\n")
	fmt.Fprint(jirix.Stdout(), "Load Stats\n")
	fmt.Fprintf(jirix.Stdout(), "\tNumber of RPCs:\t\t%.2f\n", stats.Iterations)
	fmt.Fprintf(jirix.Stdout(), "\tLatency (msec/rpc):\t%.2f\n", stats.MsecPerRpc)
	fmt.Fprintf(jirix.Stdout(), "\tQPS:\t\t\t%.2f\n", stats.Qps)
	fmt.Fprintf(jirix.Stdout(), "\tQPS/core:\t\t%.2f\n", stats.QpsPerCore)
	fmt.Fprint(jirix.Stdout(), "\n")

	// Write the test stats in json format for vmon.
	filename := filepath.Join(os.Getenv("WORKSPACE"), loadStatsOutputFile)
	if err := writeLoadStatsJSON(filename, stats); err != nil {
		if err := xunit.CreateFailureReport(jirix.Context, testName, "LoadTest", "WriteLoadStats", "Failure", err.Error()); err != nil {
			return nil, err
		}
		return &test.Result{Status: test.Failed}, nil
	}
	fmt.Fprintf(jirix.Stdout(), "Wrote load stats to %q\n", filename)
	return &test.Result{Status: test.Passed}, nil
}

type loadStats struct {
	Iterations float64
	MsecPerRpc float64
	Qps        float64
	QpsPerCore float64
}

func readLoadStats(out string, numStats int) (*loadStats, error) {
	re := regexp.MustCompile(`load stats:({.*})`)
	matches := re.FindAllSubmatch([]byte(out), -1)
	if len(matches) != numStats {
		return nil, fmt.Errorf("invalid number of stats: %d != %d", len(matches), numStats)
	}
	var merged loadStats
	for _, match := range matches {
		if len(match) != 2 {
			return nil, fmt.Errorf("invalid stats: %q", match)
		}
		var stats loadStats
		if err := json.Unmarshal(match[1], &stats); err != nil {
			return nil, fmt.Errorf("invalid stats: %q", match)
		}
		if stats.Iterations == 0 {
			return nil, fmt.Errorf("zero count: %q", match)
		}
		merged.Iterations += stats.Iterations
		merged.MsecPerRpc += stats.MsecPerRpc
		merged.Qps += stats.Qps
		merged.QpsPerCore += stats.QpsPerCore
	}
	merged.Iterations /= float64(numStats)
	merged.MsecPerRpc /= float64(numStats)
	merged.Qps /= float64(numStats)
	merged.QpsPerCore /= float64(numStats)
	return &merged, nil
}

func writeLoadStatsJSON(filename string, stats *loadStats) error {
	b, err := json.Marshal(stats)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filename, b, 0644)
}
