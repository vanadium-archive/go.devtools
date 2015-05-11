// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"reflect"
	"testing"
	"time"

	"google.golang.org/api/cloudmonitoring/v2beta2"
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
					DoubleValue: 1000,
					Start:       time.Unix(1429896102, 0).Format(time.RFC3339),
				},
				&cloudmonitoring.Point{
					DoubleValue: 1000,
					Start:       time.Unix(1429896101, 0).Format(time.RFC3339),
				},
				&cloudmonitoring.Point{
					DoubleValue: 1000,
					Start:       time.Unix(1429896100, 0).Format(time.RFC3339),
				},
			},
			expectedIncidentData: []incidentData{},
		},
		// One warning incident.
		{
			points: []*cloudmonitoring.Point{
				&cloudmonitoring.Point{
					DoubleValue: 1000,
					Start:       time.Unix(1429896103, 0).Format(time.RFC3339),
				},
				&cloudmonitoring.Point{
					DoubleValue: 3000,
					Start:       time.Unix(1429896102, 0).Format(time.RFC3339),
				},
				&cloudmonitoring.Point{
					DoubleValue: 3000,
					Start:       time.Unix(1429896101, 0).Format(time.RFC3339),
				},
				&cloudmonitoring.Point{
					DoubleValue: 1000,
					Start:       time.Unix(1429896100, 0).Format(time.RFC3339),
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
					DoubleValue: 1000,
					Start:       time.Unix(1429896104, 0).Format(time.RFC3339),
				},
				&cloudmonitoring.Point{
					DoubleValue: 3000,
					Start:       time.Unix(1429896103, 0).Format(time.RFC3339),
				},
				&cloudmonitoring.Point{
					DoubleValue: 3000,
					Start:       time.Unix(1429896102, 0).Format(time.RFC3339),
				},
				&cloudmonitoring.Point{
					DoubleValue: 5000,
					Start:       time.Unix(1429896101, 0).Format(time.RFC3339),
				},
				&cloudmonitoring.Point{
					DoubleValue: 1000,
					Start:       time.Unix(1429896100, 0).Format(time.RFC3339),
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
					DoubleValue: 3000,
					Start:       time.Unix(1429896103, 0).Format(time.RFC3339),
				},
				&cloudmonitoring.Point{
					DoubleValue: 3000,
					Start:       time.Unix(1429896102, 0).Format(time.RFC3339),
				},
				&cloudmonitoring.Point{
					DoubleValue: 1000,
					Start:       time.Unix(1429896101, 0).Format(time.RFC3339),
				},
				&cloudmonitoring.Point{
					DoubleValue: 5000,
					Start:       time.Unix(1429896100, 0).Format(time.RFC3339),
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
