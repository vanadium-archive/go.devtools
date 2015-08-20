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
	debugCommandTimeout = time.Second * 10
	buildInfoRE         = regexp.MustCompile(`devmgr/apps/([^/]*)/.*/stats/system/metadata/build.(Pristine|Time|User|Manifest):\s*(.*)`)
	manifestRE          = regexp.MustCompile(`.*label="(.*)">`)
)

type oncallData struct {
	CollectionTimestamp int64
	Zones               map[string]*zoneData // Indexed by zone names.
	OncallIDs           string               // IDs separated by ",".
}

type zoneData struct {
	Instances map[string]*allMetricsData // Indexed by instance names.
	Max       *allMetricsData
	Average   *allMetricsData
}

type allMetricsData struct {
	CloudServiceLatency   map[string]*metricData // Indexed by metric names. Same below.
	CloudServiceStats     map[string]*metricData
	CloudServiceGCE       map[string]*metricData
	CloudServiceBuildInfo map[string]*buildInfoData
	NginxLoad             map[string]*metricData
	NginxGCE              map[string]*metricData
	GCEInfo               *gceInfoData
	Range                 *rangeData
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

// metricDataMap is a map with the following structure:
// {zoneName, {instanceName, {metricName, *metricData}}}.
type metricDataMap map[string]map[string]map[string]*metricData

type gceInfoData struct {
	Status string
	Id     string
}

type buildInfoData struct {
	ZoneName     string
	InstanceName string
	ServiceName  string
	IsPristine   string
	Snapshot     string
	Time         string
	User         string
}

type rangeData struct {
	MinTime int64
	MaxTime int64
}

func (r *rangeData) update(newMinTime, newMaxTime int64) {
	if newMinTime < r.MinTime {
		r.MinTime = newMinTime
	}
	if newMaxTime > r.MaxTime {
		r.MaxTime = newMaxTime
	}
}

type aggMetricData struct {
	TimestampsToValues map[int64][]float64
}
type aggAllMetricsData struct {
	CloudServiceLatency map[string]*aggMetricData // Indexed by metric names. Same below.
	CloudServiceStats   map[string]*aggMetricData
	CloudServiceGCE     map[string]*aggMetricData
	NginxLoad           map[string]*aggMetricData
	NginxGCE            map[string]*aggMetricData
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

type int64arr []int64

func (a int64arr) Len() int           { return len(a) }
func (a int64arr) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a int64arr) Less(i, j int) bool { return a[i] < a[j] }
func (a int64arr) Sort()              { sort.Sort(a) }

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
	ctx := tool.NewContextFromEnv(env)
	s, err := monitoring.Authenticate(serviceAccountFlag, keyFileFlag)
	if err != nil {
		return err
	}
	now := time.Now()

	// Collect oncall related data used in the internal oncall dashboard.
	zones := map[string]*zoneData{}
	oncall := &oncallData{
		CollectionTimestamp: now.Unix(),
		Zones:               zones,
	}
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
	if err := collectOncallIDsData(ctx, oncall); err != nil {
		return err
	}

	// Collect service status data used in the external dashboard.
	buildInfo := zones["us-central1-c"].Instances["vanadium-cell-master"].CloudServiceBuildInfo
	statusData, err := collectServiceStatusData(ctx, s, now, buildInfo)
	if err != nil {
		return err
	}

	if err := persistOncallData(ctx, statusData, oncall, now); err != nil {
		return err
	}

	return nil
}

func collectServiceStatusData(ctx *tool.Context, s *cloudmonitoring.Service, now time.Time, buildInfo map[string]*buildInfoData) (*serviceStatusData, error) {
	// Collect data for the last 8 days and aggregate data every 10 minutes.
	resp, err := s.Timeseries.List(projectFlag, cloudServiceLatencyMetric, now.Format(time.RFC3339), &cloudmonitoring.ListTimeseriesRequest{
		Kind: "cloudmonitoring#listTimeseriesRequest",
	}).Window("10m").Timespan("8d").Aggregator("max").Do()
	if err != nil {
		return nil, fmt.Errorf("List failed: %v", err)
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
		switch serviceName {
		case "binary discharger", "google identity service", "macaroon service":
			buildInfoServiceName = "identityd"
		case "mounttable":
			buildInfoServiceName = "mounttabled"
		case "proxy service":
			buildInfoServiceName = "proxyd"
		case "binary repository":
			buildInfoServiceName = "binaryd"
		case "application repository":
			buildInfoServiceName = "applicationd"
		}
		curBuildInfo := buildInfo[buildInfoServiceName]
		if curBuildInfo != nil {
			ts, err := strconv.ParseInt(curBuildInfo.Time, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("ParseInt(%s) failed: %v", curBuildInfo.Time, err)
			}
			curStatusData.BuildTimestamp = time.Unix(ts, 0).Format(time.RFC822)
			curStatusData.SnapshotLabel = strings.Replace(curBuildInfo.Snapshot, "snapshot/labels/", "", -1)
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
	// Collect and add latency data.
	latencyMetrics, err := getMetricData(ctx, s, cloudServiceLatencyMetric, now, "latency")
	if err != nil {
		return err
	}
	for zone, instanceMap := range latencyMetrics {
		if zones[zone] == nil {
			zones[zone] = newZoneData(zone)
		}
		zoneData := zones[zone]
		aggData := map[string]*aggMetricData{}
		for instance, metricMap := range instanceMap {
			if zoneData.Instances[instance] == nil {
				zoneData.Instances[instance] = newInstanceData()
			}
			zoneData.Instances[instance].CloudServiceLatency = metricMap
			for _, metric := range metricMap {
				metric.Threshold = thresholdServiceLatency
				if metric.Threshold != -1 {
					metric.Healthy = !overThresholdFor(metric.HistoryTimestamps, metric.HistoryValues, thresholdServiceLatency, thresholdHoldMinutes)
				}
				aggregateMetricData(aggData, metric)
			}
		}
		maxData, maxRangeData, averageData, averageRangeData := calculateMaxAndAverageData(aggData, zone)
		zoneData.Max.CloudServiceLatency, zoneData.Average.CloudServiceLatency = maxData, averageData
		zoneData.Max.Range.update(maxRangeData.MinTime, maxRangeData.MaxTime)
		zoneData.Average.Range.update(averageRangeData.MinTime, averageRangeData.MaxTime)
	}

	// Collect and add stats (counters + qps) data.
	counterMetrics, err := getMetricData(ctx, s, cloudServiceCountersMetric, now, "")
	if err != nil {
		return err
	}
	qpsMetrics, err := getMetricData(ctx, s, cloudServiceQPSMetric, now, "qps")
	if err != nil {
		return err
	}
	aggDataByZone := map[string]map[string]*aggMetricData{}
	addStatsFn := func(metrics metricDataMap) {
		for zone, instanceMap := range metrics {
			if zones[zone] == nil {
				zones[zone] = newZoneData(zone)
			}
			zoneData := zones[zone]
			aggData := aggDataByZone[zone]
			if aggData == nil {
				aggData = map[string]*aggMetricData{}
			}
			aggDataByZone[zone] = aggData
			for instance, metricMap := range instanceMap {
				if zoneData.Instances[instance] == nil {
					zoneData.Instances[instance] = newInstanceData()
				}
				stats := zoneData.Instances[instance].CloudServiceStats
				if stats == nil {
					stats = map[string]*metricData{}
					zoneData.Instances[instance].CloudServiceStats = stats
				}
				for metricName, metric := range metricMap {
					metric.Threshold = getThreshold(metricName)
					if metric.Threshold != -1 {
						metric.Healthy = !overThresholdFor(metric.HistoryTimestamps, metric.HistoryValues, metric.Threshold, thresholdHoldMinutes)
					}
					stats[metricName] = metric
					aggregateMetricData(aggData, metric)
				}
			}
		}
	}
	addStatsFn(counterMetrics)
	addStatsFn(qpsMetrics)

	for zone, aggData := range aggDataByZone {
		zoneData := zones[zone]
		maxData, maxRangeData, averageData, averageRangeData := calculateMaxAndAverageData(aggData, zone)
		zoneData.Max.CloudServiceStats, zoneData.Average.CloudServiceStats = maxData, averageData
		zoneData.Max.Range.update(maxRangeData.MinTime, maxRangeData.MaxTime)
		zoneData.Average.Range.update(averageRangeData.MinTime, averageRangeData.MaxTime)
	}

	return nil
}

func collectCloudServicesBuildInfo(ctx *tool.Context, zones map[string]*zoneData) error {
	serviceLocation := monitoring.ServiceLocationMap[namespaceRoot]
	if serviceLocation == nil {
		return fmt.Errorf("failed to find service location for %q", namespaceRoot)
	}
	zone := serviceLocation.Zone
	instance := serviceLocation.Instance
	if zones[zone] == nil {
		zones[zone] = newZoneData(zone)
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
	buildInfoByServiceName := map[string]*buildInfoData{}
	for _, line := range lines {
		matches := buildInfoRE.FindStringSubmatch(line)
		if matches != nil {
			service := matches[1]
			metadataName := matches[2]
			value := matches[3]
			if _, ok := buildInfoByServiceName[service]; !ok {
				buildInfoByServiceName[service] = &buildInfoData{
					ZoneName:     zone,
					InstanceName: instance,
					ServiceName:  service,
				}
			}
			curBuildInfo := buildInfoByServiceName[service]
			switch metadataName {
			case "Manifest":
				manifestMatches := manifestRE.FindStringSubmatch(value)
				if manifestMatches != nil {
					curBuildInfo.Snapshot = strings.Replace(manifestMatches[1], "snapshot/labels/", "", -1)
				}
			case "Pristine":
				curBuildInfo.IsPristine = value
			case "Time":
				t, err := time.Parse(time.RFC3339, value)
				if err != nil {
					return fmt.Errorf("Parse(%s) failed: %v", value, err)
				}
				curBuildInfo.Time = fmt.Sprintf("%d", t.Unix())
			case "User":
				curBuildInfo.User = value
			}
		}
	}

	if zones[zone].Instances[instance] == nil {
		zones[zone].Instances[instance] = newInstanceData()
	}
	zones[zone].Instances[instance].CloudServiceBuildInfo = buildInfoByServiceName

	return nil
}

func collectNginxData(ctx *tool.Context, s *cloudmonitoring.Service, now time.Time, zones map[string]*zoneData) error {
	nginxMetrics, err := getMetricData(ctx, s, nginxStatsMetric, now, "")
	if err != nil {
		return err
	}
	for zone, instanceMap := range nginxMetrics {
		if zones[zone] == nil {
			zones[zone] = newZoneData(zone)
		}
		zoneData := zones[zone]
		aggData := map[string]*aggMetricData{}
		for instance, metricMap := range instanceMap {
			if !strings.HasPrefix(instance, "nginx") {
				continue
			}
			if zoneData.Instances[instance] == nil {
				zoneData.Instances[instance] = newInstanceData()
			}
			zoneData.Instances[instance].NginxLoad = metricMap
			for _, metric := range metricMap {
				aggregateMetricData(aggData, metric)
			}
		}
		maxData, maxRangeData, averageData, averageRangeData := calculateMaxAndAverageData(aggData, zone)
		zoneData.Max.NginxLoad, zoneData.Average.NginxLoad = maxData, averageData
		zoneData.Max.Range.update(maxRangeData.MinTime, maxRangeData.MaxTime)
		zoneData.Average.Range.update(averageRangeData.MinTime, averageRangeData.MaxTime)
	}

	return nil
}

func collectGCEInstancesData(ctx *tool.Context, s *cloudmonitoring.Service, now time.Time, zones map[string]*zoneData) error {
	// Query and add GCE stats.
	gceMetrics, err := getMetricData(ctx, s, gceStatsMetric, now, "")
	if err != nil {
		return err
	}
	for zone, instanceMap := range gceMetrics {
		if zones[zone] == nil {
			zones[zone] = newZoneData(zone)
		}
		zoneData := zones[zone]
		aggDataCloudSerivcesGCE := map[string]*aggMetricData{}
		aggDataNginxGCE := map[string]*aggMetricData{}
		for instance, metricMap := range instanceMap {
			if zoneData.Instances[instance] == nil {
				zoneData.Instances[instance] = newInstanceData()
			}
			cloudServiceGCE := zoneData.Instances[instance].CloudServiceGCE
			nginxGCE := zoneData.Instances[instance].NginxGCE
			if cloudServiceGCE == nil {
				cloudServiceGCE = map[string]*metricData{}
				zoneData.Instances[instance].CloudServiceGCE = cloudServiceGCE
			}
			if nginxGCE == nil {
				nginxGCE = map[string]*metricData{}
				zoneData.Instances[instance].NginxGCE = nginxGCE
			}
			// Set thresholds and calculate health.
			for metricName, metric := range metricMap {
				metric.Threshold = getThreshold(metricName)
				if metric.Threshold != -1 {
					metric.Healthy = !overThresholdFor(metric.HistoryTimestamps, metric.HistoryValues, metric.Threshold, thresholdHoldMinutes)
				}
				if strings.HasPrefix(instance, "vanadium") {
					cloudServiceGCE[metricName] = metric
					aggregateMetricData(aggDataCloudSerivcesGCE, metric)
				} else if strings.HasPrefix(instance, "nginx") {
					nginxGCE[metricName] = metric
					aggregateMetricData(aggDataNginxGCE, metric)
				}
			}
		}

		maxData, maxRangeData1, averageData, averageRangeData1 := calculateMaxAndAverageData(aggDataCloudSerivcesGCE, zone)
		zoneData.Max.CloudServiceGCE, zoneData.Average.CloudServiceGCE = maxData, averageData
		maxData, maxRangeData2, averageData, averageRangeData2 := calculateMaxAndAverageData(aggDataNginxGCE, zone)
		zoneData.Max.NginxGCE, zoneData.Average.NginxGCE = maxData, averageData
		zoneData.Max.Range.update(maxRangeData1.MinTime, maxRangeData1.MaxTime)
		zoneData.Max.Range.update(maxRangeData2.MinTime, maxRangeData2.MaxTime)
		zoneData.Average.Range.update(averageRangeData1.MinTime, averageRangeData1.MaxTime)
		zoneData.Average.Range.update(averageRangeData2.MinTime, averageRangeData2.MaxTime)
	}

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

	// Add instance status.
	for zone, instances := range instancesByZone {
		curZone := zones[zone]
		if curZone == nil {
			continue
		}
		for _, instance := range instances {
			curZone.Instances[instance.Name].GCEInfo = &gceInfoData{
				Status: instance.Status,
				Id:     instance.Id,
			}
		}
	}

	return nil
}

func collectOncallIDsData(ctx *tool.Context, oncall *oncallData) error {
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	if err := ctx.Run().CommandWithOpts(opts, "v23", "oncall"); err != nil {
		return err
	}
	oncall.OncallIDs = strings.TrimSpace(out.String())
	return nil
}

// overThresholdFor checks whether the most recent values of the given time
// series are over the given threshold for the the given amount of time.
// This function assumes that the given time series data points are sorted by
// time (oldest first).
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
		Instances: map[string]*allMetricsData{},
		Max:       newInstanceData(),
		Average:   newInstanceData(),
	}
}

func newInstanceData() *allMetricsData {
	return &allMetricsData{
		CloudServiceLatency:   map[string]*metricData{},
		CloudServiceStats:     map[string]*metricData{},
		CloudServiceGCE:       map[string]*metricData{},
		CloudServiceBuildInfo: map[string]*buildInfoData{},
		NginxLoad:             map[string]*metricData{},
		NginxGCE:              map[string]*metricData{},
		Range:                 newRangeData(),
	}
}

func newRangeData() *rangeData {
	return &rangeData{
		MinTime: math.MaxInt64,
		MaxTime: 0,
	}
}

// getMetricData queries GCM with the given metric, and returns metric items
// (metricData) organized in metricDataMap.
func getMetricData(ctx *tool.Context, s *cloudmonitoring.Service, metric string, now time.Time, metricNameSuffix string) (metricDataMap, error) {
	// Query the given metric.
	resp, err := s.Timeseries.List(projectFlag, metric, now.Format(time.RFC3339), &cloudmonitoring.ListTimeseriesRequest{
		Kind: "cloudmonitoring#listTimeseriesRequest",
	}).Timespan(historyDuration).Do()
	if err != nil {
		return nil, fmt.Errorf("List() failed: %v", err)
	}

	// Populate metric items and put them into a metricDataMap.
	data := metricDataMap{}
	for _, t := range resp.Timeseries {
		zone := t.TimeseriesDesc.Labels[gceZoneLabelKey]
		instance := t.TimeseriesDesc.Labels[gceInstanceLabelKey]
		metricName := t.TimeseriesDesc.Labels[metricNameLabelKey]
		if metricNameSuffix != "" {
			metricName = fmt.Sprintf("%s %s", metricName, metricNameSuffix)
		}

		instanceMap := data[zone]
		if instanceMap == nil {
			instanceMap = map[string]map[string]*metricData{}
			data[zone] = instanceMap
		}

		metricMap := instanceMap[instance]
		if metricMap == nil {
			metricMap = map[string]*metricData{}
			instanceMap[instance] = metricMap
		}

		curMetricData := metricMap[metricName]
		if curMetricData == nil {
			curMetricData = &metricData{
				ZoneName:     zone,
				InstanceName: instance,
				Name:         metricName,
				CurrentValue: t.Points[0].DoubleValue,
				MinTime:      math.MaxInt64,
				MaxTime:      0,
				MinValue:     math.MaxFloat64,
				MaxValue:     0,
				Threshold:    -1,
				Healthy:      true,
			}
			metricMap[metricName] = curMetricData
		}

		numPoints := len(t.Points)
		timestamps := []int64{}
		values := []float64{}
		// t.Points are sorted from now to past. We process them starting with the
		// latest and going back in time.
		for i := numPoints - 1; i >= 0; i-- {
			point := t.Points[i]
			epochTime, err := time.Parse(time.RFC3339, point.Start)
			if err != nil {
				fmt.Fprintf(ctx.Stderr(), "%v\n", err)
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
	}
	return data, nil
}

// aggregateMetricData aggregates the history values of the given metric data
// into the given aggData map indexed by metric names.
func aggregateMetricData(aggData map[string]*aggMetricData, metric *metricData) {
	metricName := metric.Name
	curAggMetricData := aggData[metricName]
	if curAggMetricData == nil {
		curAggMetricData = &aggMetricData{
			TimestampsToValues: map[int64][]float64{},
		}
		aggData[metricName] = curAggMetricData
	}
	numPoints := len(metric.HistoryTimestamps)
	for i := 0; i < numPoints; i++ {
		t := metric.HistoryTimestamps[i]
		v := metric.HistoryValues[i]
		curAggMetricData.TimestampsToValues[t] = append(curAggMetricData.TimestampsToValues[t], v)
	}
}

// calculateMaxAndAverageData calculates and returns the max and average data
// from the given aggregated data.
func calculateMaxAndAverageData(aggData map[string]*aggMetricData, zone string) (map[string]*metricData, *rangeData, map[string]*metricData, *rangeData) {
	maxData := map[string]*metricData{}
	maxRangeData := newRangeData()
	averageData := map[string]*metricData{}
	averageRangeData := newRangeData()

	for metricName, metricAggData := range aggData {
		sortedTimestamps := int64arr{}
		for timestamp := range metricAggData.TimestampsToValues {
			sortedTimestamps = append(sortedTimestamps, timestamp)
		}
		sortedTimestamps.Sort()
		maxHistoryValues := []float64{}
		averageHistoryValues := []float64{}
		minMaxValue := math.MaxFloat64
		maxMaxValue := 0.0
		minAverageValue := math.MaxFloat64
		maxAverageValue := 0.0
		for _, timestamp := range sortedTimestamps {
			values := metricAggData.TimestampsToValues[timestamp]
			curMax := values[0]
			curSum := 0.0
			for _, v := range values {
				if v > curMax {
					curMax = v
				}
				curSum += v
			}
			curAverage := curSum / float64(len(values))
			maxHistoryValues = append(maxHistoryValues, curMax)
			averageHistoryValues = append(averageHistoryValues, curAverage)
			minMaxValue = math.Min(minMaxValue, curMax)
			maxMaxValue = math.Max(maxMaxValue, curMax)
			minAverageValue = math.Min(minAverageValue, curAverage)
			maxAverageValue = math.Max(maxAverageValue, curAverage)
		}
		minTime := sortedTimestamps[0]
		maxTime := sortedTimestamps[len(sortedTimestamps)-1]
		threshold := getThreshold(metricName)
		maxData[metricName] = &metricData{
			ZoneName:          zone,
			Name:              metricName,
			CurrentValue:      maxHistoryValues[len(maxHistoryValues)-1],
			MinTime:           minTime,
			MaxTime:           maxTime,
			MinValue:          minMaxValue,
			MaxValue:          maxMaxValue,
			HistoryTimestamps: sortedTimestamps,
			HistoryValues:     maxHistoryValues,
			Threshold:         threshold,
			Healthy:           true,
		}
		if threshold != -1 {
			maxData[metricName].Healthy = !overThresholdFor(sortedTimestamps, maxHistoryValues, threshold, thresholdHoldMinutes)
		}
		maxRangeData.update(minTime, maxTime)
		averageData[metricName] = &metricData{
			ZoneName:          zone,
			Name:              metricName,
			CurrentValue:      averageHistoryValues[len(maxHistoryValues)-1],
			MinTime:           minTime,
			MaxTime:           maxTime,
			MinValue:          minAverageValue,
			MaxValue:          maxAverageValue,
			HistoryTimestamps: sortedTimestamps,
			HistoryValues:     averageHistoryValues,
			Threshold:         threshold,
			Healthy:           true,
		}
		if threshold != -1 {
			averageData[metricName].Healthy = !overThresholdFor(sortedTimestamps, averageHistoryValues, threshold, thresholdHoldMinutes)
		}
		averageRangeData.update(minTime, maxTime)
	}

	return maxData, maxRangeData, averageData, averageRangeData
}

func getThreshold(metricName string) float64 {
	if strings.HasSuffix(metricName, "latency") {
		return thresholdServiceLatency
	}
	switch metricName {
	case "mounttable qps":
		return thresholdMounttableQPS
	case "cpu-usage":
		return thresholdCPU
	case "disk-usage":
		return thresholdDisk
	case "memory-usage":
		return thresholdRam
	case "ping":
		return thresholdPing
	case "tcpconn":
		return thresholdTCPConn
	}
	return -1.0
}

func persistOncallData(ctx *tool.Context, statusData *serviceStatusData, oncall *oncallData, now time.Time) error {
	// Use timestamp (down to the minute part) as the main file name.
	// We store oncall data and status data separately for efficiency.
	bytesStatus, err := json.MarshalIndent(statusData, "", "  ")
	if err != nil {
		return fmt.Errorf("MarshalIndent() failed: %v", err)
	}
	bytesOncall, err := json.MarshalIndent(oncall, "", "  ")
	if err != nil {
		return fmt.Errorf("MarshalIndent() failed: %v", err)
	}

	// Write data to a temporary directory.
	curTime := now.Format("200601021504")
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
	args := []string{"-q", "cp", filepath.Join(tmpDir, "*"), bucketData + "/"}
	if err := ctx.Run().Command("gsutil", args...); err != nil {
		return err
	}

	return nil
}
