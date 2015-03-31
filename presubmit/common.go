// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"net/url"

	"v.io/x/devtools/internal/testutil"
	"v.io/x/devtools/internal/tool"
)

func genStartPresubmitBuildLink(strRefs, strProjects, strTests string) string {
	return fmt.Sprintf("%s/%s/buildWithParameters?REFS=%s&PROJECTS=%s&TESTS=%s",
		jenkinsBaseJobUrl,
		presubmitTestJobFlag,
		url.QueryEscape(strRefs),
		url.QueryEscape(strProjects),
		url.QueryEscape(strTests))
}

// postMessage posts the given message to Gerrit.
func postMessage(ctx *tool.Context, message string, refs []string, success bool) error {
	// Basic sanity check for the Gerrit base URL.
	gerritHost, err := checkGerritBaseUrl()
	if err != nil {
		return err
	}

	// Parse .netrc file to get Gerrit credential.
	gerritCred, err := gerritHostCredential(gerritHost)
	if err != nil {
		return err
	}

	// Construct and post the reviews.
	refsUsingVerifiedLabel, err := getRefsUsingVerifiedLabel(ctx, gerritCred)
	if err != nil {
		return err
	}
	value := "1"
	if !success {
		value = "-" + value
	}
	gerrit := ctx.Gerrit(gerritBaseUrlFlag, gerritCred.username, gerritCred.password)
	for _, ref := range refs {
		labels := map[string]string{}
		if _, ok := refsUsingVerifiedLabel[ref]; ok {
			labels["Verified"] = value
		}
		if err := gerrit.PostReview(ref, message, labels); err != nil {
			return err
		}
		testutil.Pass(ctx, "review posted for %q with labels %v.\n", ref, labels)
	}
	return nil
}

// testNameWithPartSuffix generates a new name from the given test name and part index.
func testNameWithPartSuffix(testName string, partIndex int) string {
	if partIndex < 0 {
		return testName
	}
	return fmt.Sprintf("%s-part%d", testName, partIndex)
}
