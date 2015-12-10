// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "fmt"

// Human-readable service names.
const (
	snMounttable       = "mounttable"
	snApplications     = "application repository"
	snBinaries         = "binary repository"
	snIdentity         = "identity service"
	snMacaroon         = "macaroon service"
	snGoogleIdentity   = "google identity service"
	snBinaryDischarger = "binary discharger"
	snRole             = "role service"
	snProxy            = "proxy service"
	snGroups           = "groups service"
)

// serviceMountedNames is a map from human-readable service names to their
// relative mounted names in the global mounttable.
var serviceMountedNames = map[string]string{
	snMounttable:       "",
	snApplications:     "applications",
	snBinaries:         "binaries",
	snIdentity:         "identity/dev.v.io:u",
	snMacaroon:         "identity/dev.v.io:u/macaroon",
	snGoogleIdentity:   "identity/dev.v.io:u/google",
	snBinaryDischarger: "identity/dev.v.io:u/discharger",
	snRole:             "identity/role",
	snProxy:            "proxy-mon",
	snGroups:           "groups",
}

func getMountedName(serviceName string) (string, error) {
	relativeName, ok := serviceMountedNames[serviceName]
	if !ok {
		return "", fmt.Errorf("service name %q not found", serviceName)
	}
	return fmt.Sprintf("%s/%s", namespaceRootFlag, relativeName), nil
}
