// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/cloudmonitoring/v2beta2"

	"v.io/x/devtools/internal/monitoring"
	"v.io/x/devtools/internal/runutil"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/tool"
)

var (
	qpsRE = regexp.MustCompile(`.*/methods/([^/]*)/.*: Count: (\d+).*`)
)

type qpsInfo struct {
	name       string
	objectName string
}

// checkAllServiceQPS checks service RPC QPS (per-method and total) and adds
// the results to GCM.
func checkAllServiceQPS(ctx *tool.Context) error {
	infos := []qpsInfo{
		qpsInfo{
			name:       "mounttable",
			objectName: namespaceRootFlag + "/__debug/stats/rpc/server/routing-id/*/methods/*/latency-ms/delta1m",
		},
	}

	hasError := false
	for _, info := range infos {
		if qpsPerMethod, total, err := checkOneServiceQPS(ctx, info); err != nil {
			if err == runutil.CommandTimedOutErr {
				test.Warn(ctx, "%s: [TIMEOUT]\n", info.name)
			} else {
				test.Fail(ctx, "%s\n", info.name)
				fmt.Fprintf(ctx.Stderr(), "%v\n", err)
			}
			hasError = true
		} else {
			test.Pass(ctx, "%s:\n     - Per method: %v\n     - Total: %f\n",
				info.name, qpsPerMethod, total)
		}
	}
	if hasError {
		return fmt.Errorf("failed to check RPC QPS for some services.")
	}
	return nil
}

func checkOneServiceQPS(ctx *tool.Context, info qpsInfo) (map[string]float64, float64, error) {
	qpsPerMethod, totalQPS, err := getQPSData(ctx, info)
	if err != nil {
		return nil, -1, err
	}
	if err := sendQPSDataToGCM(qpsPerMethod, totalQPS, info); err != nil {
		return nil, -1, err
	}

	return qpsPerMethod, totalQPS, nil
}

func getQPSData(ctx *tool.Context, info qpsInfo) (map[string]float64, float64, error) {
	// Run "debug stats read" for the corresponding object.
	debug := filepath.Join(binDirFlag, "debug")
	var buf bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &buf
	opts.Stderr = &buf
	args := []string{
		"--v23.credentials",
		credentialsFlag,
		"stats",
		"read",
		info.objectName,
	}
	if err := ctx.Run().TimedCommandWithOpts(timeout, opts, debug, args...); err != nil {
		if err != runutil.CommandTimedOutErr {
			return nil, -1, fmt.Errorf("debug command failed: %v\n%s", err, buf.String())
		}
		return nil, -1, err
	}

	// Parse output.
	qpsPerMethod := map[string]float64{}
	totalQPS := 0.0
	lines := strings.Split(buf.String(), "\n")
	for _, line := range lines {
		// Each line is in the form of:
		// <root>/__debug/stats/rpc/server/routing-id/<routing-id>/methods/<method>/latency-ms/delta1m: Count: 10  Min: 38  Max: 43  Avg: 39.30
		matches := qpsRE.FindStringSubmatch(line)
		if matches != nil {
			method := matches[1]
			strCount := matches[2]
			count, err := strconv.ParseFloat(strCount, 64)
			qps := count / 60.0
			if err != nil {
				return nil, -1, fmt.Errorf("strconv.ParseFloat(%s, 64) failed: %v", strCount, err)
			}
			qpsPerMethod[method] += qps
			totalQPS += qps
		}
	}

	return qpsPerMethod, totalQPS, nil
}

func sendQPSDataToGCM(qpsPerMethod map[string]float64, totalQPS float64, info qpsInfo) error {
	// Send QPS per method to GCM.
	serviceLocation := monitoring.ServiceLocationMap[namespaceRootFlag]
	if serviceLocation == nil {
		return fmt.Errorf("service location not found for %q", namespaceRootFlag)
	}
	mdQPSPerMethod := monitoring.CustomMetricDescriptors["service-qps-method"]
	s, err := monitoring.Authenticate(serviceAccountFlag, keyFileFlag)
	if err != nil {
		return err
	}
	timeStr := time.Now().Format(time.RFC3339)
	for method, qps := range qpsPerMethod {
		if qps == 0 {
			continue
		}
		_, err := s.Timeseries.Write(projectFlag, &cloudmonitoring.WriteTimeseriesRequest{
			Timeseries: []*cloudmonitoring.TimeseriesPoint{
				&cloudmonitoring.TimeseriesPoint{
					Point: &cloudmonitoring.Point{
						DoubleValue: qps,
						Start:       timeStr,
						End:         timeStr,
					},
					TimeseriesDesc: &cloudmonitoring.TimeseriesDescriptor{
						Metric: mdQPSPerMethod.Name,
						Labels: map[string]string{
							mdQPSPerMethod.Labels[0].Key: serviceLocation.Instance,
							mdQPSPerMethod.Labels[1].Key: serviceLocation.Zone,
							mdQPSPerMethod.Labels[2].Key: info.name,
							mdQPSPerMethod.Labels[3].Key: method,
						},
					},
				},
			},
		}).Do()
		if err != nil {
			return fmt.Errorf("Timeseries Write failed: %v", err)
		}
	}

	// Send total QPS to GCM.
	if totalQPS != 0 {
		mdQPSTotal := monitoring.CustomMetricDescriptors["service-qps-total"]
		_, err = s.Timeseries.Write(projectFlag, &cloudmonitoring.WriteTimeseriesRequest{
			Timeseries: []*cloudmonitoring.TimeseriesPoint{
				&cloudmonitoring.TimeseriesPoint{
					Point: &cloudmonitoring.Point{
						DoubleValue: totalQPS,
						Start:       timeStr,
						End:         timeStr,
					},
					TimeseriesDesc: &cloudmonitoring.TimeseriesDescriptor{
						Metric: mdQPSTotal.Name,
						Labels: map[string]string{
							mdQPSTotal.Labels[0].Key: serviceLocation.Instance,
							mdQPSTotal.Labels[1].Key: serviceLocation.Zone,
							mdQPSTotal.Labels[2].Key: info.name,
						},
					},
				},
			},
		}).Do()
		if err != nil {
			return fmt.Errorf("Timeseries Write failed: %v", err)
		}
	}

	return nil
}
