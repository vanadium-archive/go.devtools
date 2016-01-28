// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"math"
	"strings"

	"google.golang.org/api/cloudmonitoring/v2beta2"

	"v.io/jiri/tool"
	"v.io/v23"
	"v.io/v23/context"
	"v.io/v23/naming"
	"v.io/v23/options"
	"v.io/v23/rpc"
	"v.io/v23/services/stats"
	"v.io/v23/vdl"
	"v.io/x/devtools/internal/monitoring"
	"v.io/x/devtools/internal/test"
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

	hostnameStatSuffix = "__debug/stats/system/hostname"
	zoneStatSuffix     = "__debug/stats/system/gce/zone"
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

type aggregator struct {
	data []float64
	min  float64
	max  float64
	sum  float64
}

func newAggregator() *aggregator {
	return &aggregator{
		data: []float64{},
		min:  math.MaxFloat64,
	}
}

func (a *aggregator) add(v float64) {
	a.data = append(a.data, v)
	a.min = math.Min(a.min, v)
	a.max = math.Max(a.max, v)
	a.sum += v
}

func (a *aggregator) avg() float64 {
	return a.sum / float64(len(a.data))
}

func (a *aggregator) count() float64 {
	return float64(len(a.data))
}

func (a *aggregator) String() string {
	return fmt.Sprintf("min: %f, max: %f, avg: %f", a.min, a.max, a.avg())
}

type statValue struct {
	name  string
	value interface{}
}

func (sv *statValue) getStringValue() string {
	return fmt.Sprint(sv.value)
}

func (sv *statValue) getFloat64Value() (float64, error) {
	switch i := sv.value.(type) {
	case float64:
		return i, nil
	case int64:
		return float64(i), nil
	default:
		return 0, fmt.Errorf("invalid value: %v", sv.value)
	}
}

func getMountedName(serviceName string) (string, error) {
	relativeName, ok := serviceMountedNames[serviceName]
	if !ok {
		return "", fmt.Errorf("service name %q not found", serviceName)
	}
	return fmt.Sprintf("%s/%s", namespaceRootFlag, relativeName), nil
}

// getStat gets the given stat using rpc.
func getStat(v23ctx *context.T, ctx *tool.Context, me naming.MountEntry, pattern string) ([]*statValue, error) {
	v23ctx, cancel := context.WithTimeout(v23ctx, timeout)
	defer cancel()

	call, err := v23.GetClient(v23ctx).StartCall(v23ctx, "", rpc.GlobMethod, []interface{}{pattern}, options.Preresolved{&me})
	if err != nil {
		return nil, err
	}
	hasErrors := false
	ret := []*statValue{}
	mountEntryName := me.Name
	for {
		var gr naming.GlobReply
		err := call.Recv(&gr)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch v := gr.(type) {
		case naming.GlobReplyEntry:
			me.Name = naming.Join(mountEntryName, v.Value.Name)
			value, err := stats.StatsClient("").Value(v23ctx, options.Preresolved{&me})
			if err != nil {
				fmt.Fprintf(ctx.Stderr(), "Failed to get stat: %v\n%v\n", v.Value.Name, err)
				hasErrors = true
				continue
			}
			var convertedValue interface{}
			if err := vdl.Convert(&convertedValue, value); err != nil {
				fmt.Fprintf(ctx.Stderr(), "Failed to convert value for %v: %v\n", v.Value.Name, err)
				hasErrors = true
				continue
			}
			ret = append(ret, &statValue{
				name:  v.Value.Name,
				value: convertedValue,
			})
		case naming.GlobReplyError:
			fmt.Fprintf(ctx.Stderr(), "Glob failed at %q: %v", v.Value.Name, v.Value.Error)
		}
	}
	if hasErrors || len(ret) == 0 {
		return nil, fmt.Errorf("failed to get stat")
	}
	if err := call.Finish(); err != nil {
		return nil, err
	}

	return ret, nil
}

// resolveAndProcessServiceName resolves the given service name and groups the
// result entries by their routing ids.
func resolveAndProcessServiceName(v23ctx *context.T, ctx *tool.Context, serviceName, serviceMountedName string) (map[string]naming.MountEntry, error) {
	// Resolve the name.
	v23ctx, cancel := context.WithTimeout(v23ctx, timeout)
	defer cancel()

	ns := v23.GetNamespace(v23ctx)
	entry, err := ns.Resolve(v23ctx, serviceMountedName)
	if err != nil {
		return nil, err
	}
	resolvedNames := []string{}
	for _, server := range entry.Servers {
		fullName := naming.JoinAddressName(server.Server, entry.Name)
		resolvedNames = append(resolvedNames, fullName)
	}

	// Group resolved names by their routing ids.
	groups := map[string]naming.MountEntry{}
	if serviceName == snMounttable {
		// Mounttable resolves to itself, so we just use a dummy routing id with
		// its original mounted name.
		groups["-"] = naming.MountEntry{
			Servers: []naming.MountedServer{naming.MountedServer{Server: serviceMountedName}},
		}
	} else {
		for _, resolvedName := range resolvedNames {
			serverName, relativeName := naming.SplitAddressName(resolvedName)
			ep, err := v23.NewEndpoint(serverName)
			if err != nil {
				return nil, err
			}
			routingId := ep.RoutingID().String()
			if _, ok := groups[routingId]; !ok {
				groups[routingId] = naming.MountEntry{}
			}
			curMountEntry := groups[routingId]
			curMountEntry.Servers = append(curMountEntry.Servers, naming.MountedServer{Server: serverName})
			// resolvedNames are resolved from the same service so they should have
			// the same relative name.
			curMountEntry.Name = relativeName
			groups[routingId] = curMountEntry
		}
	}

	return groups, nil
}

// getServiceLocation returns the given service's location (instance and zone).
// If the service is replicated, the instance name is the pod name.
//
// To make it simpler and faster, we look up service's location in hard-coded "zone maps"
// for both non-replicated and replicated services.
func getServiceLocation(v23ctx *context.T, ctx *tool.Context, me naming.MountEntry) (*monitoring.ServiceLocation, error) {
	// Check "__debug/stats/system/metadata/hostname" stat to get service's
	// host name.
	me.Name = ""
	hostnameResult, err := getStat(v23ctx, ctx, me, hostnameStatSuffix)
	if err != nil {
		return nil, err
	}
	hostname := hostnameResult[0].getStringValue()

	// Check "__debug/stats/system/gce/zone" stat to get service's
	// zone name.
	zoneResult, err := getStat(v23ctx, ctx, me, zoneStatSuffix)
	if err != nil {
		return nil, err
	}
	zone := zoneResult[0].getStringValue()
	// The zone stat exported by services is in the form of:
	// projects/632758215260/zones/us-central1-c
	// We only need the last part.
	parts := strings.Split(zone, "/")
	zone = parts[len(parts)-1]

	return &monitoring.ServiceLocation{
		Instance: hostname,
		Zone:     zone,
	}, nil
}

// sendDataToGCM sends the given metric to Google Cloud Monitoring.
func sendDataToGCM(s *cloudmonitoring.Service, md *cloudmonitoring.MetricDescriptor, value float64, now, instance, zone string, extraLabelKeys ...string) error {
	// Sending value 0 will cause error.
	if math.Abs(value) < 1e-7 {
		return nil
	}

	labels := []string{}
	if instance != "" {
		labels = append(labels, instance)
	}
	if zone != "" {
		labels = append(labels, zone)
	}
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

func sendAggregatedDataToGCM(ctx *tool.Context, s *cloudmonitoring.Service, md *cloudmonitoring.MetricDescriptor, agg *aggregator, now string, extraLabelKeys ...string) error {
	labels := []string{}
	for _, l := range extraLabelKeys {
		labels = append(labels, l)
	}
	minLabels := append(labels, "min")
	if err := sendDataToGCM(s, md, agg.min, now, "", "", minLabels...); err != nil {
		return err
	}
	maxLabels := append(labels, "max")
	if err := sendDataToGCM(s, md, agg.max, now, "", "", maxLabels...); err != nil {
		return err
	}
	avgLabels := append(labels, "avg")
	if err := sendDataToGCM(s, md, agg.avg(), now, "", "", avgLabels...); err != nil {
		return err
	}
	sumLabels := append(labels, "sum")
	if err := sendDataToGCM(s, md, agg.sum, now, "", "", sumLabels...); err != nil {
		return err
	}
	countLabels := append(labels, "count")
	if err := sendDataToGCM(s, md, agg.count(), now, "", "", countLabels...); err != nil {
		return err
	}
	test.Pass(ctx, "%s: %s\n", strings.Join(extraLabelKeys, " "), agg)
	return nil
}
