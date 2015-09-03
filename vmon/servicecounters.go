// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/cloudmonitoring/v2beta2"
	"v.io/jiri/lib/runutil"
	"v.io/jiri/lib/tool"
	"v.io/x/devtools/internal/monitoring"
	"v.io/x/devtools/internal/test"
)

type prodServiceCounter struct {
	name       string
	objectName string
}

// checkServiceCounters checks all service counters and adds the results to GCM.
func checkServiceCounters(ctx *tool.Context) error {
	counters := []prodServiceCounter{
		prodServiceCounter{
			name:       "mounttable nodes",
			objectName: namespaceRootFlag + "/__debug/stats/mounttable/num-nodes",
		},
		prodServiceCounter{
			name:       "mounttable mounted servers",
			objectName: namespaceRootFlag + "/__debug/stats/mounttable/num-mounted-servers",
		},
	}

	hasError := false
	for _, counter := range counters {
		if v, err := checkSingleCounter(ctx, counter); err != nil {
			if err == runutil.CommandTimedOutErr {
				test.Warn(ctx, "%s: %d [TIMEOUT]\n", counter.name, int(v))
			} else {
				test.Fail(ctx, "%s\n", counter.name)
				fmt.Fprintf(ctx.Stderr(), "%v\n", err)
			}
			hasError = true
		} else {
			test.Pass(ctx, "%s: %d\n", counter.name, int(v))
		}
	}
	if hasError {
		return fmt.Errorf("failed to check some counters.")
	}
	return nil
}

func checkSingleCounter(ctx *tool.Context, counter prodServiceCounter) (float64, error) {
	// Run "debug stats read" to get the counter's value.
	debug := filepath.Join(binDirFlag, "debug")
	var buf bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &buf
	opts.Stderr = &buf
	value := 0.0
	if err := ctx.Run().TimedCommandWithOpts(timeout, opts, debug, "--v23.credentials", credentialsFlag, "stats", "read", counter.objectName); err != nil {
		if err != runutil.CommandTimedOutErr {
			return 0, fmt.Errorf("debug command failed: %v\n%s", err, buf.String())
		}
		return 0, err
	}
	parts := strings.Split(strings.TrimSpace(buf.String()), " ")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid debug output: %s", buf.String())
	}
	var err error
	value, err = strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, fmt.Errorf("ParseFloat(%s) failed: %v", parts[1], err)
	}

	// Add the counter as a custom metric to GCM.
	serviceLocation := monitoring.ServiceLocationMap[namespaceRootFlag]
	if serviceLocation == nil {
		return 0, fmt.Errorf("service location not found for %q", namespaceRootFlag)
	}
	mdLat := monitoring.CustomMetricDescriptors["service-counters"]
	s, err := monitoring.Authenticate(serviceAccountFlag, keyFileFlag)
	if err != nil {
		return 0, err
	}
	timeStr := time.Now().Format(time.RFC3339)
	_, err = s.Timeseries.Write(projectFlag, &cloudmonitoring.WriteTimeseriesRequest{
		Timeseries: []*cloudmonitoring.TimeseriesPoint{
			&cloudmonitoring.TimeseriesPoint{
				Point: &cloudmonitoring.Point{
					DoubleValue: value,
					Start:       timeStr,
					End:         timeStr,
				},
				TimeseriesDesc: &cloudmonitoring.TimeseriesDescriptor{
					Metric: mdLat.Name,
					Labels: map[string]string{
						mdLat.Labels[0].Key: serviceLocation.Instance,
						mdLat.Labels[1].Key: serviceLocation.Zone,
						mdLat.Labels[2].Key: counter.name,
					},
				},
			},
		},
	}).Do()
	if err != nil {
		return 0, fmt.Errorf("Timeseries Write failed: %v", err)
	}

	return value, nil
}
