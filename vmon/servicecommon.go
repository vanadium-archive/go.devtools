// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"math"
	"strings"

	cloudmonitoring "google.golang.org/api/monitoring/v3"

	"v.io/jiri/tool"
	"v.io/x/devtools/internal/test"
)

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
	if _, err := s.Projects.TimeSeries.Create(fmt.Sprintf("projects/%s", projectFlag), &cloudmonitoring.CreateTimeSeriesRequest{
		TimeSeries: []*cloudmonitoring.TimeSeries{
			&cloudmonitoring.TimeSeries{
				Metric: &cloudmonitoring.Metric{
					Type:   md.Type,
					Labels: labelsMap,
				},
				Points: []*cloudmonitoring.Point{
					&cloudmonitoring.Point{
						Value: &cloudmonitoring.TypedValue{
							DoubleValue: value,
						},
						Interval: &cloudmonitoring.TimeInterval{
							StartTime: now,
							EndTime:   now,
						},
					},
				},
			},
		}}).Do(); err != nil {
		return fmt.Errorf("Timeseries Write failed for metric %q with value %f and labels %v: %v", md.Name, value, labels, err)
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
