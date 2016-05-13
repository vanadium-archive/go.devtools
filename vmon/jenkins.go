// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"time"

	cloudmonitoring "google.golang.org/api/monitoring/v3"
	"v.io/jiri/tool"
	"v.io/v23/context"
	"v.io/x/devtools/internal/test"
	"v.io/x/lib/gcm"
)

const (
	jenkinsHost = "http://127.0.0.1/jenkins"
)

func checkJenkins(v23ctx *context.T, ctx *tool.Context, s *cloudmonitoring.Service) error {
	// Query Jenkins for the last vanadium-go-build run.
	j, err := ctx.Jenkins(jenkinsHost)
	if err != nil {
		return err
	}
	info, err := j.BuildInfoForSpec(fmt.Sprintf("vanadium-go-build/lastBuild"))
	if err != nil {
		return err
	}
	now := time.Now()
	strNow := now.UTC().Format(time.RFC3339)
	ageInHours := now.Sub(time.Unix(info.Timestamp/1000, 0)).Hours()
	msg := fmt.Sprintf("vanadium-go-build age: %f hours.\n", ageInHours)

	// Send data to GCM.
	md, err := gcm.GetMetric("jenkins", projectFlag)
	if err != nil {
		return err
	}
	if err := sendDataToGCM(s, md, float64(ageInHours), strNow, "", "", "vanadium-go-build age"); err != nil {
		test.Fail(ctx, msg)
		return err
	}
	test.Pass(ctx, msg)

	return nil
}
