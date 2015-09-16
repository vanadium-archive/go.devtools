// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/cloudmonitoring/v2beta2"

	"v.io/jiri/collect"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/monitoring"
	"v.io/x/devtools/internal/test"
)

const localCheckScript = `#!/bin/bash

# Check cpu.
CPU_IDLE="$(top -bn2 | grep Cpu | tail -1 | sed -n 's/.*,\s*\(\S*\) id,.*/\1/p')"
CPU_USAGE="$(echo "scale=4; 100.0-${CPU_IDLE}" | bc)"
echo ${CPU_USAGE} > output_cpu_$(hostname)

# Check memory.
MEM="$(free -m)"
MEM_TOTAL="$(echo "${MEM}" | grep Mem: | awk '{print $2}')"
MEM_FREE="$(echo "${MEM}" | grep /cache: | awk '{print $4}')"
MEM_USAGE="$(echo "scale=4; (${MEM_TOTAL}-${MEM_FREE})/${MEM_TOTAL}*100.0" | bc)"
echo ${MEM_USAGE} > output_mem_$(hostname)

# Check disk.
DISK_PCT="$(df | grep /dev/sda1 | awk '{print $5}')"
DISK_USAGE="$(echo "scale=4; ${DISK_PCT::-1}" | bc)"
echo ${DISK_USAGE} > output_disk_$(hostname)

# Check open TCP connections.
sudo netstat -anp --tcp | egrep "ESTABLISHED|CLOSE_WAIT|FIN_WAIT1|FIN_WAIT2" | wc -l > output_tcpconn_$(hostname)

# Check nginx
if [[ "$(hostname)" == nginx* ]]; then
  # Output is in the form of:
  #
  # Active connections: 12
  # server accepts handled requests
  #  64028 64028 65047
  # Reading: 3 Writing: 4 Waiting: 5
  NGINX_STATS="$(curl -s local-stackdriver-agent.stackdriver.com/nginx_status)"

  # Calculate qps.
  CUR_TIME_SECONDS="$(date +%s)"
  CUR_TOTAL_REQS="$(echo "${NGINX_STATS}" | grep '^ ' | awk '{print $3}')"
  LAST_REQS_INFO_FILE="/tmp/v-last-requests-info"
  QPS=0
  if [ -f "${LAST_REQS_INFO_FILE=}" ]; then
    LAST_TIME_SECONDS="$(cat "${LAST_REQS_INFO_FILE}" | awk '{print $1}')"
    LAST_TOTAL_REQS="$(cat "${LAST_REQS_INFO_FILE}" | awk '{print $2}')"
    QPS="$(echo "scale=2; (${CUR_TOTAL_REQS}-${LAST_TOTAL_REQS})/(${CUR_TIME_SECONDS}-${LAST_TIME_SECONDS})" | bc)"
  fi
  echo "${CUR_TIME_SECONDS} ${CUR_TOTAL_REQS}" > "${LAST_REQS_INFO_FILE}"
  echo ${QPS} > output_nginx-qps_$(hostname)

  # Other stats.
  echo "${NGINX_STATS}" | sed -n 's/^Active connections:\s*\(\d*\)/\1/p' > output_nginx-activeconn_$(hostname)
  echo "${NGINX_STATS}" | grep Reading | awk '{print $2}' > output_nginx-readingconn_$(hostname)
  echo "${NGINX_STATS}" | grep Reading | awk '{print $4}' > output_nginx-writingconn_$(hostname)
  echo "${NGINX_STATS}" | grep Reading | awk '{print $6}' > output_nginx-waitingconn_$(hostname)
fi
`

var (
	pingResultRE     = regexp.MustCompile(`(\S*)\s*:.*min/avg/max = [^/]*/([^/]*)/[^/]*`)
	gceMetricNames   = []string{"cpu-usage", "memory-usage", "disk-usage", "ping", "tcpconn"}
	nginxMetricNames = []string{"qps", "active-connections", "reading-connections", "writing-connections", "waiting-connections"}
)

type gceInstanceData struct {
	name      string
	zone      string
	ip        string
	stat      *gceInstanceStat
	nginxStat *nginxStat
}

type gceInstanceStat struct {
	cpuUsage    float64
	memUsage    float64
	diskUsage   float64
	pingLatency float64
	tcpconn     float64
}

type nginxStat struct {
	healthCheckLatency float64
	qps                float64
	activeConnections  float64
	readingConnections float64
	writingConnections float64
	waitingConnections float64
}

// checkGCEInstances checks all GCE instances in a GCE project.
func checkGCEInstances(ctx *tool.Context) error {
	msg := "Getting instance list\n"
	instances, err := getInstances(ctx)
	if err != nil {
		test.Fail(ctx, msg)
		return err
	}
	test.Pass(ctx, msg)

	if err := invoker(ctx, "Check ping latencies\n", instances, checkPing); err != nil {
		return err
	}

	if err := invoker(ctx, "Check machine stats\n", instances, checkInstanceStats); err != nil {
		return err
	}

	if err := invoker(ctx, "Check nginx health\n", instances, checkNginxHealth); err != nil {
		return err
	}

	if err := invoker(ctx, "Send data to GCM\n", instances, sendToGCM); err != nil {
		return err
	}

	return nil
}

func invoker(ctx *tool.Context, msg string, instances []*gceInstanceData, fn func(*tool.Context, []*gceInstanceData) error) error {
	if err := fn(ctx, instances); err != nil {
		test.Fail(ctx, msg)
		return err
	}
	test.Pass(ctx, msg)
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
		Status            string
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
		if instance.Status != "RUNNING" {
			continue
		}
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
				nginxStat: &nginxStat{
					healthCheckLatency: -1,
					qps:                -1,
					activeConnections:  -1,
					readingConnections: -1,
					writingConnections: -1,
					waitingConnections: -1,
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
			case "nginx-qps":
				instanceByNode[node].nginxStat.qps = value
			case "nginx-activeconn":
				instanceByNode[node].nginxStat.activeConnections = value
			case "nginx-readingconn":
				instanceByNode[node].nginxStat.readingConnections = value
			case "nginx-writingconn":
				instanceByNode[node].nginxStat.writingConnections = value
			case "nginx-waitingconn":
				instanceByNode[node].nginxStat.waitingConnections = value
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
		if strValue == "" {
			value = 0
		} else {
			return -1, fmt.Errorf("ParseFloat(%s) failed:\nfile: %s\nerr: %v", strValue, path, err)
		}
	}
	return value, nil
}

func checkNginxHealth(ctx *tool.Context, instances []*gceInstanceData) error {
	timeout := time.Duration(5 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}
	hasError := false
	for _, instance := range instances {
		if !strings.HasPrefix(instance.name, "nginx-worker") {
			continue
		}
		// Check the latency of worker's /health endpoint.
		lat := 5000.0 // default to 5s
		url := fmt.Sprintf("http://%s/health", instance.ip)
		start := time.Now()
		if resp, err := client.Get(url); err != nil {
			hasError = true
			fmt.Fprintf(ctx.Stderr(), "client.Get(%s) failed: %v\n", url, err)
		} else if resp.StatusCode != http.StatusOK {
			hasError = true
			resp.Body.Close()
			fmt.Fprintf(ctx.Stderr(), "got status code %v while checking %s, expected 200", resp.StatusCode, url)
		} else {
			resp.Body.Close()
			// Convert to ms.
			lat = float64(time.Now().Sub(start).Nanoseconds()) / 1000000.0
			if ctx.Verbose() {
				fmt.Fprintf(ctx.Stdout(), "/health latency for %s: %f ms\n", url, lat)
			}
		}
		instance.nginxStat.healthCheckLatency = lat
	}
	if hasError {
		return fmt.Errorf("some checks failed")
	}
	return nil
}

// sendToGCM sends instance stats data to GCM.
func sendToGCM(ctx *tool.Context, instances []*gceInstanceData) error {
	s, err := monitoring.Authenticate(serviceAccountFlag, keyFileFlag)
	if err != nil {
		return err
	}
	timeStr := time.Now().Format(time.RFC3339)
	for _, instance := range instances {
		msg := fmt.Sprintf("Send gce instance data for %s (%s)\n", instance.name, instance.zone)
		for _, metricName := range gceMetricNames {
			value := -1.0
			switch metricName {
			case "cpu-usage":
				value = instance.stat.cpuUsage
			case "memory-usage":
				value = instance.stat.memUsage
			case "disk-usage":
				value = instance.stat.diskUsage
			case "ping":
				value = instance.stat.pingLatency
			case "tcpconn":
				value = instance.stat.tcpconn
			default:
				test.Fail(ctx, msg)
				return fmt.Errorf("Invalid metric name: %q", metricName)
			}
			// GCM treats 0 and missing value the same.
			if value == 0 {
				value = 0.0001
			}
			if err := sendInstanceDataToGCM(s, "gce-instance", metricName, timeStr, instance, value); err != nil {
				test.Fail(ctx, msg)
				return fmt.Errorf("failed to add %q to GCM: %v\n", metricName, err)
			}
		}

		msg = fmt.Sprintf("Send nginx data for %s (%s)\n", instance.name, instance.zone)
		for _, metricName := range nginxMetricNames {
			nginxMetricNames = []string{"healthCheckLatency", "qps", "active-connections", "reading-connections", "writing-connections", "waiting-connections"}
			value := -1.0
			switch metricName {
			case "healthCheckLatency":
				value = instance.nginxStat.healthCheckLatency
			case "qps":
				value = instance.nginxStat.qps
			case "active-connections":
				value = instance.nginxStat.activeConnections
			case "reading-connections":
				value = instance.nginxStat.readingConnections
			case "writing-connections":
				value = instance.nginxStat.writingConnections
			case "waiting-connections":
				value = instance.nginxStat.waitingConnections
			default:
				test.Fail(ctx, msg)
				return fmt.Errorf("Invalid metric name: %q", metricName)
			}
			// GCM treats 0 and missing value the same.
			if value == 0 {
				value = 0.0001
			}
			if err := sendInstanceDataToGCM(s, "nginx", metricName, timeStr, instance, value); err != nil {
				test.Fail(ctx, msg)
				return fmt.Errorf("failed to add %q to GCM: %v\n", metricName, err)
			}
		}

		test.Pass(ctx, msg)
	}
	return nil
}

// sendInstanceDataToGCM sends a single instance's stat to GCM.
func sendInstanceDataToGCM(s *cloudmonitoring.Service, metricType, metricName, timeStr string, instance *gceInstanceData, value float64) error {
	pt := cloudmonitoring.Point{
		DoubleValue: value,
		Start:       timeStr,
		End:         timeStr,
	}
	md := monitoring.CustomMetricDescriptors[metricType]
	_, err := s.Timeseries.Write(projectFlag, &cloudmonitoring.WriteTimeseriesRequest{
		Timeseries: []*cloudmonitoring.TimeseriesPoint{
			&cloudmonitoring.TimeseriesPoint{
				Point: &pt,
				TimeseriesDesc: &cloudmonitoring.TimeseriesDescriptor{
					Metric: md.Name,
					Labels: map[string]string{
						md.Labels[0].Key: instance.name,
						md.Labels[1].Key: instance.zone,
						md.Labels[2].Key: metricName,
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
