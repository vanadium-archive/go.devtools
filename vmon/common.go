// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

type serviceLocation struct {
	instance string
	zone     string
}

var serviceLocationMap = map[string]*serviceLocation{
	"/ns.dev.v.io:8101": &serviceLocation{
		instance: "vanadium-cell-master",
		zone:     "us-central1-c",
	},
	"/ns.dev.staging.v.io:8101": &serviceLocation{
		instance: "vanadium-cell-master",
		zone:     "us-central1-c",
	},
}
