// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"time"

	cloudmonitoring "google.golang.org/api/monitoring/v3"

	"v.io/jiri/tool"
	"v.io/v23/context"
	"v.io/x/devtools/internal/monitoring"
	"v.io/x/devtools/internal/test"
	"v.io/x/ref/services/stats"
)

var (
	latMethodRE = regexp.MustCompile(`.*/methods/([^/]*)/.*`)
	statsSuffix = "__debug/stats/rpc/server/routing-id/*/methods/*/latency-ms/delta1m"
)

type perMethodLatencyData struct {
	location *monitoring.ServiceLocation
	latency  map[string]float64
}

// checkServicePerMethodLatency checks service per-method RPC latency and
// adds the results to GCM.
func checkServicePerMethodLatency(v23ctx *context.T, ctx *tool.Context, s *cloudmonitoring.Service) error {
	serviceNames := []string{
		monitoring.SNMounttable,
		monitoring.SNIdentity,
		monitoring.SNRole,
		monitoring.SNProxy,
		monitoring.SNBenchmark,
		monitoring.SNAllocator,
	}

	hasError := false
	mdLatPerMethod, err := monitoring.GetMetric("service-permethod-latency", projectFlag)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, serviceName := range serviceNames {
		lats, err := checkSingleServicePerMethodLatency(v23ctx, ctx, serviceName)
		if err != nil {
			test.Fail(ctx, "%s\n", serviceName)
			fmt.Fprintf(ctx.Stderr(), "%v\n", err)
			hasError = true
			continue
		}
		aggByMethod := map[string]*aggregator{}
		for _, lat := range lats {
			label := fmt.Sprintf("%s (%s, %s)", serviceName, lat.location.Instance, lat.location.Zone)
			result := ""
			methods := []string{}
			for m := range lat.latency {
				methods = append(methods, m)
			}
			sort.Strings(methods)
			for _, m := range methods {
				result += fmt.Sprintf("     - %s: %f\n", m, lat.latency[m])
			}

			// Send to GCM.
			for _, m := range methods {
				curLat := lat.latency[m]
				if _, ok := aggByMethod[m]; !ok {
					aggByMethod[m] = newAggregator()
				}
				aggByMethod[m].add(curLat)
				if err := sendDataToGCM(s, mdLatPerMethod, curLat, now, lat.location.Instance, lat.location.Zone, serviceName, m); err != nil {
					return err
				}
			}
			test.Pass(ctx, "%s:\n%s", label, result)
		}

		// Send aggregated data to GCM.
		for method, agg := range aggByMethod {
			mdAgg, err := monitoring.GetMetric("service-permethod-latency-agg", projectFlag)
			if err != nil {
				return err
			}
			if err := sendAggregatedDataToGCM(ctx, s, mdAgg, agg, now, serviceName, method); err != nil {
				return err
			}
		}
	}
	if hasError {
		return fmt.Errorf("failed to check per-method RPC latency for some services.")
	}
	return nil
}

func checkSingleServicePerMethodLatency(v23ctx *context.T, ctx *tool.Context, serviceName string) ([]perMethodLatencyData, error) {
	mountedName, err := monitoring.GetServiceMountedName(namespaceRootFlag, serviceName)
	if err != nil {
		return nil, err
	}

	// Resolve name and group results by routing ids.
	groups, err := monitoring.ResolveAndProcessServiceName(v23ctx, ctx, serviceName, mountedName)
	if err != nil {
		return nil, err
	}

	// Get per-method latency for each group.
	latencies := []perMethodLatencyData{}
	errors := []error{}
	for _, group := range groups {
		latency := map[string]float64{}
		// Run "debug stats read" for the corresponding object.
		statsResult, err := monitoring.GetStat(v23ctx, ctx, group, statsSuffix)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		// Parse output.
		latPerMethod := map[string]float64{}
		for _, r := range statsResult {
			data, ok := r.Value.(stats.HistogramValue)
			if !ok {
				return nil, fmt.Errorf("invalid latency data: %v", r)
			}
			matches := latMethodRE.FindStringSubmatch(r.Name)
			if matches == nil {
				continue
			}
			method := matches[1]
			latency := 0.0
			if data.Count != 0 {
				latency = (float64)(data.Sum) / (float64)(data.Count)
			}
			latPerMethod[method] = math.Max(latPerMethod[method], latency)
		}
		latency = latPerMethod
		if len(latency) == 0 {
			errors = append(errors, fmt.Errorf("failed to check latency for service %q", serviceName))
			continue
		}
		location, err := monitoring.GetServiceLocation(v23ctx, ctx, group)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		latencies = append(latencies, perMethodLatencyData{
			location: location,
			latency:  latency,
		})
	}
	if len(errors) == len(groups) {
		return nil, fmt.Errorf("%v", errors)
	}

	return latencies, nil
}
