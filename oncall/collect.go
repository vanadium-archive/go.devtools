// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	cloudmonitoring "google.golang.org/api/cloudmonitoring/v2beta2"

	"v.io/x/devtools/internal/monitoring"
	"v.io/x/devtools/internal/tool"
	"v.io/x/lib/cmdline"
)

const (
	cloudServiceLatencyMetric  = "custom.cloudmonitoring.googleapis.com/vanadium/service/latency"
	cloudServiceCountersMetric = "custom.cloudmonitoring.googleapis.com/vanadium/service/counters"
	cloudServiceQPSMetric      = "custom.cloudmonitoring.googleapis.com/vanadium/service/qps/total"
	nginxStatsMetric           = "custom.cloudmonitoring.googleapis.com/vanadium/nginx/stats"
	gceStatsMetric             = "custom.cloudmonitoring.googleapis.com/vanadium/gce-instance/stats"
	metricNameLabelKey         = "custom.cloudmonitoring.googleapis.com/metric-name"
	gceInstanceLabelKey        = "custom.cloudmonitoring.googleapis.com/gce-instance"
	gceZoneLabelKey            = "custom.cloudmonitoring.googleapis.com/gce-zone"
	historyDuration            = "1h"
	serviceStatusOK            = "serviceStatusOK"
	serviceStatusWarning       = "serviceStatusWarning"
	serviceStatusDown          = "serviceStatusDown"
	warningLatency             = 2000
	criticalLatency            = 5000
)

const (
	thresholdHoldMinutes = 5

	thresholdCPU            = 90
	thresholdDisk           = 85
	thresholdMounttableQPS  = 150
	thresholdPing           = 500
	thresholdRam            = 90
	thresholdServiceLatency = 2000.0
	thresholdTCPConn        = 200

	buildInfoEndpointPrefix = "devmgr/apps/*/*/*/stats/system/metadata"
	namespaceRoot           = "/ns.dev.v.io:8151"
)

var (
	binDirFlag          string
	credentialsFlag     string
	keyFileFlag         string
	projectFlag         string
	serviceAccountFlag  string
	debugCommandTimeout = time.Second * 5
	buildInfoRE         = regexp.MustCompile(`devmgr/apps/([^/]*)/.*/stats/system/metadata/build.(Pristine|Time|User|Manifest):\s*(.*)`)
	manifestRE          = regexp.MustCompile(`.*label="(.*)">`)
)

type oncallData struct {
	CollectionTimestamp int64
	Zones               map[string]*zoneData
}

type zoneData struct {
	CloudServices *cloudServiceData
	Nginx         *nginxData
	GCE           *gceInstanceData
}

type cloudServiceData struct {
	ZoneName  string
	Latency   metricsMap
	Stats     metricsMap
	BuildInfo buildInfoMap
}

type nginxData struct {
	ZoneName string
	Load     metricsMap
}

type gceInstanceData struct {
	ZoneName string
	GCEInfo  map[string]gceInfoData
	Stats    metricsMap
}

type metricData struct {
	ZoneName          string
	InstanceName      string
	Name              string
	CurrentValue      float64
	MinTime           int64
	MaxTime           int64
	MinValue          float64
	MaxValue          float64
	HistoryTimestamps []int64
	HistoryValues     []float64
	Threshold         float64
	Healthy           bool
}

// metricsMap stores metricData slices indexed by GCE instance.
type metricsMap map[string][]*metricData

type buildInfoData struct {
	ZoneName     string
	InstanceName string
	ServiceName  string
	IsPristine   string
	Snapshot     string
	Time         string
	User         string
}

// buildInfoMap stores bulidInfoData slices indexed by GCE instance.
type buildInfoMap map[string][]*buildInfoData

type gceInfoData struct {
	Status string
	Id     string
}

type serviceStatusData struct {
	CollectionTimestamp int64
	Status              []statusData
}

type statusData struct {
	Name           string
	BuildTimestamp string
	SnapshotLabel  string
	CurrentStatus  string
	Incidents      []incidentData
}

type incidentData struct {
	Start    int64
	Duration int64
	Status   string
}

func init() {
	cmdCollect.Flags.StringVar(&binDirFlag, "bin-dir", "", "The path where all binaries are downloaded.")
	cmdCollect.Flags.StringVar(&keyFileFlag, "key", "", "The path to the service account's key file.")
	cmdCollect.Flags.StringVar(&projectFlag, "project", "", "The GCM's corresponding GCE project ID.")
	cmdCollect.Flags.StringVar(&serviceAccountFlag, "account", "", "The service account used to communicate with GCM.")
	cmdCollect.Flags.StringVar(&credentialsFlag, "v23.credentials", "", "The path to v23 credentials.")
}

// cmdCollect represents the 'collect' command of the oncall tool.
var cmdCollect = &cmdline.Command{
	Name:  "collect",
	Short: "Collect data for oncall dashboard",
	Long: `
This subcommand collects data from Google Cloud Monitoring and stores the
processed data to Google Storage.
`,
	Runner: cmdline.RunnerFunc(runCollect),
}

func runCollect(env *cmdline.Env, _ []string) error {
	ctx := tool.NewContextFromEnv(env, tool.ContextOpts{
		Color:   &colorFlag,
		Verbose: &verboseFlag,
		DryRun:  &dryrunFlag,
	})

	s, err := monitoring.Authenticate(serviceAccountFlag, keyFileFlag)
	if err != nil {
		return err
	}
	now := time.Now()

	// Collect oncall related data used in the internal oncall dashboard.
	zones := map[string]*zoneData{}
	if err := collectCloudServicesData(ctx, s, now, zones); err != nil {
		return err
	}
	if err := collectCloudServicesBuildInfo(ctx, zones); err != nil {
		return err
	}
	if err := collectNginxData(ctx, s, now, zones); err != nil {
		return err
	}
	if err := collectGCEInstancesData(ctx, s, now, zones); err != nil {
		return err
	}
	oncall := &oncallData{
		CollectionTimestamp: now.Unix(),
		Zones:               zones,
	}

	// Collect service status data used in the external dashboard.
	buildInfo := zones["us-central1-c"].CloudServices.BuildInfo["vanadium-cell-master"]
	statusData, err := collectServiceStatusData(ctx, s, now, buildInfo)
	if err != nil {
		return err
	}

	if err := persistOncallData(ctx, statusData, oncall, now); err != nil {
		return err
	}

	return nil
}

func collectServiceStatusData(ctx *tool.Context, s *cloudmonitoring.Service, now time.Time, buildInfo []*buildInfoData) (*serviceStatusData, error) {
	// Collect data for the last 8 days and aggregate data every 10 minutes.
	resp, err := s.Timeseries.List(projectFlag, cloudServiceLatencyMetric, now.Format(time.RFC3339), &cloudmonitoring.ListTimeseriesRequest{
		Kind: "cloudmonitoring#listTimeseriesRequest",
	}).Window("10m").Timespan("8d").Aggregator("max").Do()
	if err != nil {
		return nil, fmt.Errorf("List failed: %v", err)
	}

	buildInfoByServiceName := map[string]*buildInfoData{}
	for _, curBuildInfo := range buildInfo {
		buildInfoByServiceName[curBuildInfo.ServiceName] = curBuildInfo
	}

	status := []statusData{}
	for _, t := range resp.Timeseries {
		serviceName := t.TimeseriesDesc.Labels[metricNameLabelKey]
		curStatusData := statusData{
			Name:          serviceName,
			CurrentStatus: statusForLatency(t.Points[0].DoubleValue), // t.Points[0] is the latest
		}
		incidents, err := calcIncidents(t.Points)
		if err != nil {
			return nil, err
		}
		curStatusData.Incidents = incidents
		buildInfoServiceName := serviceName
		if serviceName == "binary discharger" || serviceName == "google identity service" || serviceName == "macaroon service" {
			buildInfoServiceName = "identityd"
		} else if serviceName == "mounttable" {
			buildInfoServiceName = "mounttabled"
		} else if serviceName == "proxy service" {
			buildInfoServiceName = "proxyd"
		} else if serviceName == "binary repository" {
			buildInfoServiceName = "binaryd"
		} else if serviceName == "application repository" {
			buildInfoServiceName = "applicationd"
		}
		curBuildInfo := buildInfoByServiceName[buildInfoServiceName]
		if curBuildInfo != nil {
			ts, err := strconv.ParseInt(curBuildInfo.Time, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("ParseInt(%s) failed: %v", curBuildInfo.Time, err)
			}
			curStatusData.BuildTimestamp = time.Unix(ts, 0).Format(time.RFC822)
			curStatusData.SnapshotLabel = curBuildInfo.Snapshot
		}
		status = append(status, curStatusData)
	}
	return &serviceStatusData{
		CollectionTimestamp: now.Unix(),
		Status:              status,
	}, nil
}

func calcIncidents(points []*cloudmonitoring.Point) ([]incidentData, error) {
	lastStatus := serviceStatusOK
	incidents := []incidentData{}
	var curIncident incidentData
	// "points" are sorted from now to past. To calculate incidents, we iterate
	// through them backwards.
	for i := len(points) - 1; i >= 0; i-- {
		point := points[i]
		value := point.DoubleValue
		curStatus := statusForLatency(value)
		if curStatus != lastStatus {
			pointTime, err := time.Parse(time.RFC3339, point.Start)
			if err != nil {
				return nil, fmt.Errorf("time.Parse(%s) failed: %v", point.Start, err)
			}

			// Set the duration of the last incident.
			if curIncident.Status != "" {
				curIncident.Duration = pointTime.Unix() - curIncident.Start
				incidents = append(incidents, curIncident)
				curIncident.Status = ""
			}

			// At the start of an incident, create a new incidentData object, and
			// record the incident start time and status.
			if curStatus != serviceStatusOK {
				curIncident = incidentData{}
				curIncident.Start = pointTime.Unix()
				curIncident.Status = curStatus
			}
			lastStatus = curStatus
		}
	}
	// Process the possible last incident.
	if lastStatus != serviceStatusOK {
		strLastPointTime := points[0].Start
		pointTime, err := time.Parse(time.RFC3339, strLastPointTime)
		if err != nil {
			return nil, fmt.Errorf("time.Parse(%q) failed: %v", strLastPointTime, err)
		}
		curIncident.Duration = pointTime.Unix() - curIncident.Start
		incidents = append(incidents, curIncident)
	}
	return incidents, nil
}

func statusForLatency(latency float64) string {
	if latency < warningLatency {
		return serviceStatusOK
	}
	if latency < criticalLatency {
		return serviceStatusWarning
	}
	return serviceStatusDown
}

func collectCloudServicesData(ctx *tool.Context, s *cloudmonitoring.Service, now time.Time, zones map[string]*zoneData) error {
	// Collect and add Latency data.
	latencyMetrics, err := getMetricData(ctx, s, cloudServiceLatencyMetric, now, "")
	if err != nil {
		return err
	}
	for zone, instanceMetrics := range latencyMetrics {
		if _, ok := zones[zone]; !ok {
			zones[zone] = newZoneData(zone)
		}
		latencyMetrics := zones[zone].CloudServices.Latency
		for instance, metrics := range instanceMetrics {
			if _, ok := latencyMetrics[instance]; !ok {
				latencyMetrics[instance] = []*metricData{}
			}
			latencyMetrics[instance] = append(latencyMetrics[instance], metrics...)
			// Set thresholds and calculate health.
			for _, metric := range metrics {
				metric.Threshold = thresholdServiceLatency
				metric.Healthy = !overThresholdFor(metric.HistoryTimestamps, metric.HistoryValues, thresholdServiceLatency, thresholdHoldMinutes)
			}
		}
	}

	// Collect and add Stats data (counters + qps).
	counterMetrics, err := getMetricData(ctx, s, cloudServiceCountersMetric, now, "")
	if err != nil {
		return err
	}
	qpsMetrics, err := getMetricData(ctx, s, cloudServiceQPSMetric, now, " qps")
	if err != nil {
		return err
	}
	addStatsFn := func(metrics map[string]metricsMap) {
		for zone, instanceMetrics := range metrics {
			if _, ok := zones[zone]; !ok {
				zones[zone] = newZoneData(zone)
			}
			statsMetrics := zones[zone].CloudServices.Stats
			for instance, metrics := range instanceMetrics {
				if _, ok := statsMetrics[instance]; !ok {
					statsMetrics[instance] = []*metricData{}
				}
				statsMetrics[instance] = append(statsMetrics[instance], metrics...)
				// Set thresholds and calculate health.
				for _, metric := range metrics {
					switch metric.Name {
					case "mounttable qps":
						metric.Threshold = thresholdMounttableQPS
						metric.Healthy = !overThresholdFor(metric.HistoryTimestamps, metric.HistoryValues, thresholdMounttableQPS, thresholdHoldMinutes)
					}
				}
			}
		}
	}
	addStatsFn(counterMetrics)
	addStatsFn(qpsMetrics)

	return nil
}

func collectCloudServicesBuildInfo(ctx *tool.Context, zones map[string]*zoneData) error {
	serviceLocation := monitoring.ServiceLocationMap[namespaceRoot]
	if serviceLocation == nil {
		return fmt.Errorf("failed to find service location for %q", namespaceRoot)
	}
	zone := serviceLocation.Zone
	instance := serviceLocation.Instance
	if _, ok := zones[zone]; !ok {
		zones[zone] = newZoneData(zone)
	}
	buildInfoByInstance := zones[zone].CloudServices.BuildInfo
	if _, ok := buildInfoByInstance[instance]; !ok {
		buildInfoByInstance[instance] = []*buildInfoData{}
	}

	// Run "debug stats read" command to query build info data.
	debug := filepath.Join(binDirFlag, "debug")
	var buf bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &buf
	opts.Stderr = &buf
	if err := ctx.Run().TimedCommandWithOpts(
		debugCommandTimeout, opts, debug,
		"--v23.namespace.root", namespaceRoot,
		"--v23.credentials", credentialsFlag, "stats", "read", fmt.Sprintf("%s/build.[TPUM]*", buildInfoEndpointPrefix)); err != nil {
		return fmt.Errorf("debug command failed: %v\n%s", err, buf.String())
	}

	// Parse output.
	lines := strings.Split(buf.String(), "\n")
	type buildInfo struct {
		pristine string
		user     string
		time     string
		snapshot string
	}
	buildInfoByServiceName := map[string]*buildInfo{}
	for _, line := range lines {
		matches := buildInfoRE.FindStringSubmatch(line)
		if matches != nil {
			service := matches[1]
			metadataName := matches[2]
			value := matches[3]
			if _, ok := buildInfoByServiceName[service]; !ok {
				buildInfoByServiceName[service] = &buildInfo{}
			}
			curBuildInfo := buildInfoByServiceName[service]
			switch metadataName {
			case "Manifest":
				manifestMatches := manifestRE.FindStringSubmatch(value)
				if manifestMatches != nil {
					curBuildInfo.snapshot = manifestMatches[1]
				}
			case "Pristine":
				curBuildInfo.pristine = value
			case "Time":
				t, err := time.Parse(time.RFC3339, value)
				if err != nil {
					return fmt.Errorf("Parse(%s) failed: %v", value, err)
				}
				curBuildInfo.time = fmt.Sprintf("%d", t.Unix())
			case "User":
				curBuildInfo.user = value
			}
		}
	}
	sortedServiceNames := []string{}
	for serviceName := range buildInfoByServiceName {
		sortedServiceNames = append(sortedServiceNames, serviceName)
	}
	sort.Strings(sortedServiceNames)
	for _, serviceName := range sortedServiceNames {
		curBuildInfo := buildInfoByServiceName[serviceName]
		buildInfoByInstance[instance] = append(buildInfoByInstance[instance], &buildInfoData{
			ZoneName:     zone,
			InstanceName: instance,
			ServiceName:  serviceName,
			IsPristine:   curBuildInfo.pristine,
			Snapshot:     curBuildInfo.snapshot,
			Time:         curBuildInfo.time,
			User:         curBuildInfo.user,
		})
	}

	return nil
}

func collectNginxData(ctx *tool.Context, s *cloudmonitoring.Service, now time.Time, zones map[string]*zoneData) error {
	nginxMetrics, err := getMetricData(ctx, s, nginxStatsMetric, now, "")
	if err != nil {
		return err
	}
	for zone, instanceMetrics := range nginxMetrics {
		if _, ok := zones[zone]; !ok {
			zones[zone] = newZoneData(zone)
		}
		loadMetrics := zones[zone].Nginx.Load
		for instance, metrics := range instanceMetrics {
			if _, ok := loadMetrics[instance]; !ok {
				loadMetrics[instance] = []*metricData{}
			}
			loadMetrics[instance] = append(loadMetrics[instance], metrics...)
		}
	}

	return nil
}

func collectGCEInstancesData(ctx *tool.Context, s *cloudmonitoring.Service, now time.Time, zones map[string]*zoneData) error {
	// Use "gcloud compute instances list" to get instances status.
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	if err := ctx.Run().CommandWithOpts(opts,
		"gcloud", "-q", "--project="+projectFlag, "compute", "instances", "list", "--format=json"); err != nil {
		return err
	}
	type instanceData struct {
		Name   string
		Zone   string
		Status string
		Id     string
	}
	var instances []instanceData
	if err := json.Unmarshal(out.Bytes(), &instances); err != nil {
		return fmt.Errorf("Unmarshal() failed: %v", err)
	}
	instancesByZone := map[string][]instanceData{}
	for _, instance := range instances {
		if strings.HasPrefix(instance.Name, "nginx") || strings.HasPrefix(instance.Name, "vanadium") {
			instancesByZone[instance.Zone] = append(instancesByZone[instance.Zone], instance)
		}
	}

	// Query stats.
	gceMetrics, err := getMetricData(ctx, s, gceStatsMetric, now, "")
	if err != nil {
		return err
	}
	for zone, instanceMetrics := range gceMetrics {
		if _, ok := zones[zone]; !ok {
			zones[zone] = newZoneData(zone)
		}
		statsMetrics := zones[zone].GCE.Stats
		for instance, metrics := range instanceMetrics {
			if _, ok := statsMetrics[instance]; !ok {
				statsMetrics[instance] = []*metricData{}
			}
			statsMetrics[instance] = append(statsMetrics[instance], metrics...)
			// Set thresholds and calculate health.
			for _, metric := range metrics {
				switch metric.Name {
				case "cpu-usage":
					metric.Threshold = thresholdCPU
					metric.Healthy = !overThresholdFor(metric.HistoryTimestamps, metric.HistoryValues, thresholdCPU, thresholdHoldMinutes)
				case "disk-usage":
					metric.Threshold = thresholdDisk
					metric.Healthy = !overThresholdFor(metric.HistoryTimestamps, metric.HistoryValues, thresholdDisk, thresholdHoldMinutes)
				case "memory-usage":
					metric.Threshold = thresholdRam
					metric.Healthy = !overThresholdFor(metric.HistoryTimestamps, metric.HistoryValues, thresholdRam, thresholdHoldMinutes)
				case "ping":
					metric.Threshold = thresholdPing
					metric.Healthy = !overThresholdFor(metric.HistoryTimestamps, metric.HistoryValues, thresholdPing, thresholdHoldMinutes)
				case "tcpconn":
					metric.Threshold = thresholdTCPConn
					metric.Healthy = !overThresholdFor(metric.HistoryTimestamps, metric.HistoryValues, thresholdTCPConn, thresholdHoldMinutes)
				}
			}
		}
	}
	for zone, instances := range instancesByZone {
		curZone := zones[zone]
		if curZone == nil {
			continue
		}
		for _, instance := range instances {
			curZone.GCE.GCEInfo[instance.Name] = gceInfoData{
				Status: instance.Status,
				Id:     instance.Id,
			}
		}
	}

	return nil
}

func overThresholdFor(timestamps []int64, values []float64, threshold float64, holdMinutes int) bool {
	numPoints := len(timestamps)
	maxTime := timestamps[numPoints-1]
	for i := numPoints - 1; i >= 0; i-- {
		t := timestamps[i]
		v := values[i]
		if v >= threshold {
			if (maxTime - t) >= int64(holdMinutes*60) {
				return true
			}
		} else {
			return false
		}
	}
	return false
}

func newZoneData(zone string) *zoneData {
	return &zoneData{
		CloudServices: &cloudServiceData{
			ZoneName:  zone,
			Latency:   metricsMap{},
			Stats:     metricsMap{},
			BuildInfo: buildInfoMap{},
		},
		Nginx: &nginxData{
			ZoneName: zone,
			Load:     metricsMap{},
		},
		GCE: &gceInstanceData{
			ZoneName: zone,
			GCEInfo:  map[string]gceInfoData{},
			Stats:    metricsMap{},
		},
	}
}

// getMetricData queries GCM with the given metric, and returns metric items
// (metricData) in map that is first indexed by zone names then by
// instance names.
func getMetricData(ctx *tool.Context, s *cloudmonitoring.Service, metric string, now time.Time, metricNameSuffix string) (map[string]metricsMap, error) {
	// Query the given metric.
	resp, err := s.Timeseries.List(projectFlag, metric, now.Format(time.RFC3339), &cloudmonitoring.ListTimeseriesRequest{
		Kind: "cloudmonitoring#listTimeseriesRequest",
	}).Timespan(historyDuration).Do()
	if err != nil {
		return nil, fmt.Errorf("List failed: %v", err)
	}

	// Populate metric items and put them into the following zone-instance map.
	data := map[string]metricsMap{}
	for _, t := range resp.Timeseries {
		zone := t.TimeseriesDesc.Labels[gceZoneLabelKey]
		instance := t.TimeseriesDesc.Labels[gceInstanceLabelKey]
		metricName := t.TimeseriesDesc.Labels[metricNameLabelKey]

		if _, ok := data[zone]; !ok {
			data[zone] = metricsMap{}
		}
		instanceMap := data[zone]
		if _, ok := instanceMap[instance]; !ok {
			instanceMap[instance] = []*metricData{}
		}

		curMetricData := &metricData{
			ZoneName:     zone,
			InstanceName: instance,
			Name:         metricName + metricNameSuffix,
			CurrentValue: t.Points[0].DoubleValue,
			MinTime:      math.MaxInt64,
			MaxTime:      0,
			MinValue:     math.MaxFloat64,
			MaxValue:     0,
			Threshold:    -1,
			Healthy:      true,
		}
		numPoints := len(t.Points)
		timestamps := []int64{}
		values := []float64{}
		// t.Points are sorted from now to past. We iterate them backward.
		for i := numPoints - 1; i >= 0; i-- {
			point := t.Points[i]
			epochTime, err := time.Parse(time.RFC3339, point.Start)
			if err != nil {
				fmt.Fprint(ctx.Stderr(), "%v\n", err)
				continue
			}
			timestamp := epochTime.Unix()
			timestamps = append(timestamps, timestamp)
			values = append(values, point.DoubleValue)
			curMetricData.MinTime = int64(math.Min(float64(curMetricData.MinTime), float64(timestamp)))
			curMetricData.MaxTime = int64(math.Max(float64(curMetricData.MaxTime), float64(timestamp)))
			curMetricData.MinValue = math.Min(curMetricData.MinValue, point.DoubleValue)
			curMetricData.MaxValue = math.Max(curMetricData.MaxValue, point.DoubleValue)
		}
		curMetricData.HistoryTimestamps = timestamps
		curMetricData.HistoryValues = values
		instanceMap[instance] = append(instanceMap[instance], curMetricData)
	}
	return data, nil
}

func persistOncallData(ctx *tool.Context, statusData *serviceStatusData, oncall *oncallData, now time.Time) error {
	// Use timestamp (down to the minute part) as the main file name.
	// We store oncall data and status data separately for efficiency.
	curTime := now.Format("200601021504")
	bytesStatus, err := json.MarshalIndent(statusData, "", "  ")
	if err != nil {
		return fmt.Errorf("MarshalIndent() failed: %v", err)
	}
	bytesOncall, err := json.MarshalIndent(oncall, "", "  ")
	if err != nil {
		return fmt.Errorf("MarshalIndent() failed: %v", err)
	}

	// Write data to a temporary directory.
	tmpDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		return err
	}
	defer ctx.Run().RemoveAll(tmpDir)
	statusDataFile := filepath.Join(tmpDir, fmt.Sprintf("%s.status", curTime))
	if err := ctx.Run().WriteFile(statusDataFile, bytesStatus, os.FileMode(0600)); err != nil {
		return err
	}
	oncallDataFile := filepath.Join(tmpDir, fmt.Sprintf("%s.oncall", curTime))
	if err := ctx.Run().WriteFile(oncallDataFile, bytesOncall, os.FileMode(0600)); err != nil {
		return err
	}
	latestFile := filepath.Join(tmpDir, "latest")
	if err := ctx.Run().WriteFile(latestFile, []byte(curTime), os.FileMode(0600)); err != nil {
		return err
	}

	// Upload data to Google Storage.
	args := []string{"-q", "cp", filepath.Join(tmpDir, "*"), bucket + "/"}
	if err := ctx.Run().Command("gsutil", args...); err != nil {
		return err
	}

	return nil
}
