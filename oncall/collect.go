// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/api/cloudmonitoring/v2beta2"
	"v.io/x/devtools/internal/monitoring"
	"v.io/x/devtools/internal/tool"
	"v.io/x/lib/cmdline"
)

const (
	cloudServiceLatencyMetric = "custom.cloudmonitoring.googleapis.com/v/service/latency"
	metricNameLabelKey        = "custom.cloudmonitoring.googleapis.com/metric-name"
	historyDuration           = "1h"
	serviceStatusOK           = "serviceStatusOK"
	serviceStatusWarning      = "serviceStatusWarning"
	serviceStatusDown         = "serviceStatusDown"
	warningLatency            = 2000
	criticalLatency           = 5000
)

var (
	keyFileFlag        string
	projectFlag        string
	serviceAccountFlag string
)

type OncallData struct {
	CloudServices map[string][]CloudServiceData // Indexed by zone names.
	GCEInstances  map[string][]GCEInstanceData  // Indexed by zone names.
}

type CloudServiceData struct {
	Name    string
	Latency LatencyData
}

type LatencyData struct {
	CurrentValue float64
	History      []PointData
}

type PointData struct {
	Time  int64
	Value float64
}

type GCEInstanceData struct {
	Name   string
	Zone   string
	Status string
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
	cmdCollect.Flags.StringVar(&keyFileFlag, "key", "", "The path to the service account's key file.")
	cmdCollect.Flags.StringVar(&projectFlag, "project", "", "The GCM's corresponding GCE project ID.")
	cmdCollect.Flags.StringVar(&serviceAccountFlag, "account", "", "The service account used to communicate with GCM.")
}

// cmdCollect represents the 'collect' command of the oncall tool.
var cmdCollect = &cmdline.Command{
	Name:  "collect",
	Short: "Collect data for oncall dashboard",
	Long: `
This subcommand collects data from Google Cloud Monitoring and stores the
processed data to Google Storage.
`,
	Run: runCollect,
}

func runCollect(command *cmdline.Command, _ []string) error {
	ctx := tool.NewContextFromCommand(command, tool.ContextOpts{
		Color:   &colorFlag,
		Verbose: &verboseFlag,
		DryRun:  &dryrunFlag,
	})

	s, err := monitoring.Authenticate(serviceAccountFlag, keyFileFlag)
	if err != nil {
		return err
	}
	now := time.Now()

	// Collect service status data used in the external dashboard.
	statusData, err := collectServiceStatusData(ctx, s, now)
	if err != nil {
		return err
	}

	// Collect oncall related data used in the internal oncall dashboard.
	oncallData := &OncallData{}
	cloudServicesData, err := collectCloudServicesData(ctx, s, now)
	if err != nil {
		return err
	}
	oncallData.CloudServices = cloudServicesData

	gceInstancesData, err := collectGCEInstancesData(ctx)
	if err != nil {
		return err
	}
	oncallData.GCEInstances = gceInstancesData

	if err := persistOncallData(ctx, statusData, oncallData, now); err != nil {
		return err
	}

	return nil
}

func collectServiceStatusData(ctx *tool.Context, s *cloudmonitoring.Service, now time.Time) (*serviceStatusData, error) {
	// Collect data for the last 8 days and aggregate data every 10 minutes.
	resp, err := s.Timeseries.List(projectFlag, cloudServiceLatencyMetric, now.Format(time.RFC3339), &cloudmonitoring.ListTimeseriesRequest{
		Kind: "cloudmonitoring#listTimeseriesRequest",
	}).Window("10m").Timespan("8d").Aggregator("max").Do()
	if err != nil {
		return nil, fmt.Errorf("List failed: %v", err)
	}

	status := []statusData{}
	for _, t := range resp.Timeseries {
		curStatusData := statusData{
			Name:          t.TimeseriesDesc.Labels[metricNameLabelKey],
			CurrentStatus: statusForLatency(t.Points[0].DoubleValue), // t.Points[0] is the latest
			// TODO(jingjin): add build timestamp and snapshot label when they are available.
		}
		incidents, err := calcIncidents(t.Points)
		if err != nil {
			return nil, err
		}
		curStatusData.Incidents = incidents
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

func collectCloudServicesData(ctx *tool.Context, s *cloudmonitoring.Service, now time.Time) (map[string][]CloudServiceData, error) {
	// Query data for the cloud service latency metric which will return multiple
	// time series for different services.
	resp, err := s.Timeseries.List(projectFlag, cloudServiceLatencyMetric, now.Format(time.RFC3339), &cloudmonitoring.ListTimeseriesRequest{
		Kind: "cloudmonitoring#listTimeseriesRequest",
	}).Timespan(historyDuration).Do()
	if err != nil {
		return nil, fmt.Errorf("List failed: %v", err)
	}
	cloudServicesData := []CloudServiceData{}
	for _, t := range resp.Timeseries {
		cloudServiceData := CloudServiceData{
			Name: t.TimeseriesDesc.Labels[metricNameLabelKey],
		}
		pts := []PointData{}
		for _, point := range t.Points {
			epochTime, err := time.Parse(time.RFC3339, point.Start)
			if err != nil {
				fmt.Fprint(ctx.Stderr(), "%v\n", err)
				continue
			}
			pts = append(pts, PointData{
				Time:  epochTime.Unix(),
				Value: point.DoubleValue,
			})
		}
		latencyData := LatencyData{
			CurrentValue: t.Points[0].DoubleValue, // t.Points are sorted from now to past.
			History:      pts,
		}
		cloudServiceData.Latency = latencyData
		cloudServicesData = append(cloudServicesData, cloudServiceData)
	}

	// We only have services in one zone for now.
	return map[string][]CloudServiceData{
		"us-central1-c": cloudServicesData,
	}, nil
}

func collectGCEInstancesData(ctx *tool.Context) (map[string][]GCEInstanceData, error) {
	// Use "gcloud compute instances list" to get instances status.
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	if err := ctx.Run().CommandWithOpts(opts,
		"gcloud", "-q", "--project="+projectFlag, "compute", "instances", "list", "--format=json"); err != nil {
		return nil, err
	}
	var instances []GCEInstanceData
	if err := json.Unmarshal(out.Bytes(), &instances); err != nil {
		return nil, fmt.Errorf("Unmarshal() failed: %v", err)
	}
	instancesByZone := map[string][]GCEInstanceData{}
	for _, instance := range instances {
		instancesByZone[instance.Zone] = append(instancesByZone[instance.Zone], instance)
	}
	return instancesByZone, nil
}

func persistOncallData(ctx *tool.Context, statusData *serviceStatusData, oncallData *OncallData, now time.Time) error {
	// Use timestamp (down to the minute part) as the main file name.
	// We store oncall data and status data separately for efficiency.
	curTime := now.Format("200601021504")
	bytesStatus, err := json.MarshalIndent(statusData, "", "  ")
	if err != nil {
		return fmt.Errorf("MarshalIndent() failed: %v", err)
	}
	bytesOncall, err := json.MarshalIndent(oncallData, "", "  ")
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
