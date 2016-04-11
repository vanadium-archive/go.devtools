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

const (
	buildTimeStatSuffix = "__debug/stats/system/metadata/build.Time"
)

type metadataData struct {
	location  *monitoring.ServiceLocation
	buildTime time.Time
}

// checkServiceMetadata checks all service metadata and adds the results to GCM.
func checkServiceMetadata(v23ctx *context.T, ctx *tool.Context, s *cloudmonitoring.Service) error {
	serviceNames := []string{
		snMounttable,
		snIdentity,
		snRole,
		snProxy,
	}

	hasError := false
	mdMetadata, err := monitoring.GetMetric("service-metadata", projectFlag)
	if err != nil {
		return err
	}
	now := time.Now()
	strNow := now.UTC().Format(time.RFC3339)
	for _, serviceName := range serviceNames {
		ms, err := checkSingleServiceMetadata(v23ctx, ctx, serviceName)
		if err != nil {
			test.Fail(ctx, "%s\n", serviceName)
			fmt.Fprintf(ctx.Stderr(), "%v\n", err)
			hasError = true
			continue
		}
		aggBuildTime := newAggregator()
		aggBuildAge := newAggregator()
		for _, m := range ms {
			buildTimeUnix := m.buildTime.Unix()
			buildAgeInHours := now.Sub(m.buildTime).Hours()

			instance := m.location.Instance
			zone := m.location.Zone
			aggBuildTime.add(float64(buildTimeUnix))
			aggBuildAge.add(buildAgeInHours)

			// Send data to GCM
			if err := sendDataToGCM(s, mdMetadata, float64(buildTimeUnix), strNow, instance, zone, serviceName, "build time"); err != nil {
				return err
			}
			if err := sendDataToGCM(s, mdMetadata, buildAgeInHours, strNow, instance, zone, serviceName, "build age"); err != nil {
				return err
			}

			label := fmt.Sprintf("%s (%s, %s)", serviceName, instance, zone)
			test.Pass(ctx, "%s: build time: %d, build age: %v\n", label, buildTimeUnix, buildAgeInHours)
		}

		// Send aggregated data to GCM.
		mdMetadataAgg, err := monitoring.GetMetric("service-metadata-agg", projectFlag)
		if err != nil {
			return err
		}
		if err := sendAggregatedDataToGCM(ctx, s, mdMetadataAgg, aggBuildTime, strNow, serviceName, "build time"); err != nil {
			return err
		}
		if err := sendAggregatedDataToGCM(ctx, s, mdMetadataAgg, aggBuildAge, strNow, serviceName, "build age"); err != nil {
			return err
		}
	}
	if hasError {
		return fmt.Errorf("failed to check metadata for some services.")
	}

	return nil
}

func checkSingleServiceMetadata(v23ctx *context.T, ctx *tool.Context, serviceName string) ([]metadataData, error) {
	mountedName, err := getMountedName(serviceName)
	if err != nil {
		return nil, err
	}

	// Resolve name and group results by routing ids.
	groups, err := resolveAndProcessServiceName(v23ctx, ctx, serviceName, mountedName)
	if err != nil {
		return nil, err
	}

	// Get metadata for each group.
	metadata := []metadataData{}
	errors := []error{}
	for _, group := range groups {
		// Query build time.
		timeResult, err := getStat(v23ctx, ctx, group, buildTimeStatSuffix)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		strTime := timeResult[0].getStringValue()
		buildTime, err := time.Parse("2006-01-02T15:04:05Z", strTime)
		if err != nil {
			errors = append(errors, fmt.Errorf("Parse(%v) failed: %v", strTime, err))
			continue
		}
		location, err := getServiceLocation(v23ctx, ctx, group)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		metadata = append(metadata, metadataData{
			location:  location,
			buildTime: buildTime,
		})
	}
	if len(errors) == len(groups) {
		return nil, fmt.Errorf("%v", errors)
	}

	return metadata, nil
}
