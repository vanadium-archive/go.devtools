// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"net/url"
	"sort"

	"v.io/jiri/jiri"
	"v.io/x/devtools/internal/test"
)

func gerritBaseUrl() (*url.URL, error) {
	u, err := url.Parse(gerritBaseUrlFlag)
	if err != nil {
		return nil, fmt.Errorf("invalid gerrit url %q: %v", gerritBaseUrlFlag, err)
	}
	return u, nil
}

func genStartPresubmitBuildLink(strRefs, strProjects, strTests string) string {
	return fmt.Sprintf("%s/%s/buildWithParameters?REFS=%s&PROJECTS=%s&TESTS=%s",
		jenkinsBaseJobUrl,
		presubmitTestJobFlag,
		url.QueryEscape(strRefs),
		url.QueryEscape(strProjects),
		url.QueryEscape(strTests))
}

// postMessage posts the given message to Gerrit.
func postMessage(jirix *jiri.X, message string, refs []string, success bool) error {
	refsUsingVerifiedLabel, err := getRefsUsingVerifiedLabel(jirix)
	if err != nil {
		return err
	}
	value := "1"
	if !success {
		value = "-" + value
	}
	for _, ref := range refs {
		labels := map[string]string{}
		if _, ok := refsUsingVerifiedLabel[ref]; ok {
			labels["Verified"] = value
		}
		gUrl, err := gerritBaseUrl()
		if err != nil {
			return err
		}
		if err := jirix.Gerrit(gUrl).PostReview(ref, message, labels); err != nil {
			return err
		}
		test.Pass(jirix.Context, "review posted for %q with labels %v.\n", ref, labels)
	}
	return nil
}

func getRefsUsingVerifiedLabel(jirix *jiri.X) (map[string]struct{}, error) {
	// Query all open CLs.
	gUrl, err := gerritBaseUrl()
	if err != nil {
		return nil, err
	}
	cls, err := jirix.Gerrit(gUrl).Query(defaultQueryString)
	if err != nil {
		return nil, err
	}

	// Identify the refs that use the "Verified" label.
	ret := map[string]struct{}{}
	for _, cl := range cls {
		if _, ok := cl.Labels["Verified"]; ok {
			ret[cl.Reference()] = struct{}{}
		}
	}

	return ret, nil
}

// getSubmittableCLs extracts CLs that have the AutoSubmit label in the commit
// message and satisfy all the submit rules. If a CL is part of a multi-part CLs
// set, all the CLs in that set need to be submittable. It returns a list of
// clLists each of which is either a single CL or a multi-part CLs set.
func getSubmittableCLs(jirix *jiri.X, cls clList) []clList {
	submittableCLs := []clList{}
	multiPartCLs := map[string]*multiPartCLSet{}
	for _, cl := range cls {
		// Check whether a CL satisfies all the submit rules. We do this by checking
		// the states of all its labels.
		//
		// cl.Labels has the following data structure:
		// {
		//   "Code-Review": {
		//     "approved": struct{}{},
		//   },
		//   "Verified": {
		//     "rejected": struct{}{},
		//   }
		//   ...
		// }
		// A label is satisfied/green when it has an "approved" entry.
		allLabelsApproved := true
		for label, labelData := range cl.Labels {
			// We only check the following labels which might not exist
			// at the same time.
			switch label {
			case "Code-Review", "Verified", "Non-Author-Code-Review", "To-Be-Reviewed":
				isApproved := false
				for state := range labelData {
					if state == "approved" {
						isApproved = true
						break
					}
				}
				if !isApproved {
					allLabelsApproved = false
					break
				}
			}
		}
		if cl.AutoSubmit && allLabelsApproved {
			if cl.MultiPart != nil {
				topic := cl.MultiPart.Topic
				if _, ok := multiPartCLs[topic]; !ok {
					multiPartCLs[topic] = NewMultiPartCLSet()
				}
				multiPartCLs[topic].addCL(cl)
			} else {
				submittableCLs = append(submittableCLs, clList{cl})
			}
		}
	}

	// This is to make sure we have consistent results order.
	sortedTopics := []string{}
	for topic := range multiPartCLs {
		sortedTopics = append(sortedTopics, topic)
	}
	sort.Strings(sortedTopics)

	// Find complete multi part cl sets.
	for _, topic := range sortedTopics {
		set := multiPartCLs[topic]
		if set.complete() {
			submittableCLs = append(submittableCLs, set.cls())
		}
	}

	return submittableCLs
}

// submitCLs submits the given CLs.
func submitCLs(jirix *jiri.X, cls clList) error {
	for _, cl := range cls {
		curRef := cl.Reference()
		msg := fmt.Sprintf("submit CL: %s\n", curRef)
		gUrl, err := gerritBaseUrl()
		if err != nil {
			return err
		}
		if err := jirix.Gerrit(gUrl).Submit(cl.Change_id); err != nil {
			test.Fail(jirix.Context, msg)
			fmt.Fprintf(jirix.Stderr(), "%v\n", err)
			if err := postMessage(jirix, fmt.Sprintf("Failed to submit CL:\n%v\n", err), []string{curRef}, true); err != nil {
				return err
			}
		} else {
			test.Pass(jirix.Context, msg)
		}
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
