// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"regexp"
	"time"

	"google.golang.org/api/cloudmonitoring/v2beta2"

	"v.io/jiri/tool"
	"v.io/x/devtools/internal/monitoring"
	"v.io/x/devtools/internal/test"
)

const (
	buildTimeStatSuffix = "__debug/stats/system/metadata/build.Time"
)

var (
	buildTimeRE = regexp.MustCompile(`^.*/build\.Time: (.*)`)
)

type metadataData struct {
	location  *monitoring.ServiceLocation
	buildTime time.Time
}

// checkServiceMetadata checks all service metadata and adds the results to GCM.
func checkServiceMetadata(ctx *tool.Context, s *cloudmonitoring.Service) error {
	serviceNames := []string{
		snMounttable,
		snApplications,
		snBinaries,
		snIdentity,
		snGroups,
		snRole,
		snProxy,
	}

	hasError := false
	mdMetadata := monitoring.CustomMetricDescriptors["service-metadata"]
	now := time.Now()
	strNow := now.Format(time.RFC3339)
	for _, serviceName := range serviceNames {
		ms, err := checkSingleServiceMetadata(ctx, serviceName)
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
		mdMetadataAgg := monitoring.CustomMetricDescriptors["service-metadata-agg"]
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

func checkSingleServiceMetadata(ctx *tool.Context, serviceName string) ([]metadataData, error) {
	mountedName, err := getMountedName(serviceName)
	if err != nil {
		return nil, err
	}

	// Resolve name and group results by routing ids.
	groups, err := resolveAndProcessServiceName(ctx, serviceName, mountedName)
	if err != nil {
		return nil, err
	}

	// Get metadata for each group.
	metadata := []metadataData{}
	for _, group := range groups {
		buildTime := time.Time{}
		availableName := group[0]
		for _, name := range group {
			// Query build time.
			buildTimeStat := fmt.Sprintf("%s/%s", mountedName, buildTimeStatSuffix)
			if output, err := getStat(ctx, buildTimeStat, false); err == nil {
				// Parse build time.
				matches := buildTimeRE.FindStringSubmatch(output)
				if matches == nil {
					return nil, fmt.Errorf("invalid stat: %s", output)
				}
				strTime := matches[1]
				t, err := time.Parse("2006-01-02T15:04:05Z", strTime)
				if err != nil {
					return nil, fmt.Errorf("Parse(%v) failed: %v", strTime, err)
				}
				buildTime = t
				availableName = name
			}
		}
		if buildTime.IsZero() {
			return nil, fmt.Errorf("failed to check build time for service %q", serviceName)
		}
		location, err := getServiceLocation(ctx, availableName, serviceName)
		if err != nil {
			return nil, err
		}
		metadata = append(metadata, metadataData{
			location:  location,
			buildTime: buildTime,
		})
	}

	return metadata, nil
}
