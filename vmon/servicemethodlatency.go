// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"time"

	"google.golang.org/api/cloudmonitoring/v2beta2"

	"v.io/jiri/runutil"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/monitoring"
	"v.io/x/devtools/internal/test"
)

var (
	latMethodRE = regexp.MustCompile(`.*/methods/([^/]*)/.*`)
	statsSuffix = "/__debug/stats/rpc/server/routing-id/*/methods/*/latency-ms/delta1m"
)

type perMethodLatencyInfo struct {
	name       string
	objectName string
}

// checkAllServicePerMethodLatency checks service per-method RPC latency and
// adds the results to GCM.
func checkAllServicePerMethodLatency(ctx *tool.Context) error {
	infos := []perMethodLatencyInfo{
		perMethodLatencyInfo{
			name:       "mounttable",
			objectName: namespaceRootFlag,
		},
		perMethodLatencyInfo{
			name:       "binaries",
			objectName: namespaceRootFlag + "/binaries",
		},
		perMethodLatencyInfo{
			name:       "applications",
			objectName: namespaceRootFlag + "/applications",
		},
		perMethodLatencyInfo{
			name:       "groups",
			objectName: namespaceRootFlag + "/groups",
		},
		perMethodLatencyInfo{
			name:       "proxy-mon",
			objectName: namespaceRootFlag + "/proxy-mon",
		},
	}

	hasError := false
	for _, info := range infos {
		if latPerMethod, err := checkOneServicePerMethodLatency(ctx, info); err != nil {
			if err == runutil.CommandTimedOutErr {
				test.Warn(ctx, "%s: [TIMEOUT]\n", info.name)
			} else {
				test.Fail(ctx, "%s\n", info.name)
				fmt.Fprintf(ctx.Stderr(), "%v\n", err)
			}
			hasError = true
		} else {
			test.Pass(ctx, "%s:\n     - Per method: %v\n", info.name, latPerMethod)
		}
	}
	if hasError {
		return fmt.Errorf("failed to check per-method RPC latency for some services.")
	}
	return nil
}

func checkOneServicePerMethodLatency(ctx *tool.Context, info perMethodLatencyInfo) (map[string]float64, error) {
	// Run "debug stats read" for the corresponding object.
	debug := filepath.Join(binDirFlag, "debug")
	var buf bytes.Buffer
	var stderr bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &buf
	opts.Stderr = &stderr
	args := []string{
		"--v23.credentials",
		credentialsFlag,
		"stats",
		"read",
		"-json",
		info.objectName + statsSuffix,
	}
	if err := ctx.Run().TimedCommandWithOpts(timeout, opts, debug, args...); err != nil {
		if err != runutil.CommandTimedOutErr {
			return nil, fmt.Errorf("debug command failed: %v\n%s", err, stderr.String())
		}
		fmt.Fprintf(ctx.Stdout(), "%s %s TIMED OUT: %s\n", debug, args, stderr.String())
		return nil, err
	}

	// Parse output.
	var stats []struct {
		Name  string
		Value struct {
			Count float64
			Sum   float64
		}
	}
	latPerMethod := map[string]float64{}
	if err := json.Unmarshal(buf.Bytes(), &stats); err != nil {
		return nil, fmt.Errorf("json.Unmarshal() failed: %v", err)
	}
	for _, s := range stats {
		matches := latMethodRE.FindStringSubmatch(s.Name)
		if matches == nil {
			continue
		}
		method := matches[1]
		latency := 0.0
		if s.Value.Count != 0 {
			latency = s.Value.Sum / s.Value.Count
		}
		latPerMethod[method] = math.Max(latPerMethod[method], latency)
	}

	// Send data to GCM.
	serviceLocation := monitoring.ServiceLocationMap[namespaceRootFlag]
	if serviceLocation == nil {
		return nil, fmt.Errorf("service location not found for %q", namespaceRootFlag)
	}
	mdLatPerMethod := monitoring.CustomMetricDescriptors["service-permethod-latency"]
	s, err := monitoring.Authenticate(keyFileFlag)
	if err != nil {
		return nil, err
	}
	timeStr := time.Now().Format(time.RFC3339)
	for method, lat := range latPerMethod {
		if lat == 0 {
			continue
		}
		_, err := s.Timeseries.Write(projectFlag, &cloudmonitoring.WriteTimeseriesRequest{
			Timeseries: []*cloudmonitoring.TimeseriesPoint{
				&cloudmonitoring.TimeseriesPoint{
					Point: &cloudmonitoring.Point{
						DoubleValue: lat,
						Start:       timeStr,
						End:         timeStr,
					},
					TimeseriesDesc: &cloudmonitoring.TimeseriesDescriptor{
						Metric: mdLatPerMethod.Name,
						Labels: map[string]string{
							mdLatPerMethod.Labels[0].Key: serviceLocation.Instance,
							mdLatPerMethod.Labels[1].Key: serviceLocation.Zone,
							mdLatPerMethod.Labels[2].Key: info.name,
							mdLatPerMethod.Labels[3].Key: method,
						},
					},
				},
			},
		}).Do()
		if err != nil {
			return nil, fmt.Errorf("Timeseries Write failed: %v", err)
		}
	}

	return latPerMethod, nil
}
