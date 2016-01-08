// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"time"

	"google.golang.org/api/cloudmonitoring/v2beta2"

	"v.io/jiri/runutil"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/monitoring"
	"v.io/x/devtools/internal/test"
)

var (
	// Empirically, running "debug stats read -json
	// /ns.dev.v.io:8101/binaries/__debug/stats/rpc/server/routing-id/*/methods/*/latency-ms/delta1m
	// ten times took a max of 14 seconds with a standard deviation of 2.6
	// seconds.  So we take max + 2 x stdev =~ 20 seconds.
	timeout = 20 * time.Second
)

type latencyData struct {
	location *monitoring.ServiceLocation
	latency  time.Duration
}

// checkServiceLatency checks all services and adds their check latency to GCM.
func checkServiceLatency(ctx *tool.Context, s *cloudmonitoring.Service) error {
	serviceNames := []string{
		snMounttable,
		snApplications,
		snBinaries,
		snMacaroon,
		snGoogleIdentity,
		snBinaryDischarger,
		snRole,
		snProxy,
		snGroups,
	}

	hasError := false
	mdLat := monitoring.CustomMetricDescriptors["service-latency"]
	now := time.Now().Format(time.RFC3339)
	for _, serviceName := range serviceNames {
		lats, err := checkSingleServiceLatency(ctx, serviceName)
		if err != nil {
			test.Fail(ctx, "%s\n", serviceName)
			fmt.Fprintf(ctx.Stderr(), "%v\n", err)
			hasError = true
			continue
		}
		agg := newAggregator()
		for _, lat := range lats {
			instance := lat.location.Instance
			zone := lat.location.Zone
			latMs := float64(lat.latency.Nanoseconds()) / 1000000.0
			agg.add(latMs)

			// Send data to GCM.
			if err := sendDataToGCM(s, mdLat, latMs, now, instance, zone, serviceName); err != nil {
				return err
			}

			label := fmt.Sprintf("%s (%s, %s)", serviceName, instance, zone)
			if lat.latency == timeout {
				test.Warn(ctx, "%s: %fms [TIMEOUT]\n", label, latMs)
			} else {
				test.Pass(ctx, "%s: %fms\n", label, latMs)
			}
		}

		// Send aggregated data to GCM.
		if err := sendAggregatedDataToGCM(ctx, s, monitoring.CustomMetricDescriptors["service-latency-agg"], agg, now, serviceName); err != nil {
			return err
		}
	}
	if hasError {
		return fmt.Errorf("Failed to check some services.")
	}
	return nil
}

func checkSingleServiceLatency(ctx *tool.Context, serviceName string) ([]latencyData, error) {
	// Get service's mounted name.
	serviceMountedName, err := getMountedName(serviceName)
	if err != nil {
		return nil, err
	}
	// For proxy, we send "signature" RPC to "proxy-mon/__debug" endpoint.
	if serviceName == snProxy {
		serviceMountedName = fmt.Sprintf("%s/__debug", serviceMountedName)
	}
	s := ctx.NewSeq()

	// Resolve name and group results by routing ids.
	groups, err := resolveAndProcessServiceName(ctx, serviceName, serviceMountedName)
	if err != nil {
		return nil, err
	}

	// For each group, get the latency from the first available name.
	vrpc := filepath.Join(binDirFlag, "vrpc")
	latencies := []latencyData{}
	for _, group := range groups {
		latency := timeout
		availableName := group[0]
		for _, name := range group {
			start := time.Now()
			var bufErr bytes.Buffer
			if err := s.Capture(ioutil.Discard, &bufErr).Timeout(timeout).
				Last(vrpc, "signature", "--insecure", name); err != nil {
				if !runutil.IsTimeout(err) {
					// Fail immediately on non-timeout errors (e.g. vrpc command errors).
					return nil, fmt.Errorf("%v: %s", err, bufErr.String())
				}
			} else {
				latency = time.Now().Sub(start)
				availableName = name
				break
			}
		}
		location, err := getServiceLocation(ctx, availableName, serviceName)
		if err != nil {
			return nil, err
		}
		latencies = append(latencies, latencyData{
			location: location,
			latency:  latency,
		})
	}

	return latencies, nil
}
