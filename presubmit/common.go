package main

import (
	"fmt"
	"net/url"

	"v.io/x/devtools/lib/testutil"
	"v.io/x/devtools/lib/util"
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
func postMessage(ctx *util.Context, message string, refs []string, success bool) error {
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
