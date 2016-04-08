// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"reflect"
	"testing"
	"time"

	cloudmonitoring "google.golang.org/api/monitoring/v3"
)

func TestCalcIncidents(t *testing.T) {
	testCases := []struct {
		points               []*cloudmonitoring.Point
		expectedIncidentData []incidentData
	}{
		// No incidents.
		{
			points: []*cloudmonitoring.Point{
				&cloudmonitoring.Point{
					Interval: &cloudmonitoring.TimeInterval{
						StartTime: time.Unix(1429896102, 0).Format(time.RFC3339),
					},
					Value: &cloudmonitoring.TypedValue{
						DoubleValue: 1000,
					},
				},
				&cloudmonitoring.Point{
					Interval: &cloudmonitoring.TimeInterval{
						StartTime: time.Unix(1429896101, 0).Format(time.RFC3339),
					},
					Value: &cloudmonitoring.TypedValue{
						DoubleValue: 1000,
					},
				},
				&cloudmonitoring.Point{
					Interval: &cloudmonitoring.TimeInterval{
						StartTime: time.Unix(1429896100, 0).Format(time.RFC3339),
					},
					Value: &cloudmonitoring.TypedValue{
						DoubleValue: 1000,
					},
				},
			},
			expectedIncidentData: []incidentData{},
		},
		// One warning incident.
		{
			points: []*cloudmonitoring.Point{
				&cloudmonitoring.Point{
					Interval: &cloudmonitoring.TimeInterval{
						StartTime: time.Unix(1429896103, 0).Format(time.RFC3339),
					},
					Value: &cloudmonitoring.TypedValue{
						DoubleValue: 1000,
					},
				},
				&cloudmonitoring.Point{
					Interval: &cloudmonitoring.TimeInterval{
						StartTime: time.Unix(1429896102, 0).Format(time.RFC3339),
					},
					Value: &cloudmonitoring.TypedValue{
						DoubleValue: 3000,
					},
				},
				&cloudmonitoring.Point{
					Interval: &cloudmonitoring.TimeInterval{
						StartTime: time.Unix(1429896101, 0).Format(time.RFC3339),
					},
					Value: &cloudmonitoring.TypedValue{
						DoubleValue: 3000,
					},
				},
				&cloudmonitoring.Point{
					Interval: &cloudmonitoring.TimeInterval{
						StartTime: time.Unix(1429896100, 0).Format(time.RFC3339),
					},
					Value: &cloudmonitoring.TypedValue{
						DoubleValue: 1000,
					},
				},
			},
			expectedIncidentData: []incidentData{
				incidentData{
					Start:    1429896101,
					Duration: 2,
					Status:   serviceStatusWarning,
				},
			},
		},
		// One warning incident and one critical incident.
		{
			points: []*cloudmonitoring.Point{
				&cloudmonitoring.Point{
					Interval: &cloudmonitoring.TimeInterval{
						StartTime: time.Unix(1429896104, 0).Format(time.RFC3339),
					},
					Value: &cloudmonitoring.TypedValue{
						DoubleValue: 1000,
					},
				},
				&cloudmonitoring.Point{
					Interval: &cloudmonitoring.TimeInterval{
						StartTime: time.Unix(1429896103, 0).Format(time.RFC3339),
					},
					Value: &cloudmonitoring.TypedValue{
						DoubleValue: 3000,
					},
				},
				&cloudmonitoring.Point{
					Interval: &cloudmonitoring.TimeInterval{
						StartTime: time.Unix(1429896102, 0).Format(time.RFC3339),
					},
					Value: &cloudmonitoring.TypedValue{
						DoubleValue: 3000,
					},
				},
				&cloudmonitoring.Point{
					Interval: &cloudmonitoring.TimeInterval{
						StartTime: time.Unix(1429896101, 0).Format(time.RFC3339),
					},
					Value: &cloudmonitoring.TypedValue{
						DoubleValue: 5000,
					},
				},
				&cloudmonitoring.Point{
					Interval: &cloudmonitoring.TimeInterval{
						StartTime: time.Unix(1429896100, 0).Format(time.RFC3339),
					},
					Value: &cloudmonitoring.TypedValue{
						DoubleValue: 1000,
					},
				},
			},
			expectedIncidentData: []incidentData{
				incidentData{
					Start:    1429896101,
					Duration: 1,
					Status:   serviceStatusDown,
				},
				incidentData{
					Start:    1429896102,
					Duration: 2,
					Status:   serviceStatusWarning,
				},
			},
		},
		// One warning incident at the beginning and one critical incident at the end.
		{
			points: []*cloudmonitoring.Point{
				&cloudmonitoring.Point{
					Interval: &cloudmonitoring.TimeInterval{
						StartTime: time.Unix(1429896103, 0).Format(time.RFC3339),
					},
					Value: &cloudmonitoring.TypedValue{
						DoubleValue: 3000,
					},
				},
				&cloudmonitoring.Point{
					Interval: &cloudmonitoring.TimeInterval{
						StartTime: time.Unix(1429896102, 0).Format(time.RFC3339),
					},
					Value: &cloudmonitoring.TypedValue{
						DoubleValue: 3000,
					},
				},
				&cloudmonitoring.Point{
					Interval: &cloudmonitoring.TimeInterval{
						StartTime: time.Unix(1429896101, 0).Format(time.RFC3339),
					},
					Value: &cloudmonitoring.TypedValue{
						DoubleValue: 1000,
					},
				},
				&cloudmonitoring.Point{
					Interval: &cloudmonitoring.TimeInterval{
						StartTime: time.Unix(1429896100, 0).Format(time.RFC3339),
					},
					Value: &cloudmonitoring.TypedValue{
						DoubleValue: 5000,
					},
				},
			},
			expectedIncidentData: []incidentData{
				incidentData{
					Start:    1429896100,
					Duration: 1,
					Status:   serviceStatusDown,
				},
				incidentData{
					Start:    1429896102,
					Duration: 1,
					Status:   serviceStatusWarning,
				},
			},
		},
	}

	for index, test := range testCases {
		got, err := calcIncidents(test.points)
		if err != nil {
			t.Fatalf("index #%d: want no errors, got: %v", index, err)
		}
		if expected := test.expectedIncidentData; !reflect.DeepEqual(got, expected) {
			t.Fatalf("index #%d: want: %#v, got: %#v", index, expected, got)
		}
	}
}

func TestOverThresholdFor(t *testing.T) {
	testCases := []struct {
		timestamps     []int64
		values         []float64
		threshold      float64
		holdMinutes    int
		expectedResult bool
	}{
		// Not over threshold.
		{
			timestamps:     []int64{0, 60, 120, 180, 240, 300, 360},
			values:         []float64{0, 0, 0, 0, 0, 0, 0},
			threshold:      100,
			holdMinutes:    5,
			expectedResult: false,
		},
		// Over threshold, but not long enough.
		{
			timestamps:     []int64{0, 60, 120, 180, 240, 300, 360},
			values:         []float64{0, 0, 0, 0, 0, 200, 200},
			threshold:      100,
			holdMinutes:    5,
			expectedResult: false,
		},
		// Over threshold and long enough.
		{
			timestamps:     []int64{0, 60, 120, 180, 240, 300, 360},
			values:         []float64{0, 200, 200, 200, 200, 200, 200},
			threshold:      100,
			holdMinutes:    5,
			expectedResult: true,
		},
	}

	for _, test := range testCases {
		got := overThresholdFor(test.timestamps, test.values, test.threshold, test.holdMinutes)
		if got != test.expectedResult {
			t.Fatalf("want %v, got %v", test.expectedResult, got)
		}
	}
}

func TestAggregateMetricData(t *testing.T) {
	aggData := map[string]*aggMetricData{}
	testSteps := []struct {
		metric          *metricData
		expectedAggData map[string]*aggMetricData
	}{
		{
			metric: &metricData{
				Name:              "metric1",
				HistoryTimestamps: []int64{1, 2, 3},
				HistoryValues:     []float64{100.0, 200.0, 300.0},
			},
			expectedAggData: map[string]*aggMetricData{
				"metric1": &aggMetricData{
					TimestampsToValues: map[int64][]float64{
						1: []float64{100.0},
						2: []float64{200.0},
						3: []float64{300.0},
					},
				},
			},
		},
		{
			metric: &metricData{
				Name:              "metric1",
				HistoryTimestamps: []int64{1, 2},
				HistoryValues:     []float64{101.0, 201.0},
			},
			expectedAggData: map[string]*aggMetricData{
				"metric1": &aggMetricData{
					TimestampsToValues: map[int64][]float64{
						1: []float64{100.0, 101.0},
						2: []float64{200.0, 201.0},
						3: []float64{300.0},
					},
				},
			},
		},
		{
			metric: &metricData{
				Name:              "metric2",
				HistoryTimestamps: []int64{4, 5, 6},
				HistoryValues:     []float64{10.0, 20.0, 30.0},
			},
			expectedAggData: map[string]*aggMetricData{
				"metric1": &aggMetricData{
					TimestampsToValues: map[int64][]float64{
						1: []float64{100.0, 101.0},
						2: []float64{200.0, 201.0},
						3: []float64{300.0},
					},
				},
				"metric2": &aggMetricData{
					TimestampsToValues: map[int64][]float64{
						4: []float64{10.0},
						5: []float64{20.0},
						6: []float64{30.0},
					},
				},
			},
		},
	}
	for _, test := range testSteps {
		aggregateMetricData(aggData, test.metric)
		if got, want := aggData, test.expectedAggData; !reflect.DeepEqual(got, want) {
			t.Fatalf("want %v, got %v", want, got)
		}
	}
}

func TestCalculateMaxAndAverageData(t *testing.T) {
	aggData := map[string]*aggMetricData{
		"metric1": &aggMetricData{
			TimestampsToValues: map[int64][]float64{
				1: []float64{100.0, 200.0},
				2: []float64{200.0, 300.0},
				3: []float64{300.0},
			},
		},
		"metric2": &aggMetricData{
			TimestampsToValues: map[int64][]float64{
				4: []float64{10.0},
			},
		},
	}
	gotMaxData, gotMaxRangeData, gotAverageData, gotAverageRangeData := calculateMaxAndAverageData(aggData, "zone1")
	expectedMaxData := map[string]*metricData{
		"metric1": &metricData{
			ZoneName:          "zone1",
			Name:              "metric1",
			CurrentValue:      300.0,
			MinTime:           1,
			MaxTime:           3,
			MinValue:          200.0,
			MaxValue:          300.0,
			HistoryTimestamps: []int64{1, 2, 3},
			HistoryValues:     []float64{200.0, 300.0, 300.0},
			Threshold:         -1,
			Healthy:           true,
		},
		"metric2": &metricData{
			ZoneName:          "zone1",
			Name:              "metric2",
			CurrentValue:      10.0,
			MinTime:           4,
			MaxTime:           4,
			MinValue:          10.0,
			MaxValue:          10.0,
			HistoryTimestamps: []int64{4},
			HistoryValues:     []float64{10.0},
			Threshold:         -1,
			Healthy:           true,
		},
	}
	expectedMaxRangeData := &rangeData{
		MinTime: 1,
		MaxTime: 4,
	}
	expectedAverageData := map[string]*metricData{
		"metric1": &metricData{
			ZoneName:          "zone1",
			Name:              "metric1",
			CurrentValue:      300.0,
			MinTime:           1,
			MaxTime:           3,
			MinValue:          150.0,
			MaxValue:          300.0,
			HistoryTimestamps: []int64{1, 2, 3},
			HistoryValues:     []float64{150.0, 250.0, 300.0},
			Threshold:         -1,
			Healthy:           true,
		},
		"metric2": &metricData{
			ZoneName:          "zone1",
			Name:              "metric2",
			CurrentValue:      10.0,
			MinTime:           4,
			MaxTime:           4,
			MinValue:          10.0,
			MaxValue:          10.0,
			HistoryTimestamps: []int64{4},
			HistoryValues:     []float64{10.0},
			Threshold:         -1,
			Healthy:           true,
		},
	}
	expectedAverageRangeData := &rangeData{
		MinTime: 1,
		MaxTime: 4,
	}
	if !reflect.DeepEqual(gotMaxData, expectedMaxData) {
		t.Fatalf("want %#v, got %#v", expectedMaxData, gotMaxData)
	}
	if !reflect.DeepEqual(gotMaxRangeData, expectedMaxRangeData) {
		t.Fatalf("want %#v, got %#v", expectedMaxRangeData, gotMaxRangeData)
	}
	if !reflect.DeepEqual(gotAverageData, expectedAverageData) {
		t.Fatalf("want %#v, got %#v", expectedAverageData, gotAverageData)
	}
	if !reflect.DeepEqual(gotAverageRangeData, expectedAverageRangeData) {
		t.Fatalf("want %#v, got %#v", expectedAverageRangeData, gotAverageRangeData)
	}
}
