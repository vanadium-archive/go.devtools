// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"time"

	cloudmonitoring "google.golang.org/api/monitoring/v3"

	"v.io/jiri/tool"
	"v.io/v23/context"
	"v.io/v23/options"
	"v.io/v23/rpc/reserved"
	"v.io/v23/verror"
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
func checkServiceLatency(v23ctx *context.T, ctx *tool.Context, s *cloudmonitoring.Service) error {
	serviceNames := []string{
		monitoring.SNMounttable,
		monitoring.SNMacaroon,
		monitoring.SNGoogleIdentity,
		monitoring.SNBinaryDischarger,
		monitoring.SNRole,
		monitoring.SNProxy,
		monitoring.SNBenchmark,
	}

	hasError := false
	mdLat, err := monitoring.GetMetric("service-latency", projectFlag)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, serviceName := range serviceNames {
		lats, err := checkSingleServiceLatency(v23ctx, ctx, serviceName)
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
		mdAgg, err := monitoring.GetMetric("service-latency-agg", projectFlag)
		if err != nil {
			return err
		}
		if err := sendAggregatedDataToGCM(ctx, s, mdAgg, agg, now, serviceName); err != nil {
			return err
		}
	}
	if hasError {
		return fmt.Errorf("Failed to check some services.")
	}
	return nil
}

func checkSingleServiceLatency(v23ctx *context.T, ctx *tool.Context, serviceName string) ([]latencyData, error) {
	// Get service's mounted name.
	serviceMountedName, err := monitoring.GetServiceMountedName(namespaceRootFlag, serviceName)
	if err != nil {
		return nil, err
	}
	// For proxy, we send "signature" RPC to "proxy-mon/__debug" endpoint.
	if serviceName == monitoring.SNProxy {
		serviceMountedName = fmt.Sprintf("%s/__debug", serviceMountedName)
	}
	// Resolve name and group results by routing ids.
	groups, err := monitoring.ResolveAndProcessServiceName(v23ctx, ctx, serviceName, serviceMountedName)
	if err != nil {
		return nil, err
	}

	// For each group, get the latency from the first available name.
	latencies := []latencyData{}
	errors := []error{}
	for _, group := range groups {
		latency := timeout
		v23ctx, cancel := context.WithTimeout(v23ctx, timeout)
		defer cancel()
		start := time.Now()
		if _, err := reserved.Signature(v23ctx, "", options.Preresolved{&group}); err != nil {
			if verror.ErrorID(err) != verror.ErrTimeout.ID {
				errors = append(errors, err)
				continue
			}
		} else {
			latency = time.Now().Sub(start)
		}
		location, err := monitoring.GetServiceLocation(v23ctx, ctx, group)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		latencies = append(latencies, latencyData{
			location: location,
			latency:  latency,
		})
	}
	if len(errors) == len(groups) {
		return latencies, fmt.Errorf("%v", errors)
	}

	return latencies, nil
}
