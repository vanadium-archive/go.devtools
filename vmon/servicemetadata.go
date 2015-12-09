// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"time"

	"google.golang.org/api/cloudmonitoring/v2beta2"

	"v.io/jiri/runutil"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/monitoring"
)

const (
	buildTimeStatSuffix = "__debug/stats/system/metadata/build.Time"
)

var (
	buildTimeRE = regexp.MustCompile(`^.*/build\.Time: (.*)`)
)

// checkServiceMetadata checks all service metadata and adds the results to GCM.
func checkServiceMetadata(ctx *tool.Context) error {
	serviceNames := []string{
		snMounttable,
		snApplications,
		snBinaries,
		snIdentity,
		snGroups,
		snProxy,
	}

	// Run "debug stats read" to get metadata from each service.
	now := time.Now()
	strNow := now.Format(time.RFC3339)
	// TODO(jingjin): get location by reading stats/system/hostname.
	serviceLocation := monitoring.ServiceLocationMap[namespaceRootFlag]
	if serviceLocation == nil {
		return fmt.Errorf("service location not found for %q", namespaceRootFlag)
	}
	s, err := monitoring.Authenticate(keyFileFlag)
	if err != nil {
		return err
	}
	mdMetadata := monitoring.CustomMetricDescriptors["service-metadata"]
	debug := filepath.Join(binDirFlag, "debug")
	for _, serviceName := range serviceNames {
		serviceMountedName, err := getMountedName(serviceName)
		if err != nil {
			return err
		}

		// Query build time.
		buildTimeStat := fmt.Sprintf("%s/%s", serviceMountedName, buildTimeStatSuffix)
		var stdoutBuf, stderrBuf bytes.Buffer
		args := []string{
			"--v23.credentials",
			credentialsFlag,
			"stats",
			"read",
			buildTimeStat,
		}
		if err := ctx.NewSeq().Capture(&stdoutBuf, &stderrBuf).Timeout(timeout).
			Last(debug, args...); err != nil {
			if !runutil.IsTimeout(err) {
				return fmt.Errorf("debug command failed: %v\nSTDOUT:\n%s\nSTDERR:\n:%s", err, stdoutBuf.String(), stderrBuf.String())
			}
			return err
		}
		if stdoutBuf.Len() == 0 {
			return fmt.Errorf("debug command returned no output. STDERR:\n%s", stderrBuf.String())
		}

		// Parse build time.
		output := stdoutBuf.String()
		matches := buildTimeRE.FindStringSubmatch(output)
		if matches == nil {
			return fmt.Errorf("invalid stat: %s", output)
		}
		strTime := matches[1]
		buildTime, err := time.Parse("2006-01-02T15:04:05Z", strTime)
		if err != nil {
			return fmt.Errorf("Parse(%v) failed: %v", strTime, err)
		}

		// Send to GCM.
		if err := sendMetadataToGCM(mdMetadata, float64(buildTime.Unix()), strNow, serviceLocation.Instance, serviceLocation.Zone, serviceName, "build time", s); err != nil {
			return err
		}
		if err := sendMetadataToGCM(mdMetadata, now.Sub(buildTime).Hours(), strNow, serviceLocation.Instance, serviceLocation.Zone, serviceName, "build age", s); err != nil {
			return err
		}
	}
	return nil
}

func sendMetadataToGCM(md *cloudmonitoring.MetricDescriptor, value float64, now, instance, zone, serviceName, metadataName string, s *cloudmonitoring.Service) error {
	if _, err := s.Timeseries.Write(projectFlag, &cloudmonitoring.WriteTimeseriesRequest{
		Timeseries: []*cloudmonitoring.TimeseriesPoint{
			&cloudmonitoring.TimeseriesPoint{
				Point: &cloudmonitoring.Point{
					DoubleValue: value,
					Start:       now,
					End:         now,
				},
				TimeseriesDesc: &cloudmonitoring.TimeseriesDescriptor{
					Metric: md.Name,
					Labels: map[string]string{
						md.Labels[0].Key: instance,
						md.Labels[1].Key: zone,
						md.Labels[2].Key: serviceName,
						md.Labels[3].Key: metadataName,
					},
				},
			},
		},
	}).Do(); err != nil {
		return fmt.Errorf("Timeseries Write failed for service %q, metadata %q: %v", serviceName, metadataName, err)
	}
	return nil
}
