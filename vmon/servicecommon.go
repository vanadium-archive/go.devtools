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

	"google.golang.org/api/cloudmonitoring/v2beta2"

	"v.io/jiri/runutil"
	"v.io/jiri/tool"
	"v.io/v23"
	"v.io/v23/naming"
	"v.io/x/devtools/internal/monitoring"
)

// Human-readable service names.
const (
	snMounttable       = "mounttable"
	snApplications     = "application repository"
	snBinaries         = "binary repository"
	snIdentity         = "identity service"
	snMacaroon         = "macaroon service"
	snGoogleIdentity   = "google identity service"
	snBinaryDischarger = "binary discharger"
	snRole             = "role service"
	snProxy            = "proxy service"
	snGroups           = "groups service"

	defaultZone        = "us-central1-c"
	hostnameStatSuffix = "__debug/stats/system/hostname"
)

var (
	hostnameRE = regexp.MustCompile(`^.*/hostname: (.*)`)
)

// serviceMountedNames is a map from human-readable service names to their
// relative mounted names in the global mounttable.
var serviceMountedNames = map[string]string{
	snMounttable:       "",
	snApplications:     "applications",
	snBinaries:         "binaries",
	snIdentity:         "identity/dev.v.io:u",
	snMacaroon:         "identity/dev.v.io:u/macaroon",
	snGoogleIdentity:   "identity/dev.v.io:u/google",
	snBinaryDischarger: "identity/dev.v.io:u/discharger",
	snRole:             "identity/role",
	snProxy:            "proxy-mon",
	snGroups:           "groups",
}

// serviceZones records the zones of all services.
//
// It is a two-level map.
// The key of the first level is the project name.
// The key of the second level is the service name, and the corresponding value
// is the zone name.
//
// If a service is not found in the corresponding project map, it is located at
// the default zone (see "defaultZone" variable above).
//
// TODO(jingjin): make service export its project and zone, just like the
// hostname.
var serviceZones = map[string]map[string]string{
	"vanadium-production": map[string]string{},
	"vanadium-staging":    map[string]string{},
}

func getMountedName(serviceName string) (string, error) {
	relativeName, ok := serviceMountedNames[serviceName]
	if !ok {
		return "", fmt.Errorf("service name %q not found", serviceName)
	}
	return fmt.Sprintf("%s/%s", namespaceRootFlag, relativeName), nil
}

// getStat runs "debug stats read" command for the given stat.
func getStat(ctx *tool.Context, stat string, json bool) (string, error) {
	// TODO(jingjin): use RPC instead of the debug command.
	debug := filepath.Join(binDirFlag, "debug")
	args := []string{
		"--v23.credentials",
		credentialsFlag,
		"stats",
		"read",
	}
	if json {
		args = append(args, "-json")
	}
	args = append(args, stat)
	var stdoutBuf, stderrBuf bytes.Buffer
	if err := ctx.NewSeq().Capture(&stdoutBuf, &stderrBuf).Timeout(timeout).
		Last(debug, args...); err != nil {
		if !runutil.IsTimeout(err) {
			return "", fmt.Errorf("debug command failed: %v\nSTDOUT:\n%s\nSTDERR:\n:%s", err, stdoutBuf.String(), stderrBuf.String())
		}
		return "", err
	}
	if stdoutBuf.Len() == 0 {
		return "", fmt.Errorf("debug command returned no output. STDERR:\n%s", stderrBuf.String())
	}
	return stdoutBuf.String(), nil
}

// resolveAndProcessServiceName resolves the given service name and groups the
// result entries by their routing ids.
func resolveAndProcessServiceName(ctx *tool.Context, serviceName, serviceMountedName string) (map[string][]string, error) {
	s := ctx.NewSeq()

	// Resolve the name.
	// TODO(jingjin): use RPC instead of the namespace command.
	namespace := filepath.Join(binDirFlag, "namespace")
	var bufOut, bufErr bytes.Buffer
	if err := s.Capture(&bufOut, &bufErr).Timeout(timeout).
		Last(namespace, "resolve", serviceMountedName); err != nil {
		return nil, fmt.Errorf("%v: %s", err, bufErr.String())
	}
	resolvedNames := strings.Split(strings.TrimSpace(bufOut.String()), "\n")

	// Group resolved names by their routing ids.
	groups := map[string][]string{}
	if serviceName == snMounttable {
		// Mounttable resolves to itself, so we just use a dummy routing id with
		// its original mounted name.
		groups["-"] = []string{serviceMountedName}
	} else {
		for _, resolvedName := range resolvedNames {
			ep, err := v23.NewEndpoint(resolvedName)
			if err != nil {
				return nil, err
			}
			routingId := ep.RoutingID().String()
			groups[routingId] = append(groups[routingId], resolvedName)
		}
	}

	return groups, nil
}

// getServiceLocation returns the given service's location (instance and zone).
// If the service is replicated, the instance name is the pod name.
//
// To make it simpler and faster, we look up service's location in hard-coded "zone maps"
// for both non-replicated and replicated services.
func getServiceLocation(ctx *tool.Context, name, serviceName string) (*monitoring.ServiceLocation, error) {
	// Check "__debug/stats/system/metadata/hostname" stat to get service's
	// host name.
	serverName, _ := naming.SplitAddressName(name)
	hostnameStat := fmt.Sprintf("%s/%s", serverName, hostnameStatSuffix)
	output, err := getStat(ctx, hostnameStat, false)
	if err != nil {
		return nil, err
	}
	matches := hostnameRE.FindStringSubmatch(output)
	if matches == nil {
		return nil, fmt.Errorf("invalid stat: %s", output)
	}
	hostname := matches[1]

	// Look up zone from serviceZones map.
	zone := defaultZone
	if _, ok := serviceZones[projectFlag]; !ok {
		return nil, fmt.Errorf("invalid project: %s", projectFlag)
	}
	z, ok := serviceZones[projectFlag][serviceName]
	if ok {
		zone = z
	}

	return &monitoring.ServiceLocation{
		Instance: hostname,
		Zone:     zone,
	}, nil
}

// sendDataToGCM sends the given metric to Google Cloud Monitoring.
func sendDataToGCM(s *cloudmonitoring.Service, md *cloudmonitoring.MetricDescriptor, value float64, now, instance, zone string, extraLabelKeys ...string) error {
	labels := []string{instance, zone}
	for _, key := range extraLabelKeys {
		labels = append(labels, key)
	}
	if len(labels) != len(md.Labels) {
		return fmt.Errorf("wrong number of label keys: want %d, got %d", len(md.Labels), len(labels))
	}
	labelsMap := map[string]string{}
	for i := range labels {
		labelsMap[md.Labels[i].Key] = labels[i]
	}
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
					Labels: labelsMap,
				},
			},
		},
	}).Do(); err != nil {
		return fmt.Errorf("Timeseries Write failed for metric %q with value %q: %v", md.Name, value, err)
	}
	return nil
}
