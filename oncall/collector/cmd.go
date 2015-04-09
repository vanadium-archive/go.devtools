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
	nameLabelKey              = "custom.cloudmonitoring.googleapis.com/name"
	historyDuration           = "1h"
	bucket                    = "gs://vanadium-oncall/data"
)

var (
	colorFlag          bool
	dryrunFlag         bool
	keyFileFlag        string
	projectFlag        string
	serviceAccountFlag string
	verboseFlag        bool
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

func init() {
	cmdRoot.Flags.BoolVar(&colorFlag, "color", true, "Use color to format output.")
	cmdRoot.Flags.BoolVar(&dryrunFlag, "n", false, "Show what commands will run, but do not execute them.")
	cmdRoot.Flags.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
	cmdRoot.Flags.StringVar(&keyFileFlag, "key", "", "The path to the service account's key file.")
	cmdRoot.Flags.StringVar(&projectFlag, "project", "", "The GCM's corresponding GCE project ID.")
	cmdRoot.Flags.StringVar(&serviceAccountFlag, "account", "", "The service account used to communicate with GCM.")
}

// root returns a command that represents the root of the collector tool.
func root() *cmdline.Command {
	return cmdRoot
}

// cmdRoot represents the root of the collector tool.
var cmdRoot = &cmdline.Command{
	Run:   runRoot,
	Name:  "collector",
	Short: "Tool for collecting data displayed in oncall dashboard",
	Long:  "Tool for collecting data displayed in oncall dashboard.",
}

func runRoot(command *cmdline.Command, _ []string) error {
	ctx := tool.NewContextFromCommand(command, tool.ContextOpts{
		Color:   &colorFlag,
		Verbose: &verboseFlag,
		DryRun:  &dryrunFlag,
	})

	oncallData := &OncallData{}
	now := time.Now()

	cloudServicesData, err := collectCloudServicesData(ctx, now)
	if err != nil {
		return err
	}
	oncallData.CloudServices = cloudServicesData

	gceInstancesData, err := collectGCEInstancesData(ctx)
	if err != nil {
		return err
	}
	oncallData.GCEInstances = gceInstancesData

	if err := persistOncallData(ctx, oncallData, now); err != nil {
		return err
	}

	return nil
}

func collectCloudServicesData(ctx *tool.Context, now time.Time) (map[string][]CloudServiceData, error) {
	s, err := monitoring.Authenticate(serviceAccountFlag, keyFileFlag)
	if err != nil {
		return nil, err
	}

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
			Name: t.TimeseriesDesc.Labels[nameLabelKey],
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

func persistOncallData(ctx *tool.Context, oncallData *OncallData, now time.Time) error {
	// Use timestamp (down to the minute part) as file name.
	fileName := now.Format("200601021504")
	bytes, err := json.MarshalIndent(oncallData, "", "  ")
	if err != nil {
		return fmt.Errorf("MarshalIndent() failed: %v", err)
	}

	// Write data to a temporary directory.
	tmpDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		return err
	}
	defer ctx.Run().RemoveAll(tmpDir)
	oncallDataFile := filepath.Join(tmpDir, fileName)
	if err := ctx.Run().WriteFile(oncallDataFile, bytes, os.FileMode(0600)); err != nil {
		return err
	}
	latestFile := filepath.Join(tmpDir, "latest")
	if err := ctx.Run().WriteFile(latestFile, []byte(fileName), os.FileMode(0600)); err != nil {
		return err
	}

	// Upload data to Google Storage.
	args := []string{"-q", "cp", filepath.Join(tmpDir, "*"), bucket + "/"}
	if err := ctx.Run().Command("gsutil", args...); err != nil {
		return err
	}

	return nil
}
