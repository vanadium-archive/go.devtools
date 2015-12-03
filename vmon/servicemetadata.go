// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"google.golang.org/api/cloudmonitoring/v2beta2"

	"v.io/jiri/runutil"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/monitoring"
)

const (
	metadataQueryString = "devmgr/apps/*/*/*/stats/system/metadata/build.Time"
)

var (
	buildTimeRE = regexp.MustCompile(`^devmgr/apps/([^/]*)/.*metadata/build\.Time: (.*)`)
)

type serviceMetadata struct {
	serviceName string
	buildTime   int64
}

// checkServiceMetadata checks all service metadata and adds the results to GCM.
func checkServiceMetadata(ctx *tool.Context) error {
	// Run "debug stats read" to get metadata from device manager.
	debug := filepath.Join(binDirFlag, "debug")
	var buf bytes.Buffer
	args := []string{
		"--v23.namespace.root",
		namespaceRootFlag,
		"--v23.credentials",
		credentialsFlag,
		"stats",
		"read",
		metadataQueryString,
	}
	if err := ctx.NewSeq().Capture(&buf, &buf).Timeout(timeout).
		Last(debug, args...); err != nil {
		if err != runutil.CommandTimedOutErr {
			return fmt.Errorf("debug command failed: %v\n%s", err, buf.String())
		}
		return err
	}

	// Parse output and add metadata to GCM.
	serviceLocation := monitoring.ServiceLocationMap[namespaceRootFlag]
	if serviceLocation == nil {
		return fmt.Errorf("service location not found for %q", namespaceRootFlag)
	}
	s, err := monitoring.Authenticate(keyFileFlag)
	if err != nil {
		return err
	}
	mdMetadata := monitoring.CustomMetricDescriptors["service-metadata"]
	now := time.Now()
	nowStr := now.Format(time.RFC3339)
	lines := strings.Split(buf.String(), "\n")
	sendTimeseriesFn := func(value float64, serviceName, metadataName string) error {
		if _, err = s.Timeseries.Write(projectFlag, &cloudmonitoring.WriteTimeseriesRequest{
			Timeseries: []*cloudmonitoring.TimeseriesPoint{
				&cloudmonitoring.TimeseriesPoint{
					Point: &cloudmonitoring.Point{
						DoubleValue: value,
						Start:       nowStr,
						End:         nowStr,
					},
					TimeseriesDesc: &cloudmonitoring.TimeseriesDescriptor{
						Metric: mdMetadata.Name,
						Labels: map[string]string{
							mdMetadata.Labels[0].Key: serviceLocation.Instance,
							mdMetadata.Labels[1].Key: serviceLocation.Zone,
							mdMetadata.Labels[2].Key: serviceName,
							mdMetadata.Labels[3].Key: metadataName,
						},
					},
				},
			},
		}).Do(); err != nil {
			return fmt.Errorf("Timeseries Write failed for service %q, metadata %q: %v", serviceName, metadataName, err)
		}
		return nil
	}
	for _, line := range lines {
		// Build time and build age (in hours).
		matches := buildTimeRE.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		serviceName := matches[1]
		strTime := matches[2]
		buildTime, err := time.Parse("2006-01-02T15:04:05Z", strTime)
		if err != nil {
			fmt.Fprintf(ctx.Stderr(), "Parse(%v) failed: %v\n", strTime, err)
			continue
		}
		if err := sendTimeseriesFn(float64(buildTime.Unix()), serviceName, "build time"); err != nil {
			fmt.Fprintf(ctx.Stderr(), "%v\n", err)
		}
		if err := sendTimeseriesFn(now.Sub(buildTime).Hours(), serviceName, "build age"); err != nil {
			fmt.Fprintf(ctx.Stderr(), "%v\n", err)
		}
	}
	return nil
}
