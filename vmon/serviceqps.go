// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/cloudmonitoring/v2beta2"

	"v.io/jiri/tool"
	"v.io/x/devtools/internal/monitoring"
	"v.io/x/devtools/internal/test"
)

const (
	qpsSuffix = "__debug/stats/rpc/server/routing-id/*/methods/*/latency-ms/delta1m"
)

var (
	qpsRE = regexp.MustCompile(`.*/methods/([^/]*)/.*: Count: (\d+).*`)
)

type qpsData struct {
	location     *monitoring.ServiceLocation
	perMethodQPS map[string]float64
	totalQPS     float64
}

// checkServiceQPS checks service RPC QPS (per-method and total) and adds
// the results to GCM.
func checkServiceQPS(ctx *tool.Context, s *cloudmonitoring.Service) error {
	serviceNames := []string{
		snMounttable,
		snApplications,
		snBinaries,
		snIdentity,
		snRole,
		// snProxy,
		// TODO(jingjin): add this back when the RPC bug is fixed.
		snGroups,
	}

	hasError := false
	mdPerMethodQPS := monitoring.CustomMetricDescriptors["service-qps-method"]
	mdTotalQPS := monitoring.CustomMetricDescriptors["service-qps-total"]
	now := time.Now().Format(time.RFC3339)
	for _, serviceName := range serviceNames {
		qps, err := checkSingleServiceQPS(ctx, serviceName)
		if err != nil {
			test.Fail(ctx, "%s\n", serviceName)
			fmt.Fprintf(ctx.Stderr(), "%v\n", err)
			hasError = true
			continue
		}
		agg := newAggregator()
		aggByMethod := map[string]*aggregator{}
		for _, curQPS := range qps {
			instance := curQPS.location.Instance
			zone := curQPS.location.Zone
			agg.add(curQPS.totalQPS)

			label := fmt.Sprintf("%s (%s, %s)", serviceName, instance, zone)
			result := ""
			methods := []string{}
			for m := range curQPS.perMethodQPS {
				methods = append(methods, m)
			}
			sort.Strings(methods)
			for _, m := range methods {
				result += fmt.Sprintf("     - %s: %f\n", m, curQPS.perMethodQPS[m])
			}
			result += fmt.Sprintf("     Total: %f", curQPS.totalQPS)

			// Send data to GCM.
			// Total qps:
			if err := sendDataToGCM(s, mdTotalQPS, curQPS.totalQPS, now, instance, zone, serviceName); err != nil {
				return err
			}
			// Per-method qps:
			for _, m := range methods {
				curPerMethodQPS := curQPS.perMethodQPS[m]
				if _, ok := aggByMethod[m]; !ok {
					aggByMethod[m] = newAggregator()
				}
				aggByMethod[m].add(curPerMethodQPS)
				if err := sendDataToGCM(s, mdPerMethodQPS, curPerMethodQPS, now, instance, zone, serviceName, m); err != nil {
					return err
				}
			}
			test.Pass(ctx, "%s:\n%s\n", label, result)
		}

		// Send aggregated data to GCM.
		if err := sendAggregatedDataToGCM(ctx, s, monitoring.CustomMetricDescriptors["service-qps-total-agg"], agg, now, serviceName); err != nil {
			return err
		}
		for method, agg := range aggByMethod {
			if err := sendAggregatedDataToGCM(ctx, s, monitoring.CustomMetricDescriptors["service-qps-method-agg"], agg, now, serviceName, method); err != nil {
				return err
			}
		}
	}
	if hasError {
		return fmt.Errorf("failed to check RPC QPS for some services.")
	}
	return nil
}

func checkSingleServiceQPS(ctx *tool.Context, serviceName string) ([]qpsData, error) {
	mountedName, err := getMountedName(serviceName)
	if err != nil {
		return nil, err
	}

	// Resolve name and group results by routing ids.
	groups, err := resolveAndProcessServiceName(ctx, serviceName, mountedName)
	if err != nil {
		return nil, err
	}

	// Get qps for each group.
	qps := []qpsData{}
	for _, group := range groups {
		availableName := group[0]
		perMethodQPS := map[string]float64{}
		totalQPS := 0.0
		for _, name := range group {
			// Run "debug stats read" for the corresponding object.
			if output, err := getStat(ctx, fmt.Sprintf("%s/%s", name, qpsSuffix), false); err == nil {
				// Parse output.
				curPerMethodQPS := map[string]float64{}
				curTotalQPS := 0.0
				lines := strings.Split(output, "\n")
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
							return nil, fmt.Errorf("strconv.ParseFloat(%s, 64) failed: %v", strCount, err)
						}
						curPerMethodQPS[method] += qps
						curTotalQPS += qps
					}
				}
				availableName = name
				perMethodQPS = curPerMethodQPS
				totalQPS = curTotalQPS
				break
			}
		}
		if len(perMethodQPS) == 0 {
			return nil, fmt.Errorf("failed to check qps for service %q", serviceName)
		}
		location, err := getServiceLocation(ctx, availableName, serviceName)
		if err != nil {
			return nil, err
		}
		qps = append(qps, qpsData{
			location:     location,
			perMethodQPS: perMethodQPS,
			totalQPS:     totalQPS,
		})
	}

	return qps, nil
}
