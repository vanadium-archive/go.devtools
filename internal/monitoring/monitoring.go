// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package monitoring

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"
	"time"

	"v.io/jiri/tool"
	"v.io/v23"
	"v.io/v23/context"
	"v.io/v23/naming"
	"v.io/v23/options"
	"v.io/v23/rpc"
	"v.io/v23/services/stats"
	"v.io/v23/vdl"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	cloudmonitoring "google.golang.org/api/monitoring/v3"
)

const (
	customMetricPrefix = "custom.googleapis.com"
	defaultTimeout     = 20 * time.Second
)

// Human-readable service names.
const (
	SNMounttable       = "mounttable"
	SNIdentity         = "identity service"
	SNMacaroon         = "macaroon service"
	SNBinaryDischarger = "binary discharger"
	SNRole             = "role service"
	SNProxy            = "proxy service"
	SNBenchmark        = "benchmark service"
	SNAllocator        = "syncbase allocator"

	hostnameStatSuffix = "__debug/stats/system/hostname"
	zoneStatSuffix     = "__debug/stats/system/gce/zone"
)

// Human-readable metric names.
const (
	MNMounttableMountedServers = "mounttable mounted servers"
	MNMounttableNodes          = "mounttable nodes"
)

// serviceMountedNames is a map from human-readable service names to their
// relative mounted names in the global mounttable.
var serviceMountedNames = map[string]string{
	SNMounttable:       "",
	SNIdentity:         "identity/dev.v.io:u",
	SNMacaroon:         "identity/dev.v.io:u/macaroon",
	SNBinaryDischarger: "identity/dev.v.io:u/discharger",
	SNRole:             "identity/role",
	SNProxy:            "proxy-mon",
	SNBenchmark:        "benchmarks",
	SNAllocator:        "syncbase-allocator",
}

// StatValue stores the name and the value returned from the GetStat function.
type StatValue struct {
	Name  string
	Value interface{}
}

func (sv *StatValue) GetStringValue() string {
	return fmt.Sprint(sv.Value)
}

func (sv *StatValue) GetFloat64Value() (float64, error) {
	switch i := sv.Value.(type) {
	case float64:
		return i, nil
	case int64:
		return float64(i), nil
	default:
		return 0, fmt.Errorf("invalid value: %v", sv.Value)
	}
}

type ServiceLocation struct {
	Instance string
	Zone     string
}

type labelData struct {
	key         string
	description string
}

var aggLabelData = []labelData{
	labelData{
		key:         "aggregation",
		description: "The aggregation type (min, max, avg, sum, count)",
	},
}

// customMetricDescriptors is a map from metric's short names to their
// MetricDescriptor definitions.
var customMetricDescriptors = map[string]*cloudmonitoring.MetricDescriptor{
	// Custom metrics for recording check latency and its aggregation
	// of vanadium production services.
	"service-latency":     createMetric("service/latency", "The check latency (ms) of vanadium production services.", "double", true, nil),
	"service-latency-agg": createMetric("service/latency-agg", "The aggregated check latency (ms) of vanadium production services.", "double", false, aggLabelData),

	// Custom metric for recording per-method rpc latency and its aggregation
	// for a service.
	"service-permethod-latency": createMetric("service/latency/method", "Service latency (ms) per method.", "double", true, []labelData{
		labelData{
			key:         "method_name",
			description: "The method name",
		},
	}),
	"service-permethod-latency-agg": createMetric("service/latency/method-agg", "Aggregated service latency (ms) per method.", "double", false, []labelData{
		labelData{
			key:         "method_name",
			description: "The method name",
		},
		aggLabelData[0],
	}),

	// Custom metric for recording various counters and their aggregations
	// of vanadium production services.
	"service-counters":     createMetric("service/counters", "Various counters of vanadium production services.", "double", true, nil),
	"service-counters-agg": createMetric("service/counters-agg", "Aggregated counters of vanadium production services.", "double", false, aggLabelData),

	// Custom metric for recording service metadata and its aggregation
	// of vanadium production services.
	"service-metadata": createMetric("service/metadata", "Various metadata of vanadium production services.", "double", true, []labelData{
		labelData{
			key:         "metadata_name",
			description: "The metadata name",
		},
	}),
	"service-metadata-agg": createMetric("service/metadata-agg", "Aggregated metadata of vanadium production services.", "double", false, []labelData{
		labelData{
			key:         "metadata_name",
			description: "The metadata name",
		},
		aggLabelData[0],
	}),

	// Custom metric for recording total rpc qps and its aggregation for a service.
	"service-qps-total":     createMetric("service/qps/total", "Total service QPS.", "double", true, nil),
	"service-qps-total-agg": createMetric("service/qps/total-agg", "Aggregated total service QPS.", "double", false, aggLabelData),

	// Custom metric for recording per-method rpc qps for a service.
	"service-qps-method": createMetric("service/qps/method", "Service QPS per method.", "double", true, []labelData{
		labelData{
			key:         "method_name",
			description: "The method name",
		},
	}),
	"service-qps-method-agg": createMetric("service/qps/method-agg", "Aggregated service QPS per method.", "double", false, []labelData{
		labelData{
			key:         "method_name",
			description: "The method name",
		},
		aggLabelData[0],
	}),

	// Custom metric for recording gce instance stats.
	"gce-instance": createMetric("gce-instance/stats", "Various stats for GCE instances.", "double", true, nil),

	// Custom metric for recording nginx stats.
	"nginx": createMetric("nginx/stats", "Various stats for Nginx server.", "double", true, nil),

	// Custom metric for rpc load tests.
	"rpc-load-test": createMetric("rpc-load-test", "Results of rpc load test.", "double", false, nil),

	// Custom metric for recording jenkins related data.
	"jenkins": createMetric("jenkins", "Jenkins related data.", "double", false, nil),
}

func createMetric(metricType, description, valueType string, includeGCELabels bool, extraLabels []labelData) *cloudmonitoring.MetricDescriptor {
	labels := []*cloudmonitoring.LabelDescriptor{}
	if includeGCELabels {
		labels = append(labels, &cloudmonitoring.LabelDescriptor{
			Key:         "gce_instance",
			Description: "The name of the GCE instance associated with this metric.",
			ValueType:   "string",
		}, &cloudmonitoring.LabelDescriptor{
			Key:         "gce_zone",
			Description: "The zone of the GCE instance associated with this metric.",
			ValueType:   "string",
		})
	}
	labels = append(labels, &cloudmonitoring.LabelDescriptor{
		Key:         "metric_name",
		Description: "The name of the metric.",
		ValueType:   "string",
	})
	if extraLabels != nil {
		for _, data := range extraLabels {
			labels = append(labels, &cloudmonitoring.LabelDescriptor{
				Key:         fmt.Sprintf("%s", data.key),
				Description: data.description,
				ValueType:   "string",
			})
		}
	}

	return &cloudmonitoring.MetricDescriptor{
		Type:        fmt.Sprintf("%s/vanadium/%s", customMetricPrefix, metricType),
		Description: description,
		MetricKind:  "gauge",
		ValueType:   valueType,
		Labels:      labels,
	}
}

// GetMetric gets the custom metric descriptor with the given name and project.
func GetMetric(name, project string) (*cloudmonitoring.MetricDescriptor, error) {
	md, ok := customMetricDescriptors[name]
	if !ok {
		return nil, fmt.Errorf("metric %q doesn't exist", name)
	}
	md.Name = fmt.Sprintf("projects/%s/metricDescriptors/%s", project, md.Type)
	return md, nil
}

// GetSortedMetricNames gets the sorted metric names.
func GetSortedMetricNames() []string {
	names := []string{}
	for n := range customMetricDescriptors {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// GetServiceMountedName gets the full mounted name for the given service.
func GetServiceMountedName(namespaceRoot, serviceName string) (string, error) {
	relativeName, ok := serviceMountedNames[serviceName]
	if !ok {
		return "", fmt.Errorf("service %q doesn't exist", serviceName)
	}
	return fmt.Sprintf("%s/%s", namespaceRoot, relativeName), nil
}

// ResolveAndProcessServiceName resolves the given service name and groups the
// result entries by their routing ids.
func ResolveAndProcessServiceName(v23ctx *context.T, ctx *tool.Context, serviceName, serviceMountedName string) (map[string]naming.MountEntry, error) {
	// Resolve the name.
	v23ctx, cancel := context.WithTimeout(v23ctx, defaultTimeout)
	defer cancel()

	ns := v23.GetNamespace(v23ctx)
	entry, err := ns.ShallowResolve(v23ctx, serviceMountedName)
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
	if serviceName == SNMounttable {
		// Mounttable resolves to itself, so we just use a dummy routing id with
		// its original mounted name.
		groups["-"] = naming.MountEntry{
			Servers: []naming.MountedServer{naming.MountedServer{Server: serviceMountedName}},
		}
	} else {
		for _, resolvedName := range resolvedNames {
			serverName, relativeName := naming.SplitAddressName(resolvedName)
			ep, err := naming.ParseEndpoint(serverName)
			if err != nil {
				return nil, err
			}
			routingId := ep.RoutingID.String()
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

// GetServiceLocation returns the given service's location (instance and zone).
// If the service is replicated, the instance name is the pod name.
//
// To make it simpler and faster, we look up service's location in hard-coded "zone maps"
// for both non-replicated and replicated services.
func GetServiceLocation(v23ctx *context.T, ctx *tool.Context, me naming.MountEntry) (*ServiceLocation, error) {
	// Check "__debug/stats/system/metadata/hostname" stat to get service's
	// host name.
	me.Name = ""
	hostnameResult, err := GetStat(v23ctx, ctx, me, hostnameStatSuffix)
	if err != nil {
		return nil, err
	}
	hostname := hostnameResult[0].GetStringValue()

	// Check "__debug/stats/system/gce/zone" stat to get service's
	// zone name.
	zoneResult, err := GetStat(v23ctx, ctx, me, zoneStatSuffix)
	if err != nil {
		return nil, err
	}
	zone := zoneResult[0].GetStringValue()
	// The zone stat exported by services is in the form of:
	// projects/632758215260/zones/us-central1-c
	// We only need the last part.
	parts := strings.Split(zone, "/")
	zone = parts[len(parts)-1]

	return &ServiceLocation{
		Instance: hostname,
		Zone:     zone,
	}, nil
}

// GetStat gets the given stat using rpc.
func GetStat(v23ctx *context.T, ctx *tool.Context, me naming.MountEntry, pattern string) ([]*StatValue, error) {
	v23ctx, cancel := context.WithTimeout(v23ctx, defaultTimeout)
	defer cancel()

	call, err := v23.GetClient(v23ctx).StartCall(v23ctx, "", rpc.GlobMethod, []interface{}{pattern}, options.Preresolved{&me})
	if err != nil {
		return nil, err
	}
	hasErrors := false
	ret := []*StatValue{}
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
				fmt.Fprintf(ctx.Stderr(), "Failed to get stat (pattern: %q, entry: %#v): %v\n%v\n", pattern, me, v.Value.Name, err)
				hasErrors = true
				continue
			}
			var convertedValue interface{}
			if err := vdl.Convert(&convertedValue, value); err != nil {
				fmt.Fprintf(ctx.Stderr(), "Failed to convert value for %v (pattern: %q, entry: %#v): %v\n", pattern, me, v.Value.Name, err)
				hasErrors = true
				continue
			}
			ret = append(ret, &StatValue{
				Name:  v.Value.Name,
				Value: convertedValue,
			})
		case naming.GlobReplyError:
			fmt.Fprintf(ctx.Stderr(), "Glob failed at %q: %v", v.Value.Name, v.Value.Error)
		}
	}
	if hasErrors || len(ret) == 0 {
		return nil, fmt.Errorf("failed to get stat (pattern: %q, entry: %#v)", pattern, me)
	}
	if err := call.Finish(); err != nil {
		return nil, err
	}

	return ret, nil
}

func createClient(keyFilePath string) (*http.Client, error) {
	if len(keyFilePath) > 0 {
		data, err := ioutil.ReadFile(keyFilePath)
		if err != nil {
			return nil, err
		}
		conf, err := google.JWTConfigFromJSON(data, cloudmonitoring.MonitoringScope)
		if err != nil {
			return nil, fmt.Errorf("failed to create JWT config file: %v", err)
		}
		return conf.Client(oauth2.NoContext), nil
	}

	return google.DefaultClient(oauth2.NoContext, cloudmonitoring.MonitoringScope)
}

// Authenticate authenticates with the given JSON credentials file (or the
// default client if the file is not provided). If successful, it returns a
// service object that can be used in GCM API calls.
func Authenticate(keyFilePath string) (*cloudmonitoring.Service, error) {
	c, err := createClient(keyFilePath)
	if err != nil {
		return nil, err
	}
	s, err := cloudmonitoring.New(c)
	if err != nil {
		return nil, fmt.Errorf("New() failed: %v", err)
	}
	return s, nil
}
