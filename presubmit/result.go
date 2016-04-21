// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"v.io/jiri"
	"v.io/jiri/jenkins"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/xunit"
	"v.io/x/devtools/tooldata"
	"v.io/x/lib/cmdline"
	"v.io/x/lib/set"
)

type testStatus int

func (s testStatus) String() string {
	switch s {
	case statusUnknown:
		return "?"
	case statusSuccess:
		return "✔"
	default:
		return "✖"
	}
}

func stringToTestStatus(s string) testStatus {
	switch s {
	case unknownStatusString:
		return statusUnknown
	case successStatusString:
		return statusSuccess
	default:
		return statusFail
	}
}

// Constants used for aggregating test status for tests that have multiple parts.
const (
	statusUnknown testStatus = iota
	statusSuccess
	statusFail
)

// testResultSummary stores data for generating summary for a test.
type testResultSummary struct {
	testNameWithLabels string // labels include os and architecture.
	lastStatus         testStatus
	curStatus          testStatus
	timeoutValue       time.Duration
}

var (
	dashboardHostFlag string
	projectsFlag      string
	reviewMessageFlag string

	unknownStatusString = "UNKNOWN"
	successStatusString = "SUCCESS"
)

func init() {
	cmdResult.Flags.StringVar(&dashboardHostFlag, "dashboard-host", "https://dashboard.v.io", "The host of the dashboard server.")
	cmdResult.Flags.StringVar(&projectsFlag, "projects", "", "The base names of the remote projects containing the CLs pointed by the refs, separated by ':'.")
	cmdResult.Flags.StringVar(&reviewTargetRefsFlag, "refs", "", "The review references separated by ':'.")
	cmdResult.Flags.IntVar(&jenkinsBuildNumberFlag, "build-number", -1, "The number of the Jenkins build.")

	tool.InitializeProjectFlags(&cmdResult.Flags)
}

// cmdResult represents the 'result' command of the presubmit tool.
var cmdResult = &cmdline.Command{
	Name:  "result",
	Short: "Process and post test results",
	Long: `
Result processes all the test statuses and results files collected from all the
presubmit test configuration builds, creates a result summary, and posts the
summary back to the corresponding Gerrit review thread.
`,
	Runner: jiri.RunnerFunc(runResult),
}

type testResultInfo struct {
	Result           test.Result
	TestName         string // This is the test name without the part suffix (vanadium-go-race).
	Timestamp        int64
	PostSubmitResult string
	AxisValues       axisValuesInfo
}

type axisValuesInfo struct {
	Arch      string
	OS        string
	PartIndex int
}

// genBuildSpec returns a spec string for the given Jenkins build.
//
// If the main job is a multi-configuration job, the spec is in the form of:
// <jobName>/axis1Label=axis1Value,axis2Label=axis2Value,.../<suffix>
// The axis values are taken from the given axisValuesInfo object, and only
// the axes set in the job's axisInfo object will appear in the spec.
//
// If the main job is not a multi-configuration job, the spec will be:
// <jobName>/<suffix>.
func genBuildSpec(jobName string, axisValues axisValuesInfo, suffix string, matrixJobsConf map[string]tooldata.JenkinsMatrixJobInfo) string {
	axis, ok := matrixJobsConf[jobName]

	// Not a multi-configuration job.
	if !ok {
		return fmt.Sprintf("%s/%s", jobName, suffix)
	}

	// Multi-configuration job.
	// The axis order doesn't matter.
	parts := []string{}
	if axis.HasArch {
		parts = append(parts, fmt.Sprintf("ARCH=%s", axisValues.Arch))
	}
	if axis.HasOS {
		parts = append(parts, fmt.Sprintf("OS=%s", axisValues.OS))
	}
	if axis.HasParts {
		parts = append(parts, fmt.Sprintf("P=%d", axisValues.PartIndex))
	}
	return fmt.Sprintf("%s/%s/%s", jobName, strings.Join(parts, ","), suffix)
}

// genSubJobLabel returns a descriptive label for given Jenkins job's sub-job.
// For more info, please see comments of the subJobSpec method above.
func genSubJobLabel(jobName string, axisValues axisValuesInfo, matrixJobsConf map[string]tooldata.JenkinsMatrixJobInfo) string {
	axis, ok := matrixJobsConf[jobName]

	// Not a multi-configuration job.
	if !ok {
		return ""
	}

	// Multi-configuration job.
	parts := []string{}
	if axis.HasOS && axis.ShowOS {
		parts = append(parts, axisValues.OS)
	}
	if axis.HasArch {
		parts = append(parts, axisValues.Arch)
	}
	// Note that we omit the part index here to make parts transparent to users.
	return strings.Join(parts, ",")
}

// key returns a unique key for the test wrapped in the given
// testResultInfo object.
func (ri testResultInfo) key() string {
	return fmt.Sprintf("%s_%s_%s_%d", ri.TestName, ri.AxisValues.OS, ri.AxisValues.Arch, ri.AxisValues.PartIndex)
}

// runResult implements the 'result' subcommand.
//
// In the new presubmit "master" job, the collected results related files are
// organized using the following structure:
//
// ${WORKSPACE}
// ├── root
// └── test_results
//     ├── 45    (build number)
//     │    ├── ARCH=amd64,OS=linux,TEST=vanadium-go-build
//     │    │   ├── status_vanadium_go_build.json
//     │    │   └─- tests_vanadium_go_build.xml
//     │    ├── ARCH=amd64,OS=linux,TEST=vanadium-go-test
//     │    │   ├── status_vanadium_go_test.json
//     │    │   └─- tests_vanadium_go_test.xml
//     │    ├── ARCH=386,OS=mac,TEST=vanadium-go-build
//     │    │   ├── status_vanadium_go_build.json
//     │    │   └─- tests_vanadium_go_build.xml
//     │    ├── ARCH=amd64,OS=linux,TEST=vanadium-go-race_part0
//     │    │   ├── status_vanadium_go_race.json
//     │    │   └─- tests_vanadium_go_race.xml
//     │    └── ...
//     ├── 46
//     ...
//
// The .json files record the test status (a test.TestResult object), and
// the .xml files are xUnit reports.
//
// Each individual presubmit test will generate the .json file and the .xml file
// at the end of their run, and the presubmit "master" job is configured to
// collect all those files and store them in the above directory structure.
func runResult(jirix *jiri.X, args []string) (e error) {
	// Load Jenkins matrix jobs config.
	config, err := tooldata.LoadConfig(jirix)
	if err != nil {
		return err
	}
	matrixJobsConf := config.JenkinsMatrixJobs()

	// Find all status files and store their paths in a slice.
	workspaceDir := os.Getenv("WORKSPACE")
	curTestResultsDir := filepath.Join(workspaceDir, "test_results", fmt.Sprintf("%d", jenkinsBuildNumberFlag))
	statusFiles := []string{}
	filepath.Walk(curTestResultsDir, func(path string, info os.FileInfo, err error) error {
		fileName := info.Name()
		if strings.HasPrefix(fileName, "status_") && strings.HasSuffix(fileName, ".json") {
			statusFiles = append(statusFiles, path)
		}
		return nil
	})

	// Read status files and add them to the "results" map below.
	sort.Strings(statusFiles)
	testResults := []testResultInfo{}
	for _, statusFile := range statusFiles {
		bytes, err := ioutil.ReadFile(statusFile)
		if err != nil {
			return fmt.Errorf("ReadFile(%v) failed: %v", statusFile, err)
		}
		var curResult testResultInfo
		if err := json.Unmarshal(bytes, &curResult); err != nil {
			return fmt.Errorf("Unmarshal() failed: %v", err)
		}
		testResults = append(testResults, curResult)
	}

	// Post results.
	refs := strings.Split(reviewTargetRefsFlag, ":")
	postSubmitResults, err := getPostSubmitBuildData(jirix, testResults, matrixJobsConf)
	if err != nil {
		return err
	}
	reporter := testReporter{matrixJobsConf, testResults, postSubmitResults, refs, &bytes.Buffer{}}
	if allTestsPassed, err := reporter.postReport(jirix); err != nil {
		return err
	} else if allTestsPassed {
		if err := submitPresubmitCLs(jirix, refs); err != nil {
			fmt.Fprintf(jirix.Stderr(), "%v\n", err)
		}
	}

	// Process result files.
	return processRemoteTestResults(jirix)
}

// getPostSubmitBuildData returns a map from job names to the data of the
// corresponding postsubmit builds that ran before the recorded test result
// timestamps.
func getPostSubmitBuildData(jirix *jiri.X, testResults []testResultInfo, matrixJobsConf map[string]tooldata.JenkinsMatrixJobInfo) (map[string]*postSubmitBuildData, error) {
	jenkinsObj, err := jirix.Jenkins(jenkinsHostFlag)
	if err != nil {
		return nil, err
	}

	data := map[string]*postSubmitBuildData{}
outer:
	for _, resultInfo := range testResults {
		name := resultInfo.TestName
		timestamp := resultInfo.Timestamp
		axisValues := resultInfo.AxisValues
		fmt.Fprintf(jirix.Stdout(), "Getting postsubmit build info for %q before timestamp %d...\n", resultInfo.key(), timestamp)

		buildInfo, err := lastCompletedBuildStatus(jirix, name, axisValues, matrixJobsConf)
		if err != nil {
			test.Fail(jirix.Context, "%v\n", err)
			continue
		}
		curIdStr := buildInfo.Id
		curId, err := strconv.Atoi(curIdStr)
		if err != nil {
			test.Fail(jirix.Context, "Atoi(%v) failed: %v\n", curIdStr, err)
			continue
		}
		for i := curId; i >= 0; i-- {
			fmt.Fprintf(jirix.Stdout(), "Checking build %d...\n", i)
			buildSpec := genBuildSpec(name, resultInfo.AxisValues, fmt.Sprintf("%d", i), matrixJobsConf)
			curBuildInfo, err := jenkinsObj.BuildInfoForSpec(buildSpec)
			if err != nil {
				test.Fail(jirix.Context, "%v\n", err)
				continue outer
			}
			if curBuildInfo.Timestamp > timestamp {
				continue
			}
			// "cases" will be empty on error.
			cases, _ := jenkinsObj.FailedTestCasesForBuildSpec(buildSpec)
			test.Pass(jirix.Context, "Got build status of build %d: %s\n", i, curBuildInfo.Result)
			data[resultInfo.key()] = &postSubmitBuildData{
				result:          curBuildInfo.Result,
				failedTestCases: cases,
			}
			break
		}
	}
	return data, nil
}

type testReporter struct {
	// matrixJobsConf stores configs for Jenkins matrix jobs.
	matrixJobsConf map[string]tooldata.JenkinsMatrixJobInfo
	// testResults stores presubmit results to report.
	testResults []testResultInfo
	// postSubmitResults stores postsubmit results (indexed by test names) used to
	// compare with the presubmit results.
	postSubmitResults map[string]*postSubmitBuildData
	// refs identifies the references to post the report to.
	refs []string
	// report stores the report content.
	report *bytes.Buffer
}

type postSubmitBuildData struct {
	result          string
	failedTestCases []jenkins.TestCase
}

// postReport generates a test report and posts it to Gerrit.
// It returns whether the presubmit test is considered successful.
func (r *testReporter) postReport(jirix *jiri.X) (bool, error) {
	// Do not post a test report if no tests were run.
	if len(r.testResults) == 0 {
		return true, nil
	}

	printf(jirix.Stdout(), "### Preparing report\n")

	if r.reportFailedPresubmitBuild(jirix) {
		return false, nil
	}

	// Report failures from presubmit itself.
	// If any failures are found and reported, don't generate any further report.
	if r.reportPresubmitFailure(jirix) {
		return false, nil
	}

	r.reportOncall(jirix)

	failedTestNames := map[string]struct{}{}
	numNewFailures := 0
	if failedTestNames = r.reportTestResultsSummary(jirix); len(failedTestNames) != 0 {
		// Report failed test cases grouped by failure types.
		var err error
		if numNewFailures, err = r.reportFailedTestCases(jirix); err != nil {
			return false, err
		}
	}

	r.reportUsefulLinks(failedTestNames)

	printf(jirix.Stdout(), "### Posting test results to Gerrit\n")
	success := numNewFailures == 0
	if err := postMessage(jirix, r.report.String(), r.refs, success); err != nil {
		return false, err
	}
	return success, nil
}

// reportFailedPresubmitBuild reports a failed presubmit build.
// It returns whether the presubmit build failed or not.
//
// In theory, a failed presubmit master build won't even execute the
// result reporting step (the cmdResult command implemented in this file),
// but just in case.
func (r *testReporter) reportFailedPresubmitBuild(jirix *jiri.X) bool {
	jenkins, err := jirix.Jenkins(jenkinsHostFlag)
	if err != nil {
		fmt.Fprintf(jirix.Stderr(), "%v\n", err)
		return false
	}

	masterJobInfo, err := jenkins.BuildInfo(presubmitTestJobFlag, jenkinsBuildNumberFlag)
	if err != nil {
		fmt.Fprintf(jirix.Stderr(), "%v\n", err)
		return false
	}
	if masterJobInfo.Result == "FAILURE" {
		fmt.Fprintf(r.report, "SOME TESTS FAILED TO RUN.\nRetrying...\n")
		return true
	}
	return false
}

// reportPresubmitFailure posts a review about failure from presubmit itself
// (not from the test it runs).
func (r *testReporter) reportPresubmitFailure(jirix *jiri.X) bool {
	for _, resultInfo := range r.testResults {
		message := ""
		switch resultInfo.Result.Status {
		case test.MergeConflict:
			message = fmt.Sprintf(mergeConflictMessageTmpl, resultInfo.Result.MergeConflictCL)
		case test.ToolsBuildFailure:
			message = fmt.Sprintf(toolsBuildFailureMessageTmpl, resultInfo.Result.ToolsBuildFailureMsg)
		}

		if message != "" {
			if err := postMessage(jirix, message, r.refs, false); err != nil {
				printf(jirix.Stderr(), "%v\n", err)
			}
			return true
		}
	}
	return false
}

// reportOncall reports current vanadium oncall.
func (r *testReporter) reportOncall(jirix *jiri.X) {
	shift, err := tooldata.Oncall(jirix, time.Now())
	if err != nil {
		fmt.Fprintf(jirix.Stderr(), "%v\n", err)
	} else {
		fmt.Fprintf(r.report, "\nCurrent Oncall: %s, %s\n\n", shift.Primary, shift.Secondary)
	}
}

// reportTestResultsSummary populates the given buffer with a test
// results summary (one transition for each test) and returns a list of
// failed tests.
func (r *testReporter) reportTestResultsSummary(jirix *jiri.X) map[string]struct{} {
	fmt.Fprintf(r.report, "Test results:\n")
	// This set will be used to generate the "retry failed tests only" link where
	// we should use the names with the part suffix.
	failedTests := map[string]struct{}{}

	// The "test key" is testName+os+arch.
	testResultSummaries := map[string]*testResultSummary{}
	// For tests with multiple parts, we'd like to show a single summary line for
	// all their parts. To do this, we aggregate test status/results data for all
	// their parts first.
	for _, resultInfo := range r.testResults {
		name := resultInfo.TestName
		result := resultInfo.Result
		if result.Status == test.Skipped {
			fmt.Fprintf(r.report, "skipped %v\n", name)
			continue
		}

		testKey := fmt.Sprintf("%s_%s_%s", name, resultInfo.AxisValues.OS, resultInfo.AxisValues.Arch)
		summary := testResultSummaries[testKey]
		if summary == nil {
			// Generate test name with labels (os, architecture, etc).
			// It is ok to initialize this string using any part of the multi-part
			// tests as the part index is not used by the initialization.
			nameString := name
			subJobLabel := genSubJobLabel(name, resultInfo.AxisValues, r.matrixJobsConf)
			if subJobLabel != "" {
				nameString += fmt.Sprintf(" [%s]", subJobLabel)
			}
			summary = &testResultSummary{
				testNameWithLabels: nameString,
				timeoutValue:       -1,
			}
			testResultSummaries[testKey] = summary
		}
		if testFailed := r.mergeTestResults(resultInfo, summary); testFailed {
			failedTests[testNameWithPartSuffix(name, resultInfo.AxisValues.PartIndex)] = struct{}{}
		}
	}

	// Generate one summary line for each aggregated test.
	nameStrings := []string{}
	nameStringToSummaryLine := map[string]string{}
	for _, summary := range testResultSummaries {
		var lineBuf bytes.Buffer
		nameString := summary.testNameWithLabels
		fmt.Fprintf(&lineBuf, "%s ➔ %s: %s", summary.lastStatus.String(), summary.curStatus.String(), nameString)
		if summary.timeoutValue > 0 {
			fmt.Fprintf(&lineBuf, " [TIMED OUT after %s]\n", summary.timeoutValue)
		} else {
			fmt.Fprintf(&lineBuf, "\n")
		}
		nameStrings = append(nameStrings, nameString)
		nameStringToSummaryLine[nameString] = lineBuf.String()
	}

	// Sort summary lines by test name strings and output them to the report.
	sort.Strings(nameStrings)
	for _, n := range nameStrings {
		fmt.Fprintf(r.report, "%s", nameStringToSummaryLine[n])
	}
	return failedTests
}

// mergeTestResults merges the given test result data to the given test summary.
// It returns whether the given test fails.
func (r *testReporter) mergeTestResults(resultInfo testResultInfo, summary *testResultSummary) bool {
	result := resultInfo.Result
	testFailed := false

	// Get the status of the corresponding postsubmit test.
	lastStatus := statusUnknown
	if data := r.postSubmitResults[resultInfo.key()]; data != nil {
		lastStatus = stringToTestStatus(data.result)
	}
	// The aggregated test status is:
	// - FAILED if any of the individual statuses is FAILED.
	// - SUCCESS if none of the individual status is FAILED and any of the
	//   individual status is SUCCESS.
	// - UNKNOWN otherwise.
	if lastStatus > summary.lastStatus {
		summary.lastStatus = lastStatus
	}

	// Get the status of the current presubmit test.
	curStatus := statusUnknown
	if result.Status == test.Passed {
		curStatus = statusSuccess
	} else {
		testFailed = true
		curStatus = statusFail
	}
	if curStatus > summary.curStatus {
		summary.curStatus = curStatus
	}

	// Timeout value.
	if result.Status == test.TimedOut {
		timeoutValue := test.DefaultTimeout
		if result.TimeoutValue != 0 {
			timeoutValue = result.TimeoutValue
		}
		if timeoutValue > summary.timeoutValue {
			summary.timeoutValue = timeoutValue
		}
	}

	return testFailed
}

// lastCompletedBuildStatus gets the status of the last completed
// build for a given Jenkins job.
func lastCompletedBuildStatus(jirix *jiri.X, jobName string, axisValues axisValuesInfo, matrixJobsConf map[string]tooldata.JenkinsMatrixJobInfo) (*jenkins.BuildInfo, error) {
	jenkins, err := jirix.Jenkins(jenkinsHostFlag)
	if err != nil {
		return nil, err
	}

	buildSpec := genBuildSpec(jobName, axisValues, "lastCompletedBuild", matrixJobsConf)
	buildInfo, err := jenkins.BuildInfoForSpec(buildSpec)
	if err != nil {
		return nil, err
	}
	return buildInfo, nil
}

type failureType int

const (
	fixedFailure failureType = iota
	newFailure
	knownFailure
)

func (t failureType) String() string {
	switch t {
	case fixedFailure:
		return "FIXED FAILURE"
	case newFailure:
		return "NEW FAILURE"
	case knownFailure:
		return "KNOWN FAILURE"
	default:
		return "UNKNOWN FAILURE TYPE"
	}
}

// failedTestLinks maps from failure type to links.
type failedTestLinksMap map[failureType][]string

// reportFailedTestCasesByFailureTypes reports failed test cases grouped by
// failure types: new failures, known failures, and fixed failures.
func (r *testReporter) reportFailedTestCases(jirix *jiri.X) (int, error) {
	// Get groups.
	groups, err := r.genFailedTestCasesGroupsForAllTests(jirix)
	if err != nil {
		return -1, err
	}

	// Generate links for all groups.
	for _, failureType := range []failureType{newFailure, knownFailure, fixedFailure} {
		failedTestCaseInfos, ok := groups[failureType]
		if !ok || len(failedTestCaseInfos) == 0 {
			continue
		}
		failureTypeStr := failureType.String()
		if len(failedTestCaseInfos) > 1 {
			failureTypeStr += "S"
		}
		curLinks := []string{}
		for _, testCase := range failedTestCaseInfos {
			curLink := genTestResultLink(testCase.suiteName, testCase.className, testCase.testCaseName, testCase.testName, testCase.axisValues)
			curLinks = append(curLinks, curLink)
		}
		fmt.Fprintf(r.report, "\n%s:\n%s\n\n", failureTypeStr, strings.Join(curLinks, "\n"))
	}

	return len(groups[newFailure]), nil
}

type failedTestCaseInfo struct {
	suiteName    string
	className    string
	testCaseName string
	testName     string
	axisValues   axisValuesInfo
}

type failedTestCasesGroups map[failureType][]failedTestCaseInfo

// genFailedTestCasesGroupsForAllTests iterate all tests from the given
// testResults, compares the presubmit failed test cases (read from the given
// xUnit report) with the postsubmit failed test cases, and groups the failed
// tests into three groups: new failures, known failures, and fixed failures.
// Each group has a slice of failedTestLinkInfo which is used to generate
// dashboard links.
func (r *testReporter) genFailedTestCasesGroupsForAllTests(jirix *jiri.X) (failedTestCasesGroups, error) {
	groups := failedTestCasesGroups{}

	for _, testResult := range r.testResults {
		axisValues := testResult.AxisValues
		// For a given test script this-is-a-test.sh, its test
		// report file is: tests_this_is_a_test.xml.
		xUnitReportFileName := fmt.Sprintf("tests_%s.xml", strings.Replace(testResult.TestName, "-", "_", -1))
		// The collected xUnit test report is located at:
		// $WORKSPACE/test_results/$buildNumber/ARCH=amd64,OS=$OS,TEST=$testNameWithPartSuffix/tests_xxx.xml
		//
		// See more details in result.go.
		xUnitReportFile := filepath.Join(
			os.Getenv("WORKSPACE"),
			"test_results",
			fmt.Sprintf("%d", jenkinsBuildNumberFlag),
			fmt.Sprintf("ARCH=%s,OS=%s,TEST=%s", axisValues.Arch, axisValues.OS, testNameWithPartSuffix(testResult.TestName, testResult.AxisValues.PartIndex)),
			xUnitReportFileName)
		bytes, err := ioutil.ReadFile(xUnitReportFile)
		if err != nil {
			// It is normal that certain tests don't have report available.
			printf(jirix.Stderr(), "ReadFile(%v) failed: %v\n", xUnitReportFile, err)
			continue
		}

		// Get the failed test cases from the corresponding postsubmit Jenkins job
		// to compare with the presubmit failed tests.
		postsubmitFailedTestCases := []jenkins.TestCase{}
		if data := r.postSubmitResults[testResult.key()]; data != nil {
			postsubmitFailedTestCases = data.failedTestCases
		}
		curFailedTestCasesGroups, err := r.genFailedTestCasesGroupsForOneTest(jirix, testResult, bytes, postsubmitFailedTestCases)
		if err != nil {
			printf(jirix.Stderr(), "%v\n", err)
			continue
		}
		for curFailureType, curFailedTestCaseInfos := range *curFailedTestCasesGroups {
			groups[curFailureType] = append(groups[curFailureType], curFailedTestCaseInfos...)
		}
	}
	return groups, nil
}

// genFailedTestCasesGroupsForOneTest generates groups for failed tests.
// See comments of genFailedTestsGroupsForAllTests.
func (r *testReporter) genFailedTestCasesGroupsForOneTest(jirix *jiri.X, testResult testResultInfo, presubmitXUnitReport []byte, postsubmitFailedTestCases []jenkins.TestCase) (*failedTestCasesGroups, error) {
	testName := testResult.TestName

	// Parse xUnit report of the presubmit test.
	suites := xunit.TestSuites{}
	if err := xml.Unmarshal(presubmitXUnitReport, &suites); err != nil {
		return nil, fmt.Errorf("Unmarshal(%v) failed: %v", string(presubmitXUnitReport), err)
	}

	groups := failedTestCasesGroups{}
	curFailedTestCases := []jenkins.TestCase{}
	for _, curTestSuite := range suites.Suites {
		for _, curTestCase := range curTestSuite.Cases {
			// Unescape test name and class name.
			curTestCase.Classname = html.UnescapeString(curTestCase.Classname)
			curTestCase.Name = html.UnescapeString(curTestCase.Name)
			// A failed test.
			if len(curTestCase.Failures) > 0 {
				linkInfo := failedTestCaseInfo{
					suiteName:    curTestSuite.Name,
					className:    curTestCase.Classname,
					testCaseName: curTestCase.Name,
					testName:     testName,
					axisValues:   testResult.AxisValues,
				}
				// Determine whether the curTestCase is a new failure or not.
				isNewFailure := true
				for _, postsubmitFailedTestCase := range postsubmitFailedTestCases {
					curClassName := curTestCase.Classname
					if curClassName == "" {
						curClassName = curTestSuite.Name
					}
					if curClassName == postsubmitFailedTestCase.ClassName && curTestCase.Name == postsubmitFailedTestCase.Name {
						isNewFailure = false
						break
					}
				}
				if isNewFailure {
					groups[newFailure] = append(groups[newFailure], linkInfo)
				} else {
					groups[knownFailure] = append(groups[knownFailure], linkInfo)
				}
				curFailedTestCases = append(curFailedTestCases, jenkins.TestCase{
					ClassName: curTestCase.Classname,
					Name:      curTestCase.Name,
				})
			}
		}
	}
	// Populate fixed failure group.
	for _, postsubmitFailedTestCase := range postsubmitFailedTestCases {
		fixed := true
		for _, curFailedTestCase := range curFailedTestCases {
			if postsubmitFailedTestCase.Equal(curFailedTestCase) {
				fixed = false
				break
			}
		}
		if fixed {
			groups[fixedFailure] = append(groups[fixedFailure], failedTestCaseInfo{
				className:    postsubmitFailedTestCase.ClassName,
				testCaseName: postsubmitFailedTestCase.Name,
			})
		}
	}
	return &groups, nil
}

// genTestResultLink generates a link to a dashboard page for the given failed test case.
func genTestResultLink(suiteName, className, testCaseName string, testName string, axisValues axisValuesInfo) string {
	testFullName := genTestFullName(className, testCaseName)
	u, err := url.Parse(dashboardHostFlag + "/")
	if err != nil {
		return fmt.Sprintf("- %s\n  Result link not available (%v)", testFullName, err)
	}
	partIndex := axisValues.PartIndex
	// For tests without multi-parts, set its partIndex to 0.
	if partIndex < 0 {
		partIndex = 0
	}
	q := u.Query()
	q.Set("type", "presubmit")
	q.Set("n", fmt.Sprintf("%d", jenkinsBuildNumberFlag))
	q.Set("arch", axisValues.Arch)
	q.Set("os", axisValues.OS)
	q.Set("part", fmt.Sprintf("%d", partIndex))
	q.Set("job", testName)
	q.Set("suite", suiteName)
	q.Set("class", className)
	q.Set("test", testCaseName)
	u.RawQuery = q.Encode()
	return fmt.Sprintf("- %s\n%s", testFullName, u.String())
}

func genTestFullName(className, testName string) string {
	testFullName := fmt.Sprintf("%s.%s", className, testName)
	// Replace the period "." in testFullName with
	// "::" to stop gmail from turning it into a
	// link automatically.
	return strings.Replace(testFullName, ".", "::", -1)
}

// reportUsefulLinks reports useful links:
// - Current presubmit-test master status page.
// - Retry failed tests only.
// - Retry current build.
func (r *testReporter) reportUsefulLinks(failedTestNames map[string]struct{}) {
	fmt.Fprintf(r.report, "\nMore details at:\n%s/?type=presubmit&n=%d\n", dashboardHostFlag, jenkinsBuildNumberFlag)
	if len(failedTestNames) > 0 {
		// Generate link to retry failed tests only.
		names := set.String.ToSlice(failedTestNames)
		link := genStartPresubmitBuildLink(reviewTargetRefsFlag, projectsFlag, strings.Join(names, " "))
		fmt.Fprintf(r.report, "\nTo re-run FAILED TESTS ONLY without uploading a new patch set:\n(click Proceed button on the next screen)\n%s\n", link)

		// Generate link to retry the whole presubmit test.
		link = genStartPresubmitBuildLink(reviewTargetRefsFlag, projectsFlag, os.Getenv("TESTS"))
		fmt.Fprintf(r.report, "\nTo re-run presubmit tests without uploading a new patch set:\n(click Proceed button on the next screen)\n%s\n", link)
	}
}

// submitPresubmitCLs tries to submit CLs in the current presubmit test.
func submitPresubmitCLs(jirix *jiri.X, refs []string) error {
	// Query open CLs.
	gUrl, err := gerritBaseUrl()
	if err != nil {
		return err
	}
	openCLs, err := jirix.Gerrit(gUrl).Query(defaultQueryString)
	if err != nil {
		return err
	}

	// Check whether all of the current CLs (refs) are in one of the
	// submittable CL lists. If so, submit that whole CL list.
	submittableCLs := getSubmittableCLs(jirix, openCLs)
	for _, curCLList := range submittableCLs {
		refsSet := map[string]struct{}{}
		for _, cl := range curCLList {
			refsSet[cl.Reference()] = struct{}{}
		}
		allRefsSubmittable := true
		for _, ref := range refs {
			if _, ok := refsSet[ref]; !ok {
				allRefsSubmittable = false
				break
			}
		}
		if allRefsSubmittable {
			if err := submitCLs(jirix, curCLList); err != nil {
				return err
			}
			break
		}
	}

	return nil
}

// processRemoteTestResults copies result files to a local tmp dir, compress
// them, and upload the tar file.
func processRemoteTestResults(jirix *jiri.X) error {
	// Check the existence of the remote results dir.
	// If it doesn't exist, it means the test phase failed (e.g. merge conflict).
	// We don't fail the "result" phase in those cases.
	s := jirix.NewSeq()
	remoteResultsPath := gsPrefix + fmt.Sprintf("presubmit/%d", jenkinsBuildNumberFlag)
	if err := s.Last("gsutil", "ls", remoteResultsPath); err != nil {
		fmt.Fprintf(jirix.Stderr(), "Results not exist: %s\n", remoteResultsPath)
		return nil
	}

	tmp, err := s.TempDir("", "")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	tarFile := "results.tar.gz"
	return s.
		MkdirAll(tmp, 0755).
		Chdir(tmp).
		Run("gsutil", "-m", "cp", "-r", remoteResultsPath, ".").
		Run("tar", "-zcf", tarFile, fmt.Sprintf("%d", jenkinsBuildNumberFlag)).
		Last("gsutil", "cp", tarFile, remoteResultsPath)
}
