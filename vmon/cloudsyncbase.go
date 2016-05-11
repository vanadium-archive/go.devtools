// Copyright 2016 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"runtime"
	"time"

	cloudmonitoring "google.golang.org/api/monitoring/v3"

	"v.io/jiri/tool"
	"v.io/v23"
	"v.io/v23/context"
	"v.io/v23/naming"
	"v.io/x/devtools/internal/monitoring"
	"v.io/x/devtools/internal/test"
)

const (
	cloudSyncbasePattern    = "home/*/syncbased"
	sysStatCPUUsagePercent  = "__debug/stats/system/syscpu/Percent"
	sysStatMemUsagePercent  = "__debug/stats/system/sysmem/UsedPercent"
	sysStatMemUsageBytes    = "__debug/stats/system/sysmem/Used"
	sysStatDiskUsagePercent = "__debug/stats/system/sysdisk/%2Fdata/UsedPercent"
	sysStatDiskUsageBytes   = "__debug/stats/system/sysdisk/%2Fdata/Used"
)

var (
	cloudSyncbaseTimeout = 20 * time.Second
)

type cloudSyncbaseStatsTask struct {
	mountedName  string
	sbMountEntry *naming.MountEntry
	taskType     cloudSyncbaseStatsTaskType
}

type cloudSyncbaseStatsTaskType int

const (
	taskTypeCpu cloudSyncbaseStatsTaskType = iota
	taskTypeMem
	taskTypeDisk
	taskTypeLatency
	taskTypeQPS
)

type cloudSyncbaseStatsResult struct {
	err error
}

type metricData struct {
	metricLabel string
	value       float64
}

func checkCloudSyncbaseInstances(v23ctx *context.T, ctx *tool.Context, s *cloudmonitoring.Service) error {
	v23ctx, cancel := context.WithTimeout(v23ctx, cloudSyncbaseTimeout)
	defer cancel()

	// Find all cloud syncbase instances.
	glob, err := v23.GetNamespace(v23ctx).Glob(v23ctx, cloudSyncbasePattern)
	if err != nil {
		return err
	}
	sbInstances := []*naming.MountEntry{}
	for e := range glob {
		switch e := e.(type) {
		case *naming.GlobReplyEntry:
			sbInstances = append(sbInstances, &e.Value)
		case *naming.GlobReplyError:
			fmt.Fprintf(ctx.Stderr(), "%v\n", e.Value.Error)
		}
	}

	// Query stats from each instance.
	taskTypes := []cloudSyncbaseStatsTaskType{taskTypeCpu, taskTypeMem, taskTypeDisk, taskTypeLatency, taskTypeQPS}
	numTasks := len(sbInstances) * len(taskTypes)
	tasks := make(chan cloudSyncbaseStatsTask, numTasks)
	taskResults := make(chan cloudSyncbaseStatsResult, numTasks)
	aggs := map[string]*aggregator{}
	md, err := monitoring.GetMetric("cloud-syncbase", projectFlag)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	// Start workers and distribute work to them.
	for i := 0; i < runtime.NumCPU(); i++ {
		go statsWorker(v23ctx, ctx, s, now, aggs, md, tasks, taskResults)
	}
	for _, sb := range sbInstances {
		for _, t := range taskTypes {
			tasks <- cloudSyncbaseStatsTask{
				mountedName:  sb.Name,
				sbMountEntry: sb,
				taskType:     t,
			}
		}
	}
	close(tasks)

	// Wait for all results to come back and send the aggregated data to GCM.
	for i := 0; i < numTasks; i++ {
		result := <-taskResults
		if result.err != nil {
			fmt.Fprintf(ctx.Stderr(), "%v\n", result.err)
			continue
		}
	}
	mdAgg, err := monitoring.GetMetric("cloud-syncbase-agg", projectFlag)
	if err != nil {
		return err
	}
	for metricLabel, agg := range aggs {
		if err := sendAggregatedDataToGCM(ctx, s, mdAgg, agg, now, metricLabel); err != nil {
			fmt.Fprintf(ctx.Stderr(), "%v\n", err)
		}
	}

	return nil
}

// statsWorker queries stats based on the task type, sends the results to GCM,
// and updates the corresponding aggregator.
func statsWorker(
	v23ctx *context.T, ctx *tool.Context, s *cloudmonitoring.Service,
	now string, aggs map[string]*aggregator, md *cloudmonitoring.MetricDescriptor,
	tasks <-chan cloudSyncbaseStatsTask, results chan<- cloudSyncbaseStatsResult) {
	for t := range tasks {
		result := cloudSyncbaseStatsResult{}
		metrics := []metricData{}
		switch t.taskType {
		case taskTypeCpu:
			if cpuUsagePct, err := getSysStat(v23ctx, ctx, t.sbMountEntry, sysStatCPUUsagePercent); err != nil {
				result.err = err
			} else {
				metrics = append(metrics, metricData{
					metricLabel: "syscpu-usage-pct",
					value:       cpuUsagePct,
				})
			}
		case taskTypeMem:
			if memUsagePct, err := getSysStat(v23ctx, ctx, t.sbMountEntry, sysStatMemUsagePercent); err != nil {
				result.err = err
			} else {
				metrics = append(metrics,
					metricData{
						metricLabel: "sysmem-usage-pct",
						value:       memUsagePct,
					})
			}
			if memUsageBytes, err := getSysStat(v23ctx, ctx, t.sbMountEntry, sysStatMemUsageBytes); err != nil {
				result.err = err
			} else {
				metrics = append(metrics,
					metricData{
						metricLabel: "sysmem-usage-bytes",
						value:       memUsageBytes,
					})
			}
		case taskTypeDisk:
			if diskUsagePct, err := getSysStat(v23ctx, ctx, t.sbMountEntry, sysStatDiskUsagePercent); err != nil {
				result.err = err
			} else {
				metrics = append(metrics,
					metricData{
						metricLabel: "sysdisk-usage-pct",
						value:       diskUsagePct,
					})
			}
			if diskUsageBytes, err := getSysStat(v23ctx, ctx, t.sbMountEntry, sysStatDiskUsageBytes); err != nil {
				result.err = err
			} else {
				metrics = append(metrics,
					metricData{
						metricLabel: "sysdisk-usage-bytes",
						value:       diskUsageBytes,
					})
			}
		case taskTypeLatency:
			if lat, err := getLatency(v23ctx, t.sbMountEntry); err != nil {
				result.err = err
			} else {
				metrics = append(metrics, metricData{
					metricLabel: "latency",
					value:       float64(lat.Nanoseconds()) / 1000000.0,
				})
			}
		case taskTypeQPS:
			if _, totalQPS, err := getQPS(v23ctx, ctx, t.sbMountEntry); err != nil {
				result.err = err
			} else {
				metrics = append(metrics, metricData{
					metricLabel: "qps",
					value:       totalQPS,
				})
			}
		}
		for _, metric := range metrics {
			getAggregator(aggs, metric.metricLabel).add(metric.value)
			if err := sendDataToGCM(s, md, metric.value, now, "", "", metric.metricLabel, t.mountedName); err != nil {
				fmt.Fprintf(ctx.Stderr(), "%v\n", err)
			} else {
				test.Pass(ctx, "%s, %s: %v\n", t.mountedName, metric.metricLabel, metric.value)
			}
		}
		results <- result
	}
}

func getAggregator(aggs map[string]*aggregator, metricLabel string) *aggregator {
	_, ok := aggs[metricLabel]
	if !ok {
		aggs[metricLabel] = newAggregator()
	}
	return aggs[metricLabel]
}

func getSysStat(v23ctx *context.T, ctx *tool.Context, me *naming.MountEntry, stat string) (float64, error) {
	me.Name = "" // Need this to make monitoring.GetStat work correctly.
	values, err := monitoring.GetStat(v23ctx, ctx, *me, stat)
	if err != nil {
		return -1, err
	}
	v := values[0]
	fv, err := v.GetFloat64Value()
	if err != nil {
		return -1, err
	}
	return fv, nil
}
