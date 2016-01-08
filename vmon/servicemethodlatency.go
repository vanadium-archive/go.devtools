// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"time"

	"google.golang.org/api/cloudmonitoring/v2beta2"

	"v.io/jiri/tool"
	"v.io/x/devtools/internal/monitoring"
	"v.io/x/devtools/internal/test"
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
func checkServicePerMethodLatency(ctx *tool.Context, s *cloudmonitoring.Service) error {
	serviceNames := []string{
		snMounttable,
		snBinaries,
		snApplications,
		snIdentity,
		snGroups,
		snRole,
		snProxy,
	}

	hasError := false
	mdLatPerMethod := monitoring.CustomMetricDescriptors["service-permethod-latency"]
	now := time.Now().Format(time.RFC3339)
	for _, serviceName := range serviceNames {
		lats, err := checkSingleServicePerMethodLatency(ctx, serviceName)
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
			if err := sendAggregatedDataToGCM(ctx, s, monitoring.CustomMetricDescriptors["service-permethod-latency-agg"], agg, now, serviceName, method); err != nil {
				return err
			}
		}
	}
	if hasError {
		return fmt.Errorf("failed to check per-method RPC latency for some services.")
	}
	return nil
}

func checkSingleServicePerMethodLatency(ctx *tool.Context, serviceName string) ([]perMethodLatencyData, error) {
	mountedName, err := getMountedName(serviceName)
	if err != nil {
		return nil, err
	}

	// Resolve name and group results by routing ids.
	groups, err := resolveAndProcessServiceName(ctx, serviceName, mountedName)
	if err != nil {
		return nil, err
	}

	// Get per-method latency for each group.
	latencies := []perMethodLatencyData{}
	for _, group := range groups {
		latency := map[string]float64{}
		availableName := group[0]
		for _, name := range group {
			// Run "debug stats read" for the corresponding object.
			if output, err := getStat(ctx, fmt.Sprintf("%s/%s", name, statsSuffix), true); err == nil {
				// Parse output.
				var stats []struct {
					Name  string
					Value struct {
						Count float64
						Sum   float64
					}
				}
				latPerMethod := map[string]float64{}
				if err := json.Unmarshal([]byte(output), &stats); err != nil {
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
				latency = latPerMethod
				availableName = name
				break
			}
		}
		if len(latency) == 0 {
			return nil, fmt.Errorf("failed to check latency for service %q", serviceName)
		}
		location, err := getServiceLocation(ctx, availableName, serviceName)
		if err != nil {
			return nil, err
		}
		latencies = append(latencies, perMethodLatencyData{
			location: location,
			latency:  latency,
		})
	}

	return latencies, nil
}
