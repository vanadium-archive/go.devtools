// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"v.io/jiri/collect"
	"v.io/jiri/gerrit"
	"v.io/jiri/jiri"
	"v.io/jiri/project"
	"v.io/jiri/runutil"
	"v.io/jiri/tool"
	"v.io/jiri/util"
	"v.io/x/lib/cmdline"
)

const (
	defaultLogFilePath = "${HOME}/tmp/presubmit_log"
)

var (
	queryStringFlag string
	logFilePathFlag string
)

func init() {
	cmdQuery.Flags.StringVar(&queryStringFlag, "query", defaultQueryString, "The string used to query Gerrit for open CLs.")
	cmdQuery.Flags.StringVar(&logFilePathFlag, "log-file", os.ExpandEnv(defaultLogFilePath), "The file that stores the refs from the previous Gerrit query.")
	cmdQuery.Flags.Lookup("log-file").DefValue = defaultLogFilePath

	tool.InitializeProjectFlags(&cmdQuery.Flags)
}

type clList []gerrit.Change

// clRefMap indexes cls by their ref strings.
type clRefMap map[string]gerrit.Change

// clNumberToPatchsetMap is a map from CL numbers to the latest patchset of the CL.
type clNumberToPatchsetMap map[int]int

// multiPartCLSet represents a set of CLs that spans multiple projects.
type multiPartCLSet struct {
	parts         map[int]gerrit.Change // Indexed by cl's part index.
	expectedTotal int
	expectedTopic string
}

// NewMultiPartCLSet creates a new instance of multiPartCLSet.
func NewMultiPartCLSet() *multiPartCLSet {
	return &multiPartCLSet{
		parts:         map[int]gerrit.Change{},
		expectedTotal: -1,
		expectedTopic: "",
	}
}

// addCL adds a CL to the set after it passes a series of checks.
func (s *multiPartCLSet) addCL(cl gerrit.Change) error {
	if cl.MultiPart == nil {
		return fmt.Errorf("no multi part info found: %#v", cl)
	}
	multiPartInfo := cl.MultiPart
	if s.expectedTotal < 0 {
		s.expectedTotal = multiPartInfo.Total
	}
	if s.expectedTopic == "" {
		s.expectedTopic = multiPartInfo.Topic
	}
	if s.expectedTotal != multiPartInfo.Total {
		return fmt.Errorf("inconsistent total number of cls in this set: want %d, got %d", s.expectedTotal, multiPartInfo.Total)
	}
	if s.expectedTopic != multiPartInfo.Topic {
		return fmt.Errorf("inconsistent cl topics in this set: want %s, got %s", s.expectedTopic, multiPartInfo.Topic)
	}
	if existingCL, ok := s.parts[multiPartInfo.Index]; ok {
		return fmt.Errorf("duplicated cl part %d found:\ncl to add: %v\nexisting cl:%v", multiPartInfo.Index, cl, existingCL)
	}
	s.parts[multiPartInfo.Index] = cl
	return nil
}

// complete returns whether the current set has all the cl parts it needs.
func (s *multiPartCLSet) complete() bool {
	return len(s.parts) == s.expectedTotal
}

// cls returns a list of CLs in this set sorted by their part number.
func (s *multiPartCLSet) cls() clList {
	ret := clList{}
	sortedKeys := []int{}
	for part := range s.parts {
		sortedKeys = append(sortedKeys, part)
	}
	sort.Ints(sortedKeys)
	for _, part := range sortedKeys {
		ret = append(ret, s.parts[part])
	}
	return ret
}

// cmdQuery represents the 'query' command of the presubmit tool.
var cmdQuery = &cmdline.Command{
	Name:  "query",
	Short: "Query open CLs from Gerrit",
	Long: `
This subcommand queries open CLs from Gerrit, calculates diffs from the previous
query results, and sends each one with related metadata (ref, project, changeId)
to a Jenkins job which will run tests against the corresponding CL and post
review with test results.
`,
	Runner: jiri.RunnerFunc(runQuery),
}

// runQuery implements the "query" subcommand.
func runQuery(jirix *jiri.X, args []string) error {
	numSentCLs := 0
	defer func() {
		printf(jirix.Stdout(), "%d sent.\n", numSentCLs)
	}()

	// Load Jenkins matrix jobs config.
	config, err := util.LoadConfig(jirix)
	if err != nil {
		return err
	}
	matrixJobsConf := config.JenkinsMatrixJobs()

	// Don't query anything if the last "presubmit-test" build failed.
	lastBuildInfo, err := lastCompletedBuildStatus(jirix, presubmitTestJobFlag, axisValuesInfo{}, matrixJobsConf)
	if err != nil {
		fmt.Fprintf(jirix.Stderr(), "%v\n", err)
	} else {
		if lastBuildInfo.Result == "FAILURE" {
			printf(jirix.Stdout(), "%s is failing. Skipping this round.\n", presubmitTestJobFlag)
			return nil
		}
	}

	// Read previous CLs from the log file.
	prevCLsMap, err := readLog()
	if err != nil {
		return err
	}

	// Query Gerrit.
	gUrl, err := gerritBaseUrl()
	if err != nil {
		return err
	}
	curCLs, err := jirix.Gerrit(gUrl).Query(queryStringFlag)
	if err != nil {
		return fmt.Errorf("Query(%q) failed: %v", queryStringFlag, err)
	}

	// Write current CLs to the log file.
	err = writeLog(jirix, curCLs)
	if err != nil {
		return err
	}

	// Don't send anything if jenkins host is not specified.
	if jenkinsHostFlag == "" {
		printf(jirix.Stdout(), "Not sending CLs to run presubmit tests due to empty Jenkins host.\n")
		return nil
	}

	// Don't send anything if prevCLsMap is empty.
	if len(prevCLsMap) == 0 {
		printf(jirix.Stdout(), "Not sending CLs to run presubmit tests due to empty log file.\n")
		return nil
	}

	// Get new clLists.
	newCLLists := newOpenCLs(jirix, prevCLsMap, curCLs)

	// Send the new open CLs one by one to the given Jenkins
	// project to run presubmit-test builds.
	projects, _, err := project.ReadJiriManifest(jirix)
	if err != nil {
		return err
	}
	sender := clsSender{
		clLists:          newCLLists,
		projects:         projects,
		clsSent:          0,
		removeOutdatedFn: removeOutdatedBuilds,
		addPresubmitFn:   addPresubmitTestBuild,
		postMessageFn:    postMessage,
	}
	if err := sender.sendCLListsToPresubmitTest(jirix); err != nil {
		return err
	}
	numSentCLs += sender.clsSent

	// Get all submittable CLs and submit them.
	submittableCLs := getSubmittableCLs(jirix, curCLs)
	if len(submittableCLs) > 0 {
		fmt.Fprintf(jirix.Stdout(), "Submitting CLs...\n")
	}
	for _, curCLList := range submittableCLs {
		if err := submitCLs(jirix, curCLList); err != nil {
			return err
		}
	}

	return nil
}

// readLog returns CLs indexed by thier refs stored in the log file.
func readLog() (clRefMap, error) {
	results := clRefMap{}
	path := logFilePathFlag
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		if runutil.IsNotExist(err) {
			return results, nil
		}
		return nil, fmt.Errorf("ReadFile(%q) failed: %v", path, err)
	}

	if err := json.Unmarshal(bytes, &results); err != nil {
		return nil, fmt.Errorf("Unmarshal failed: %v\n%v", err, string(bytes))
	}
	return results, nil
}

// writeLog writes the refs of the given CLs to the log file.
func writeLog(jirix *jiri.X, cls clList) (e error) {
	// Index CLs with their refs.
	results := clRefMap{}
	for _, cl := range cls {
		results[cl.Reference()] = cl
	}
	path := logFilePathFlag
	fd, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return fmt.Errorf("OpenFile(%q) failed: %v", path, err)
	}
	defer collect.Error(func() error { return fd.Close() }, &e)

	bytes, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("MarshalIndent(%v) failed: %v", results, err)
	}
	if err := jirix.NewSeq().WriteFile(path, bytes, os.FileMode(0644)).Done(); err != nil {
		return fmt.Errorf("WriteFile(%q) failed: %v", path, err)
	}
	return nil
}

// newOpenCLs returns a slice of clLists that are "newer" relative to the
// previous query. A clList is newer if one of the following condition holds:
// - If a clList has only one cl, then it is newer if:
//   * Its ref string cannot be found among the CLs from the previous query.
//
//   For example: from the previous query, we got cl 1000/1 (cl number 1000 and
//   patchset 1). Then clLists [1000/2] and [2000/1] are both newer.
//
// - If a clList has multiple CLs, then it is newer if:
//   * It forms a "consistent" (its CLs have the same topic) and "complete"
//     (it contains all the parts) multi-part CL set.
//   * At least one of their ref strings cannot be found in the CLs from the
//     previous query.
//
//   For example: from the previous query, we got cl 3001/1 which is the first
//   part of a multi part cl set with topic "T1". Suppose the current query
//   returns cl 3002/1 which is the second part of the same set. In this case,
//   a clList [3001/1 3002/1] will be returned. Then suppose in the next query,
//   we got cl 3002/2 which is newer then 3002/1. In this case, a clList
//   [3001/1 3002/2] will be returned.
func newOpenCLs(jirix *jiri.X, prevCLsMap clRefMap, curCLs clList) []clList {
	newCLs := []clList{}
	topicsInNewCLs := map[string]struct{}{}
	multiPartCLs := clList{}
	for _, curCL := range curCLs {
		// Ref could be empty in cases where a patchset is causing conflicts.
		if curCL.Reference() == "" {
			continue
		}
		if _, ok := prevCLsMap[curCL.Reference()]; !ok {
			// This individual cl is newer.
			if curCL.MultiPart == nil {
				// This cl is not a multi part cl.
				// Add it to the return slice.
				newCLs = append(newCLs, clList{curCL})
			} else {
				// This cl is a multi part cl.
				// Record its topic.
				topicsInNewCLs[curCL.MultiPart.Topic] = struct{}{}
			}
		}
		// Record all multi part CLs.
		if curCL.MultiPart != nil {
			multiPartCLs = append(multiPartCLs, curCL)
		}
	}

	// Find complete multi part cl sets.
	setMap := map[string]*multiPartCLSet{}
	for _, curCL := range multiPartCLs {
		multiPartInfo := curCL.MultiPart

		// Skip topics that contain no new CLs.
		topic := multiPartInfo.Topic
		if _, ok := topicsInNewCLs[topic]; !ok {
			continue
		}

		if _, ok := setMap[topic]; !ok {
			setMap[topic] = NewMultiPartCLSet()
		}
		curSet := setMap[topic]
		if err := curSet.addCL(curCL); err != nil {
			curCLRef := curCL.Reference()
			message := fmt.Sprintf("failed to process multi-part CL %s:\n%v\n", curCLRef, err.Error())
			if err := postMessage(jirix, message, []string{curCLRef}, false); err != nil {
				printf(jirix.Stderr(), "%v\n", err)
			}
			printf(jirix.Stderr(), "%v\n", err)
		}
	}
	for _, set := range setMap {
		if set.complete() {
			newCLs = append(newCLs, set.cls())
		}
	}

	return newCLs
}

type clsSender struct {
	clLists          []clList
	projects         project.Projects
	clsSent          int
	removeOutdatedFn func(*jiri.X, clNumberToPatchsetMap) []error
	addPresubmitFn   func(*jiri.X, clList, []string) error
	postMessageFn    func(*jiri.X, string, []string, bool) error
}

// sendCLListsToPresubmitTest sends the given clLists to presubmit-test Jenkins
// job one by one to run presubmit-test builds. It returns how many CLs have
// been sent successfully.
func (s *clsSender) sendCLListsToPresubmitTest(jirix *jiri.X) error {
	for _, curCLList := range s.clLists {
		clListInfo := s.processCLList(jirix, curCLList)
		curCLList = clListInfo.filteredCLList
		if len(curCLList) == 0 {
			printf(jirix.Stdout(), "SKIP: Empty CL set\n")
			continue
		}

		// Don't send curCLList to presubmit-test if at least one of them
		// have PresubmitTest set to none.
		if clListInfo.skipPresubmitTest {
			// Set verified+1 label.
			if err := s.postMessageFn(jirix, "Presubmit tests skipped.\n", clListInfo.refs, true); err != nil {
				return err
			}
			printf(jirix.Stdout(), "SKIP: Add %s (presubmit=none)\n", clListInfo.clString)
			continue
		}

		// Skip if there is no tests to run.
		tests, err := s.getTestsToRun(jirix, clListInfo.projects)
		if err != nil {
			return err
		}
		if len(tests) == 0 {
			// Set verified+1 label when there is no tests to run.
			if err := s.postMessageFn(jirix, "No tests found.\n", clListInfo.refs, true); err != nil {
				return err
			}
			printf(jirix.Stdout(), "SKIP: Add %s (no tests found)\n", clListInfo.clString)
			continue
		}

		// Don't send curCLList to presubmit-test if at least one of them
		// has an non-google owner. Instead, post a link that one of our
		// team members has to click to trigger the presubmit-test manually.
		if clListInfo.hasNonGoogleOwner {
			if err := s.handleNonGoogleOwner(jirix, clListInfo.refs, clListInfo.projects, tests); err != nil {
				return err
			}
			printf(jirix.Stdout(), "SKIP: Add %s (non-google owner)\n", clListInfo.clString)
			continue
		}

		// Check and cancel matched outdated builds.
		for _, err := range s.removeOutdatedFn(jirix, clListInfo.clMap) {
			if err != nil {
				printf(jirix.Stderr(), "%v\n", err)
			}
		}

		// Send curCLList to presubmit-test.
		strCLs := fmt.Sprintf("Add %s", clListInfo.clString)
		if err := s.addPresubmitFn(jirix, curCLList, tests); err != nil {
			printf(jirix.Stdout(), "FAIL: %s\n", strCLs)
			printf(jirix.Stderr(), "addPresubmitTestBuild failed: %v\n", err)
		} else {
			printf(jirix.Stdout(), "PASS: %s\n", strCLs)
			s.clsSent += len(curCLList)
		}
	}
	return nil

}

type clListInfo struct {
	clMap             clNumberToPatchsetMap
	clString          string
	skipPresubmitTest bool
	hasNonGoogleOwner bool
	projects          []string
	refs              []string
	filteredCLList    clList
}

func (s *clsSender) processCLList(jirix *jiri.X, curCLList clList) *clListInfo {
	curCLMap := clNumberToPatchsetMap{}
	clStrings := []string{}
	skipPresubmitTest := false
	hasNonGoogleOwner := false
	projects := []string{}
	refs := []string{}
	filteredCLList := clList{}
	for _, curCL := range curCLList {
		// Ignore all CLs that are not in projects identified by the manifestFlag.
		// TODO(jingjin): find a better way so we can remove this check.
		if s.projects != nil && !isKnownProject(jirix, curCL, s.projects) {
			continue
		}
		filteredCLList = append(filteredCLList, curCL)

		cl, patchset, err := parseRefString(curCL.Reference())
		if err != nil {
			printf(jirix.Stderr(), "%v\n", err)
			return nil
		}
		curCLMap[cl] = patchset
		clStrings = append(clStrings, fmt.Sprintf("http://go/vcl/%d/%d", cl, patchset))

		if curCL.PresubmitTest == gerrit.PresubmitTestTypeNone {
			skipPresubmitTest = true
		}

		if !strings.HasSuffix(curCL.OwnerEmail(), "@google.com") {
			hasNonGoogleOwner = true
		}

		projects = append(projects, curCL.Project)
		refs = append(refs, curCL.Reference())
	}
	return &clListInfo{
		clMap:             curCLMap,
		clString:          strings.Join(clStrings, ", "),
		skipPresubmitTest: skipPresubmitTest,
		hasNonGoogleOwner: hasNonGoogleOwner,
		projects:          projects,
		refs:              refs,
		filteredCLList:    filteredCLList,
	}
}

func (s *clsSender) getTestsToRun(jirix *jiri.X, projects []string) ([]string, error) {
	config, err := util.LoadConfig(jirix)
	if err != nil {
		return nil, err
	}
	tmpTests := config.ProjectTests(projects)
	tests := []string{}
	// Append the part suffix to tests that have multiple parts specified in the config file.
	for _, test := range tmpTests {
		if parts := config.TestParts(test); parts != nil {
			for i := 0; i <= len(parts); i++ {
				tests = append(tests, testNameWithPartSuffix(test, i))
			}
		} else {
			tests = append(tests, test)
		}
	}
	sort.Strings(tests)
	return tests, nil
}

func (s *clsSender) handleNonGoogleOwner(jirix *jiri.X, refs, projects, tests []string) error {
	link := genStartPresubmitBuildLink(strings.Join(refs, ":"), strings.Join(projects, ":"), strings.Join(tests, " "))
	message := fmt.Sprintf("A Vanadium team member will manually trigger presubmit tests for this change:\n%s\n", link)
	if err := s.postMessageFn(jirix, message, refs, false); err != nil {
		return err
	}
	return nil
}

// isKnownProject checks whether the given cl's project is in the
// given set of projects.
func isKnownProject(jirix *jiri.X, cl gerrit.Change, projects project.Projects) bool {
	foundProjects := projects.Find(cl.Project)
	if len(foundProjects) == 0 {
		printf(jirix.Stdout(), "project=%q (%s) not found. Skipped.\n", cl.Project, cl.Reference())
		return false
	}
	return true
}

// removeOutdatedBuilds removes all the outdated presubmit-test builds
// that have the given cl number and equal or smaller patchset
// number. Outdated builds include queued builds and ongoing build.
//
// Since this is not a critical operation, we simply print out the
// errors if we see any.
func removeOutdatedBuilds(jirix *jiri.X, cls clNumberToPatchsetMap) (errs []error) {
	collect.Errors(func() error { return removeQueuedOutdatedBuilds(jirix, cls) }, &errs)
	collect.Errors(func() error { return removeOngoingOutdatedBuilds(jirix, cls) }, &errs)
	return
}

func removeQueuedOutdatedBuilds(jirix *jiri.X, cls clNumberToPatchsetMap) error {
	jenkins, err := jirix.Jenkins(jenkinsHostFlag)
	if err != nil {
		return err
	}

	// Get queued outdated builds.
	queuedBuilds, err := jenkins.QueuedBuilds(presubmitTestJobFlag)
	if err != nil {
		return err
	}

	for _, build := range queuedBuilds {
		refs := build.ParseRefs()
		if refs == "" {
			return err
		}
		buildOutdated, err := isBuildOutdated(refs, cls)
		if err != nil {
			return err
		}
		if buildOutdated {
			if err := jenkins.CancelQueuedBuild(fmt.Sprintf("%d", build.Id)); err != nil {
				return err
			}
			printf(jirix.Stdout(), "Cancelled build %s as it is no longer current.\n", refs)
		}
	}
	return nil
}

func removeOngoingOutdatedBuilds(jirix *jiri.X, cls clNumberToPatchsetMap) error {
	jenkins, err := jirix.Jenkins(jenkinsHostFlag)
	if err != nil {
		return err
	}

	buildInfos, err := jenkins.OngoingBuilds(presubmitTestJobFlag)
	if err != nil {
		return err
	}

	for _, buildInfo := range buildInfos {
		if !buildInfo.Building {
			continue
		}
		refs := buildInfo.ParseRefs()
		if refs != "" {
			buildOutdated, err := isBuildOutdated(refs, cls)
			if err != nil {
				fmt.Fprintf(jirix.Stderr(), "%v\n", err)
				continue
			}
			// Cancel outdated running build.
			if buildOutdated {
				if err := jenkins.CancelOngoingBuild(presubmitTestJobFlag, buildInfo.Number); err != nil {
					return err
				}
				printf(jirix.Stdout(), "Cancelled build %s as it is no longer current.\n", refs)
			}
		}
	}
	return nil
}

// isBuildOutdated checks whether a build (identified by the given refs string)
// is older than the cls in newCLs.
// Note that curRefs may contain multiple ref strings separated by ":".
func isBuildOutdated(curRefs string, newCLs clNumberToPatchsetMap) (bool, error) {
	// Parse the refs string into a clNumberToPatchsetMap object.
	curCLs := clNumberToPatchsetMap{}
	refs := strings.Split(curRefs, ":")
	for _, ref := range refs {
		cl, patchset, err := parseRefString(ref)
		if err != nil {
			return false, err
		}
		curCLs[cl] = patchset
	}

	// Check curCLs and newCLs have the same set of cl numbers.
	newCLNumbers := sortedKeys(newCLs)
	if !reflect.DeepEqual(sortedKeys(curCLs), newCLNumbers) {
		// curCLs are outdated when curCLs and newCLs have overlapping refs.
		// For example: curCLs = {1000/1}, and newCLs = {1000/2, 2000/1}.
		// In this case, 1000/1 becomes part of the MultiPart CLs, which makes
		// 1000/1 outdated.
		for curCLNumber, curPatchset := range curCLs {
			if newPatchset, ok := newCLs[curCLNumber]; ok && newPatchset >= curPatchset {
				return true, nil
			}
		}
		return false, nil
	}

	// Check patchsets.
	outdated := true
	for _, clNumber := range newCLNumbers {
		curPatchset := curCLs[clNumber]
		newPatchset := newCLs[clNumber]
		if newPatchset < curPatchset {
			outdated = false
			break
		}
	}
	return outdated, nil
}

func sortedKeys(cls clNumberToPatchsetMap) []int {
	keys := []int{}
	for k := range cls {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

// parseRefString parses the cl and patchset number from the given ref string.
func parseRefString(ref string) (int, int, error) {
	parts := strings.Split(ref, "/")
	if expected, got := 5, len(parts); expected != got {
		return -1, -1, fmt.Errorf("unexpected number of %q parts: expected %v, got %v", ref, expected, got)
	}
	cl, err := strconv.Atoi(parts[3])
	if err != nil {
		return -1, -1, fmt.Errorf("Atoi(%q) failed: %v", parts[3], err)
	}
	patchset, err := strconv.Atoi(parts[4])
	if err != nil {
		return -1, -1, fmt.Errorf("Atoi(%q) failed: %v", parts[4], err)
	}
	return cl, patchset, nil
}

// addPresubmitTestBuild uses Jenkins' remote access API to add a build for
// a set of open CLs to run presubmit tests.
func addPresubmitTestBuild(jirix *jiri.X, cls clList, tests []string) error {
	jenkins, err := jirix.Jenkins(jenkinsHostFlag)
	if err != nil {
		return err
	}

	refs, projects := []string{}, []string{}
	for _, cl := range cls {
		refs = append(refs, cl.Reference())
		projects = append(projects, cl.Project)
	}
	if err := jenkins.AddBuildWithParameter(presubmitTestJobFlag, url.Values{
		"REFS":     {strings.Join(refs, ":")},
		"PROJECTS": {strings.Join(projects, ":")},
		// Separating by spaces is required by the Dynamic Axis plugin used in the
		// new presubmit test target.
		"TESTS": {strings.Join(tests, " ")},
	}); err != nil {
		return err
	}
	return nil
}
