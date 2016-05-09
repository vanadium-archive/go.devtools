// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
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

const (
	qpsSuffix = "__debug/stats/rpc/server/routing-id/*/methods/*/latency-ms/delta1m"
)

var (
	qpsRE = regexp.MustCompile(`.*/methods/([^/]*)/.*`)
)

type qpsData struct {
	location     *monitoring.ServiceLocation
	perMethodQPS map[string]float64
	totalQPS     float64
}

// checkServiceQPS checks service RPC QPS (per-method and total) and adds
// the results to GCM.
func checkServiceQPS(v23ctx *context.T, ctx *tool.Context, s *cloudmonitoring.Service) error {
	serviceNames := []string{
		monitoring.SNMounttable,
		monitoring.SNIdentity,
		monitoring.SNRole,
		monitoring.SNProxy,
		monitoring.SNBenchmark,
		monitoring.SNAllocator,
	}

	hasError := false
	mdPerMethodQPS, err := monitoring.GetMetric("service-qps-method", projectFlag)
	if err != nil {
		return err
	}
	mdTotalQPS, err := monitoring.GetMetric("service-qps-total", projectFlag)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, serviceName := range serviceNames {
		qps, err := checkSingleServiceQPS(v23ctx, ctx, serviceName)
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
		mdTotalAgg, err := monitoring.GetMetric("service-qps-total-agg", projectFlag)
		if err != nil {
			return err
		}
		if err := sendAggregatedDataToGCM(ctx, s, mdTotalAgg, agg, now, serviceName); err != nil {
			return err
		}
		for method, agg := range aggByMethod {
			mdMethodAgg, err := monitoring.GetMetric("service-qps-method-agg", projectFlag)
			if err != nil {
				return err
			}
			if err := sendAggregatedDataToGCM(ctx, s, mdMethodAgg, agg, now, serviceName, method); err != nil {
				return err
			}
		}
	}
	if hasError {
		return fmt.Errorf("failed to check RPC QPS for some services.")
	}
	return nil
}

func checkSingleServiceQPS(v23ctx *context.T, ctx *tool.Context, serviceName string) ([]qpsData, error) {
	mountedName, err := monitoring.GetServiceMountedName(namespaceRootFlag, serviceName)
	if err != nil {
		return nil, err
	}

	// Resolve name and group results by routing ids.
	groups, err := monitoring.ResolveAndProcessServiceName(v23ctx, ctx, serviceName, mountedName)
	if err != nil {
		return nil, err
	}

	// Get qps for each group.
	qps := []qpsData{}
	errors := []error{}
	for _, group := range groups {
		perMethodQPS := map[string]float64{}
		totalQPS := 0.0
		qpsResults, err := monitoring.GetStat(v23ctx, ctx, group, qpsSuffix)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		curPerMethodQPS := map[string]float64{}
		curTotalQPS := 0.0
		for _, r := range qpsResults {
			data, ok := r.Value.(stats.HistogramValue)
			if !ok {
				return nil, fmt.Errorf("invalid qps data: %v", r)
			}
			matches := qpsRE.FindStringSubmatch(r.Name)
			if matches == nil {
				continue
			}
			method := matches[1]
			qps := (float64)(data.Count) / 60.0
			curPerMethodQPS[method] += qps
			curTotalQPS += qps
		}
		perMethodQPS = curPerMethodQPS
		totalQPS = curTotalQPS
		if len(perMethodQPS) == 0 {
			errors = append(errors, fmt.Errorf("failed to check qps for service %q", serviceName))
			continue
		}
		location, err := monitoring.GetServiceLocation(v23ctx, ctx, group)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		qps = append(qps, qpsData{
			location:     location,
			perMethodQPS: perMethodQPS,
			totalQPS:     totalQPS,
		})
	}

	if len(errors) == len(groups) {
		return nil, fmt.Errorf("%v", errors)
	}

	return qps, nil
}
