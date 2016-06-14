// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package monitoring

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
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
	defaultTimeout = 20 * time.Second
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
	SNMounttable:       "mt",
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
	case uint64:
		return float64(i), nil
	default:
		return 0, fmt.Errorf("invalid value: %v, %T", sv.Value, sv.Value)
	}
}

type ServiceLocation struct {
	Instance string
	Zone     string
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
