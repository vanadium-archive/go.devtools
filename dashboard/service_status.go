// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"v.io/jiri/lib/tool"
	"v.io/x/devtools/internal/cache"
)

const (
	statusBucket = "gs://vanadium-oncall/data"
)

const (
	hourDivWidth = 5
)

var statusPageTemplate = template.Must(template.New("status").Funcs(statusFuncMap).Parse(`
{{ $days := makeSlice 0 1 2 3 4 5 6 }}
<!DOCTYPE html>
<html>
	<head>
		<title>Vanadium Services Status</title>
		<link href="//fonts.googleapis.com/css?family=Source+Code+Pro:400,500|Roboto:500,400italic,300,500italic,300italic,400" rel="stylesheet" type="text/css">
		<link rel="stylesheet" href="/static/status.css">
		<script type="text/javascript" src="/static/status.js"></script>
	</head>
	<body>
		<div id="incident-details"></div>
		<div class="header">
			<div class="title">
				Vanadium Services Status
			</div>
		</div>
		<div class="main-container">
			<div class="main">
				<div class="header-row">
					<div class="header-current">Current</div>
					<div class="header-history">
						{{ range $dayIndex := $days }}
						<div class="day-label" style="left: {{ offsetPxForDay $.CollectionTimestamp $dayIndex }}">
							{{ dayLabel $.CollectionTimestamp $dayIndex }}
						</div>
						{{ end }}
					</div>
				</div>
				{{ range $serviceData := .Status }}
				<div class="service-row">
				  <div class="service-header">
		  			<div class="service-name">{{ $serviceData.Name }}</div>
						<div class="service-buildts">Built at: {{ $serviceData.BuildTimestamp }}</div>
						<div class="service-snapshot">Snapshot: {{ $serviceData.SnapshotLabel }}</div>
					</div>
					<div class="service-cur-status {{ $serviceData.CurrentStatus }}">
					</div>
					<div style="width: {{ offsetPxForDay $.CollectionTimestamp 7 }}" class="service-history">
						{{ range $dayIndex2 := $days }}
						<div class="day-divider" style="left: {{ offsetPxForDay $.CollectionTimestamp $dayIndex2 }}">
						</div>
						{{ end }}
						{{ range $incidentData := $serviceData.Incidents}}
						<div class="service-history-item {{ $incidentData.Status }}"
								 style="left: {{ offsetPx $.CollectionTimestamp $incidentData.Start }};
								        width: {{ widthPxForDuration $incidentData.Duration }}"
								 onmouseover="mouseOverIncidentItem(this, '{{ incidentDetails $incidentData.Start $incidentData.Duration }}')"
								 onmouseout="mouseOutIncidentItem()">
						</div>
						{{ end }}
					</div>
				</div>
				{{ end }}
			</div>
		</div>
	</body>
</html>
`))

var statusFuncMap = template.FuncMap{
	"dayLabel":           dayLabel,
	"incidentDetails":    incidentDetails,
	"makeSlice":          makeSlice,
	"offsetPx":           offsetPx,
	"offsetPxForDay":     offsetPxForDay,
	"widthPxForDuration": widthPxForDuration,
}

func dayLabel(collectionTime int64, dayIndex int) string {
	t := time.Unix(collectionTime, 0)
	roundedTime := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
	return time.Unix(roundedTime.Unix()-int64((dayIndex+1)*24*3600), 0).Format("Jan 02")
}

func incidentDetails(startTime, duration int64) string {
	return fmt.Sprintf("%s - %s", time.Unix(startTime, 0).Format("2006-01-02 15:04"), time.Unix(startTime+duration, 0).Format("2006-01-02 15:04"))
}

func makeSlice(args ...interface{}) []interface{} {
	return args
}

func offsetPx(collectionTime, curTime int64) string {
	offsetHours := float32(collectionTime-curTime) / 3600.0
	return fmt.Sprintf("%fpx", offsetHours*hourDivWidth)
}

func offsetPxForDay(collectionTime int64, dayIndex int) string {
	t := time.Unix(collectionTime, 0)
	roundedTime := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
	return offsetPx(collectionTime, roundedTime.Unix()-int64(dayIndex*24*3600))
}

func widthPxForDuration(duration int64) string {
	// 2px Minimum width.
	width := math.Max(2, float64(duration)/3600.0*hourDivWidth)
	return fmt.Sprintf("%fpx", width)
}

func displayServiceStatusPage(ctx *tool.Context, w http.ResponseWriter, r *http.Request) (e error) {
	// Set up the root directory.
	root := cacheFlag
	if root == "" {
		tmpDir, err := ctx.Run().TempDir("", "")
		if err != nil {
			return err
		}
		defer ctx.Run().RemoveAll(tmpDir)
		root = tmpDir
	}
	root = filepath.Join(root, "status")
	if err := ctx.Run().MkdirAll(root, 0700); err != nil {
		return err
	}

	// Read timestamp from the "latest" file.
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	if err := ctx.Run().CommandWithOpts(opts, "gsutil", "-q", "cat", statusBucketFlag+"/latest"); err != nil {
		return err
	}

	// Read status file.
	cachedFile, err := cache.StoreGoogleStorageFile(ctx, root, statusBucketFlag, out.String()+".status")
	if err != nil {
		return err
	}
	fileBytes, err := ctx.Run().ReadFile(cachedFile)
	if err != nil {
		return err
	}

	// Parse status file and render.
	type statusData struct {
		Name           string
		BuildTimestamp string
		SnapshotLabel  string
		CurrentStatus  string
		Incidents      []struct {
			Start    int64
			Duration int64
			Status   string
		}
	}
	var data struct {
		CollectionTimestamp int64
		Status              []statusData
	}
	if err := json.Unmarshal(fileBytes, &data); err != nil {
		return fmt.Errorf("Unmarshal(%v) failed: %v", string(fileBytes), err)
	}
	filteredStatus := []statusData{}
	for _, s := range data.Status {
		// Ignore application and binary repo.
		if s.Name == "application repository" || s.Name == "binary repository" {
			continue
		}
		s.Name = strings.ToUpper(s.Name)
		if s.BuildTimestamp == "" {
			s.BuildTimestamp = "N/A"
		}
		if s.SnapshotLabel == "" {
			s.SnapshotLabel = "N/A"
		}
		filteredStatus = append(filteredStatus, s)
	}
	data.Status = filteredStatus

	if err := statusPageTemplate.Execute(w, data); err != nil {
		return fmt.Errorf("Execute() failed: %v!!", err)
	}

	return nil
}
