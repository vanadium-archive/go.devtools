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

	cloudmonitoring "google.golang.org/api/monitoring/v3"

	"v.io/jiri/tool"
	"v.io/v23/context"
	"v.io/x/devtools/internal/monitoring"
	"v.io/x/devtools/internal/test"
)

// checkRPCLoadTest checks the result of RPC load test and sends the result to GCM.
func checkRPCLoadTest(v23ctx *context.T, ctx *tool.Context, s *cloudmonitoring.Service) error {
	// Parse result file.
	seq := ctx.NewSeq()
	resultFile := filepath.Join(os.Getenv("WORKSPACE"), "load_stats.json")
	bytes, err := seq.ReadFile(resultFile)
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
	mdRpcLoadTest, err := monitoring.GetMetric("rpc-load-test", projectFlag)
	if err != nil {
		return err
	}
	fi, err := seq.Stat(resultFile)
	if err != nil {
		return err
	}
	timeStr := fi.ModTime().UTC().Format(time.RFC3339)
	for label, value := range items {
		if err := sendDataToGCM(s, mdRpcLoadTest, value, timeStr, "", "", label); err != nil {
			test.Fail(ctx, "%s: %f\n", label, value)
			return fmt.Errorf("Timeseries Write failed: %v", err)
		}
		test.Pass(ctx, "%s: %f\n", label, value)
	}
	return nil
}
