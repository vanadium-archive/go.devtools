// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/cloudmonitoring/v2beta2"

	"v.io/jiri/tool"
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
func checkServiceCounters(ctx *tool.Context, s *cloudmonitoring.Service) error {
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
	mdCounter := monitoring.CustomMetricDescriptors["service-counters"]
	now := time.Now().Format(time.RFC3339)
	for serviceName, serviceCounters := range counters {
		for _, counter := range serviceCounters {
			vs, err := checkSingleCounter(ctx, serviceName, counter)
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
			if err := sendAggregatedDataToGCM(ctx, s, monitoring.CustomMetricDescriptors["service-counters-agg"], agg, now, counter.name); err != nil {
				return err
			}
		}
	}
	if hasError {
		return fmt.Errorf("failed to check some counters.")
	}
	return nil
}

func checkSingleCounter(ctx *tool.Context, serviceName string, counter prodServiceCounter) ([]counterData, error) {
	mountedName, err := getMountedName(serviceName)
	if err != nil {
		return nil, err
	}

	// Resolve name and group results by routing ids.
	groups, err := resolveAndProcessServiceName(ctx, serviceName, mountedName)
	if err != nil {
		return nil, err
	}

	// Get counters for each group.
	counters := []counterData{}
	for _, group := range groups {
		value := 0.0
		availableName := group[0]
		succeeded := false
		for _, name := range group {
			if output, err := getStat(ctx, fmt.Sprintf("%s/%s", mountedName, counter.statSuffix), false); err == nil {
				parts := strings.Split(strings.TrimSpace(output), " ")
				if len(parts) != 2 {
					return nil, fmt.Errorf("invalid debug output: %s", output)
				}
				v, err := strconv.ParseFloat(parts[1], 64)
				if err != nil {
					return nil, fmt.Errorf("ParseFloat(%s) failed: %v", parts[1], err)
				}
				availableName = name
				value = v
				succeeded = true
			}
		}
		if !succeeded {
			return nil, fmt.Errorf("failed to check service %q", serviceName)
		}
		location, err := getServiceLocation(ctx, availableName, serviceName)
		if err != nil {
			return nil, err
		}
		counters = append(counters, counterData{
			location: location,
			value:    value,
		})
	}

	return counters, nil
}
