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
	"v.io/x/devtools/internal/monitoring"
	"v.io/x/devtools/internal/test"
)

type prodServiceCounter struct {
	name       string
	statSuffix string
}

type counterData struct {
	location *monitoring.ServiceLocation
	value    float64
}

// checkServiceCounters checks all service counters and adds the results to GCM.
func checkServiceCounters(v23ctx *context.T, ctx *tool.Context, s *cloudmonitoring.Service) error {
	counters := map[string][]prodServiceCounter{
		snMounttable: []prodServiceCounter{
			prodServiceCounter{
				name:       "mounttable nodes",
				statSuffix: "__debug/stats/mounttable/num-nodes",
			},
			prodServiceCounter{
				name:       "mounttable mounted servers",
				statSuffix: "__debug/stats/mounttable/num-mounted-servers",
			},
		},
	}

	hasError := false
	mdCounter, err := monitoring.GetMetric("service-counters", projectFlag)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for serviceName, serviceCounters := range counters {
		for _, counter := range serviceCounters {
			vs, err := checkSingleCounter(v23ctx, ctx, serviceName, counter)
			if err != nil {
				test.Fail(ctx, "%s\n", counter.name)
				fmt.Fprintf(ctx.Stderr(), "%v\n", err)
				hasError = true
				continue
			}
			agg := newAggregator()
			for _, v := range vs {
				instance := v.location.Instance
				zone := v.location.Zone
				agg.add(v.value)

				// Send data to GCM.
				if err := sendDataToGCM(s, mdCounter, v.value, now, instance, zone, counter.name); err != nil {
					return err
				}

				label := fmt.Sprintf("%s (%s, %s)", counter.name, instance, zone)
				test.Pass(ctx, "%s: %f\n", label, v.value)
			}

			// Send aggregated data to GCM.
			mdAgg, err := monitoring.GetMetric("service-counters-agg", projectFlag)
			if err != nil {
				return err
			}
			if err := sendAggregatedDataToGCM(ctx, s, mdAgg, agg, now, counter.name); err != nil {
				return err
			}
		}
	}
	if hasError {
		return fmt.Errorf("failed to check some counters.")
	}
	return nil
}

func checkSingleCounter(v23ctx *context.T, ctx *tool.Context, serviceName string, counter prodServiceCounter) ([]counterData, error) {
	mountedName, err := getMountedName(serviceName)
	if err != nil {
		return nil, err
	}

	// Resolve name and group results by routing ids.
	groups, err := resolveAndProcessServiceName(v23ctx, ctx, serviceName, mountedName)
	if err != nil {
		return nil, err
	}

	// Get counters for each group.
	counters := []counterData{}
	errors := []error{}
	for _, group := range groups {
		counterResult, err := getStat(v23ctx, ctx, group, counter.statSuffix)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		value, err := counterResult[0].getFloat64Value()
		if err != nil {
			errors = append(errors, err)
			continue
		}
		location, err := getServiceLocation(v23ctx, ctx, group)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		counters = append(counters, counterData{
			location: location,
			value:    value,
		})
	}
	if len(errors) == len(groups) {
		return counters, fmt.Errorf("%v", errors)
	}

	return counters, nil
}
