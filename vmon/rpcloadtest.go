// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/api/cloudmonitoring/v2beta2"

	"v.io/jiri/tool"
	"v.io/x/devtools/internal/monitoring"
	"v.io/x/devtools/internal/test"
)

// checkRPCLoadTest checks the result of RPC load test and sends the result to GCM.
func checkRPCLoadTest(ctx *tool.Context) error {
	// Parse result file.
	s := ctx.NewSeq()
	resultFile := filepath.Join(os.Getenv("WORKSPACE"), "load_stats.json")
	bytes, err := s.ReadFile(resultFile)
	if err != nil {
		return err
	}
	var results struct {
		MsecPerRpc float64
		Qps        float64
	}
	if err := json.Unmarshal(bytes, &results); err != nil {
		return nil
	}

	// Send to GCM.
	items := map[string]float64{
		"latency": results.MsecPerRpc,
		"qps":     results.Qps,
	}
	mdRpcLoadTest := monitoring.CustomMetricDescriptors["rpc-load-test"]
	sa, err := monitoring.Authenticate(keyFileFlag)
	if err != nil {
		return err
	}
	fi, err := s.Stat(resultFile)
	if err != nil {
		return err
	}
	timeStr := fi.ModTime().Format(time.RFC3339)
	for label, value := range items {
		_, err = sa.Timeseries.Write(projectFlag, &cloudmonitoring.WriteTimeseriesRequest{
			Timeseries: []*cloudmonitoring.TimeseriesPoint{
				&cloudmonitoring.TimeseriesPoint{
					Point: &cloudmonitoring.Point{
						DoubleValue: value,
						Start:       timeStr,
						End:         timeStr,
					},
					TimeseriesDesc: &cloudmonitoring.TimeseriesDescriptor{
						Metric: mdRpcLoadTest.Name,
						Labels: map[string]string{
							mdRpcLoadTest.Labels[0].Key: label,
						},
					},
				},
			},
		}).Do()
		if err != nil {
			test.Fail(ctx, "%s: %f\n", label, value)
			return fmt.Errorf("Timeseries Write failed: %v", err)
		}
		test.Pass(ctx, "%s: %f\n", label, value)
	}
	return nil
}
