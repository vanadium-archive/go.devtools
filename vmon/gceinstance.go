// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/cloudmonitoring/v2beta2"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/monitoring"
	"v.io/x/devtools/internal/testutil"
	"v.io/x/devtools/internal/tool"
)

const localCheckScript = `#!/bin/bash

# Check cpu.
CPU_IDLE="$(top -bn2 | grep Cpu | tail -1 | sed -n 's/.*,\s*\(\S*\) id,.*/\1/p')"
CPU_USAGE="$(echo "scale=4; (100.0-${CPU_IDLE})/100.0" | bc)"
echo ${CPU_USAGE} > output_cpu_$(hostname)

# Check memory.
MEM="$(free -m)"
MEM_TOTAL="$(echo "${MEM}" | grep Mem: | awk '{print $2}')"
MEM_FREE="$(echo "${MEM}" | grep /cache: | awk '{print $4}')"
MEM_USAGE="$(echo "scale=4; (${MEM_TOTAL}-${MEM_FREE})/${MEM_TOTAL}" | bc)"
echo ${MEM_USAGE} > output_mem_$(hostname)

# Check disk.
DISK_PCT="$(df | grep /dev/sda1 | awk '{print $5}')"
DISK_USAGE="$(echo "scale=4; ${DISK_PCT::-1}/100" | bc)"
echo ${DISK_USAGE} > output_disk_$(hostname)

# Check open TCP connections.
sudo netstat -anp --tcp | egrep "ESTABLISHED|CLOSE_WAIT|FIN_WAIT1|FIN_WAIT2" | wc -l > output_tcpconn_$(hostname)
`

var (
	pingResultRE = regexp.MustCompile(`(\S*)\s*:.*min/avg/max = [^/]*/([^/]*)/[^/]*`)
	metricNames  = []string{"gce-instance-cpu", "gce-instance-memory", "gce-instance-disk", "gce-instance-ping", "gce-instance-tcpconn"}
)

type gceInstanceData struct {
	name string
	zone string
	ip   string
	stat *gceInstanceStat
}

type gceInstanceStat struct {
	cpuUsage    float64
	memUsage    float64
	diskUsage   float64
	pingLatency float64
	tcpconn     float64
}

// checkGCEInstances checks all GCE instances in a GCE project.
func checkGCEInstances(ctx *tool.Context) error {
	msg := "Getting instance list\n"
	instances, err := getInstances(ctx)
	if err != nil {
		testutil.Fail(ctx, msg)
		return err
	}
	testutil.Pass(ctx, msg)

	if err := invoker(ctx, "Check ping latencies\n", instances, checkPing); err != nil {
		return err
	}

	if err := invoker(ctx, "Check machine stats\n", instances, checkInstanceStats); err != nil {
		return err
	}

	if err := invoker(ctx, "Send data to GCM\n", instances, sendToGCM); err != nil {
		return err
	}

	return nil
}

func invoker(ctx *tool.Context, msg string, instances []*gceInstanceData, fn func(*tool.Context, []*gceInstanceData) error) error {
	if err := fn(ctx, instances); err != nil {
		testutil.Fail(ctx, msg)
		return err
	}
	testutil.Pass(ctx, msg)
	return nil
}

// getInstances uses "gcloud compute instances list" to get a list of instances
// we care about.
func getInstances(ctx *tool.Context) ([]*gceInstanceData, error) {
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	if err := ctx.Run().CommandWithOpts(opts,
		"gcloud", "-q", "--project="+projectFlag, "compute", "instances", "list", "--format=json"); err != nil {
		return nil, err
	}
	var instances []struct {
		Name              string
		Zone              string
		NetworkInterfaces []struct {
			AccessConfigs []struct {
				NatIP string
			}
		}
	}
	if err := json.Unmarshal(out.Bytes(), &instances); err != nil {
		return nil, fmt.Errorf("Unmarshal() failed: %v", err)
	}
	filteredInstances := []*gceInstanceData{}
	for _, instance := range instances {
		name := instance.Name
		// We only collect data for the machines that are running vanadium services
		// and nginx servers.
		// New patterns should be added if new machine name patterns are introduced.
		if strings.HasPrefix(name, "vanadium") || strings.HasPrefix(name, "nginx") {
			filteredInstances = append(filteredInstances, &gceInstanceData{
				name: name,
				zone: instance.Zone,
				ip:   instance.NetworkInterfaces[0].AccessConfigs[0].NatIP,
				stat: &gceInstanceStat{
					cpuUsage:    -1,
					memUsage:    -1,
					diskUsage:   -1,
					pingLatency: -1,
					tcpconn:     -1,
				},
			})
		}
	}
	return filteredInstances, nil
}

// checkPing checks the ping response from all instances using fping.
//
// By default, fping is installed on debian/ubuntu linux machines in GCE. For
// Macs, fping needs to be installed through homebrew.
func checkPing(ctx *tool.Context, instances []*gceInstanceData) error {
	// Check the fping program.
	if _, err := exec.LookPath("fping"); err != nil {
		return fmt.Errorf("fping not installed.")
	}

	// Run fping.
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	args := []string{
		"-q",
		"-c3", // ping 3 times for each host.
	}
	ipToInstance := map[string]*gceInstanceData{}
	for _, instance := range instances {
		args = append(args, instance.ip)
		ipToInstance[instance.ip] = instance
	}
	if err := ctx.Run().CommandWithOpts(opts, "fping", args...); err != nil {
		// When some hosts are not reachable, the command's exit code will be non-zero.
		fmt.Fprintf(ctx.Stdout(), "Output:\n%s\n", out.String())
	}

	// Parse output.
	for _, line := range strings.Split(out.String(), "\n") {
		matches := pingResultRE.FindStringSubmatch(line)
		if matches != nil {
			ip := matches[1]
			strLatency := matches[2]
			latency, err := strconv.ParseFloat(strLatency, 64)
			if err != nil {
				return fmt.Errorf("ParseFloat(%q) failed: %v", strLatency, err)
			}
			ipToInstance[ip].stat.pingLatency = latency
		}
	}
	return nil
}

// checkInstanceStats uses "vcloud run" command to run a script in remote
// instances and collects and processes results.
func checkInstanceStats(ctx *tool.Context, instances []*gceInstanceData) (e error) {
	// Create the check script in a tmp dir.
	tmpdir, err := ctx.Run().TempDir("", "")
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpdir) }, &e)
	scriptPath := filepath.Join(tmpdir, "localtest.sh")
	if err := ctx.Run().WriteFile(scriptPath, []byte(localCheckScript), os.FileMode(0755)); err != nil {
		return err
	}

	// Run "vcloud run" on all nodes.
	// This will run the local check script remotely and copy the "output" files back to
	// <tmpdir>/<node_name>/<tmpdir2>/output_<checktype>_<nodename>.
	nodes := []string{}
	instanceByNode := map[string]*gceInstanceData{}
	for _, instance := range instances {
		nodes = append(nodes, instance.name)
		instanceByNode[instance.name] = instance
	}
	vcloud := filepath.Join(binDirFlag, "vcloud")
	opts := ctx.Run().Opts()
	opts.Stdout = ioutil.Discard
	args := []string{
		"--project=" + projectFlag,
		"run",
		"--outdir=" + tmpdir,
		strings.Join(nodes, "|"),
		scriptPath,
	}
	if err := ctx.Run().CommandWithOpts(opts, vcloud, args...); err != nil {
		return err
	}

	// Find and read output files.
	if err := filepath.Walk(tmpdir, func(path string, info os.FileInfo, err error) error {
		fileName := info.Name()
		if strings.HasPrefix(fileName, "output_") {
			// The filename is in the form of: output_<checktype>_<nodename>.
			parts := strings.SplitN(fileName, "_", 3)
			checkType := parts[1]
			node := parts[2]
			value, err := readFloatFromFile(ctx, path)
			if err != nil {
				return err
			}
			switch checkType {
			case "cpu":
				instanceByNode[node].stat.cpuUsage = value
			case "mem":
				instanceByNode[node].stat.memUsage = value
			case "disk":
				instanceByNode[node].stat.diskUsage = value
			case "tcpconn":
				instanceByNode[node].stat.tcpconn = value
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("Walk() failed: %v", err)
	}
	return nil
}

// readFloatFromFile reads the given file's content as a number and converts it
// to float64.
func readFloatFromFile(ctx *tool.Context, path string) (float64, error) {
	bytes, err := ctx.Run().ReadFile(path)
	if err != nil {
		return -1, err
	}
	strValue := strings.TrimSpace(string(bytes))
	value, err := strconv.ParseFloat(strValue, 64)
	if err != nil {
		return -1, fmt.Errorf("ParseFloat(%s) failed: %v", strValue, err)
	}
	return value, nil
}

// sendToGCM sends instance stats data to GCM.
func sendToGCM(ctx *tool.Context, instances []*gceInstanceData) error {
	s, err := monitoring.Authenticate(serviceAccountFlag, keyFileFlag)
	if err != nil {
		return err
	}
	timeStr := time.Now().Format(time.RFC3339)
	for _, instance := range instances {
		msg := fmt.Sprintf("Send data for %s (%s)\n", instance.name, instance.zone)
		for _, metricName := range metricNames {
			value := -1.0
			switch metricName {
			case "gce-instance-cpu":
				value = instance.stat.cpuUsage
			case "gce-instance-memory":
				value = instance.stat.memUsage
			case "gce-instance-disk":
				value = instance.stat.diskUsage
			case "gce-instance-ping":
				value = instance.stat.pingLatency
			case "gce-instance-tcpconn":
				value = instance.stat.tcpconn
			default:
				testutil.Fail(ctx, msg)
				return fmt.Errorf("Invalid metric name: %q", metricName)
			}
			// GCM treats 0 and missing value the same.
			if value == 0 {
				continue
			}
			if err := sendInstanceDataToGCM(s, metricName, timeStr, instance, value); err != nil {
				testutil.Fail(ctx, msg)
				return fmt.Errorf("failed to add %q to GCM: %v\n", metricName, err)
			}
		}
		testutil.Pass(ctx, msg)
	}
	return nil
}

// sendInstanceDataToGCM sends a single instance's stat to GCM.
func sendInstanceDataToGCM(s *cloudmonitoring.Service, metricName, timeStr string, instance *gceInstanceData, value float64) error {
	pt := cloudmonitoring.Point{
		DoubleValue: value,
		Start:       timeStr,
		End:         timeStr,
	}
	if metricName == "gce-instance-tcpconn" {
		pt = cloudmonitoring.Point{
			Int64Value: int64(value),
			Start:      timeStr,
			End:        timeStr,
		}
	}
	md := monitoring.CustomMetricDescriptors[metricName]
	_, err := s.Timeseries.Write(projectFlag, &cloudmonitoring.WriteTimeseriesRequest{
		Timeseries: []*cloudmonitoring.TimeseriesPoint{
			&cloudmonitoring.TimeseriesPoint{
				Point: &pt,
				TimeseriesDesc: &cloudmonitoring.TimeseriesDescriptor{
					Metric: md.Name,
					Labels: map[string]string{
						md.Labels[0].Key: instance.name,
						md.Labels[1].Key: instance.zone,
					},
				},
			},
		},
	}).Do()
	if err != nil {
		return fmt.Errorf("failed to write timeseries: %v", err)
	}
	return nil
}
