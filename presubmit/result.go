package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"v.io/lib/cmdline"
	"v.io/tools/lib/testutil"
	"v.io/tools/lib/util"
)

// cmdResult represents the 'result' command of the presubmit tool.
var cmdResult = &cmdline.Command{
	Name:  "result",
	Short: "Process and post test results.",
	Long: `
Result processes all the test statuses and results files collected from all the
presubmit test configuration builds, creates a result summary, and posts the
summary back to the corresponding Gerrit review thread.
`,
	Run: runResult,
}

var (
	subJobDirRE = regexp.MustCompile(".*L=(.*),TEST=.*")
)

// runResult implements the 'result' subcommand.
//
// In the new presubmit "master" job, the collected results related files are
// organized using the following structure:
//
// ${WORKSPACE}
// ├── root
// └── test_results
//     ├── 45    (build number)
//     │    ├── L=linux-slave,TEST=vanadium-go-build
//     │    │   ├── status_vanadium_go_build.json
//     │    │   └─- tests_vanadium_go_build.xml
//     │    ├── L=linux-slave,TEST=vanadium-go-test
//     │    │   ├── status_vanadium_go_test.json
//     │    │   └─- tests_vanadium_go_test.xml
//     │    ├── L=mac-slave,TEST=vanadium-go-build
//     │    │   ├── status_vanadium_go_build.json
//     │    │   └─- tests_vanadium_go_build.xml
//     │    └── ...
//     ├── 46
//     ...
//
// The .json files record the test status (a testutil.TestResult object), and
// the .xml files are xUnit reports.
//
// Each individual presubmit test will generate the .json file and the .xml file
// at the end of their run, and the presubmit "master" job is configured to
// collect all those files and store them in the above directory structure.
func runResult(command *cmdline.Command, args []string) (e error) {
	ctx := util.NewContextFromCommand(command, !noColorFlag, dryRunFlag, verboseFlag)

	// Process test status files.
	workspaceDir := os.Getenv("WORKSPACE")
	curTestResultsDir := filepath.Join(workspaceDir, "test_results", fmt.Sprintf("%d", jenkinsBuildNumberFlag))
	// Store all status file paths in a map indexed by slave labels.
	statusFiles := map[string][]string{}
	filepath.Walk(curTestResultsDir, func(path string, info os.FileInfo, err error) error {
		fileName := info.Name()
		if strings.HasPrefix(fileName, "status_") && strings.HasSuffix(fileName, ".json") {
			// Find the slave label from the file path.
			if matches := subJobDirRE.FindStringSubmatch(path); matches != nil {
				slaveLabel := matches[1]
				statusFiles[slaveLabel] = append(statusFiles[slaveLabel], path)
			}
		}
		return nil
	})

	// Read status files and add them to the "results" map below.
	results := map[string]testutil.TestResult{}
	names := []string{}
	for slaveLabel, curStatusFiles := range statusFiles {
		for _, statusFile := range curStatusFiles {
			bytes, err := ioutil.ReadFile(statusFile)
			if err != nil {
				return fmt.Errorf("ReadFile(%v) failed: %v", statusFile, err)
			}
			curResult := map[string]testutil.TestResult{}
			if err := json.Unmarshal(bytes, &curResult); err != nil {
				return fmt.Errorf("Unmarshal() failed: %v", err)
			}
			// The key of the "results" map is "${testName}|${slaveLabel}" so we can
			// sort them nicely when generating the "testResults" slice below.
			for t, r := range curResult {
				name := t + "|" + slaveLabel
				results[name] = r
				names = append(names, name)
			}
		}
	}

	// Create testResultInfo slice sorted by names.
	sort.Strings(names)
	testResults := []testResultInfo{}
	for _, name := range names {
		parts := strings.Split(name, "|")
		testResults = append(testResults, testResultInfo{
			result:     results[name],
			testName:   parts[0],
			slaveLabel: parts[1],
		})
	}

	// Post results.
	refs := strings.Split(reviewTargetRefsFlag, ":")
	if err := postTestReport(ctx, testResults, refs); err != nil {
		return err
	}

	return nil
}

// postTestReport generates a test report and posts it to Gerrit.
func postTestReport(ctx *util.Context, testResults []testResultInfo, refs []string) (e error) {
	// Do not post a test report if no tests were run.
	if len(testResults) == 0 {
		return nil
	}

	// Report current build cop.
	var report bytes.Buffer
	buildCop, err := util.BuildCop(ctx, time.Now())
	if err != nil {
		fmt.Fprintf(ctx.Stderr(), "%v\n", err)
	} else {
		fmt.Fprintf(&report, "\nCurrent Build Cop: %s\n\n", buildCop)
	}

	// Report merge conflict.
	for _, resultInfo := range testResults {
		if resultInfo.result.Status == testutil.TestFailedMergeConflict {
			message := fmt.Sprintf(mergeConflictMessageTmpl, resultInfo.result.MergeConflictCL)
			if err := postMessage(ctx, message, refs); err != nil {
				printf(ctx.Stderr(), "%v\n", err)
			}
			return nil
		}
	}

	// Report test results.
	fmt.Fprintf(&report, "Test results:\n")
	nfailed := 0
	for _, resultInfo := range testResults {
		name := resultInfo.testName
		result := resultInfo.result
		if result.Status == testutil.TestSkipped {
			fmt.Fprintf(&report, "skipped %v\n", name)
			continue
		}

		// Get the status of the last completed build for this Jenkins test.
		lastStatus, err := lastCompletedBuildStatus(name, resultInfo.slaveLabel)
		lastStatusString := "?"
		if err != nil {
			fmt.Fprintf(ctx.Stderr(), "%v\n", err)
		} else {
			if lastStatus == "SUCCESS" {
				lastStatusString = "✔"
			} else {
				lastStatusString = "✖"
			}
		}

		var curStatusString string
		if result.Status == testutil.TestPassed {
			curStatusString = "✔"
		} else {
			nfailed++
			curStatusString = "✖"
		}

		nameString := name
		slaveLabel := resultInfo.slaveLabel
		// Remove "-slave" from the label simplicity.
		if isMultiConfigurationProject(name) {
			slaveLabel = strings.Replace(slaveLabel, "-slave", "", -1)
			nameString += fmt.Sprintf(" [%s]", slaveLabel)
		}
		fmt.Fprintf(&report, "%s ➔ %s: %s", lastStatusString, curStatusString, nameString)

		if result.Status == testutil.TestTimedOut {
			timeoutValue := testutil.DefaultTestTimeout
			if result.TimeoutValue != 0 {
				timeoutValue = result.TimeoutValue
			}
			fmt.Fprintf(&report, " [TIMED OUT after %s]\n", timeoutValue)
			errorMessage := fmt.Sprintf("The test timed out after %s.", timeoutValue)
			if err := generateFailureReport(name, "timeout", errorMessage); err != nil {
				fmt.Fprintf(ctx.Stderr(), "%v\n", err)
			}
		} else {
			fmt.Fprintf(&report, "\n")
		}
	}

	// Check whether the master job failed or not.
	// If the master job failed, then some sub-jobs failed to finish.
	masterJobStatus, err := checkMasterJobStatus(ctx)
	if err != nil {
		fmt.Fprint(ctx.Stderr(), "%v\n", err)
	} else {
		if masterJobStatus == "FAILURE" {
			msg := fmt.Sprintf(
				"\nSOME TESTS FAILED TO RUN:\n"+
					"Please check the tests with RED status in the following status page:\n"+
					"%s/%s/%d/\n", jenkinsBaseJobUrl, presubmitTestFlag, jenkinsBuildNumberFlag)
			fmt.Fprintf(&report, "%s", msg)
		}
	}

	if nfailed != 0 {
		failedTestsReport, err := createFailedTestsReport(ctx, testResults)
		if err != nil {
			return err
		}
		fmt.Fprintf(&report, "\n%s\n", failedTestsReport)
	}

	fmt.Fprintf(&report, "\nMore details at:\n%s/%s/%d/\n", jenkinsBaseJobUrl, presubmitTestFlag, jenkinsBuildNumberFlag)
	link := fmt.Sprintf("https://dev.v.io/jenkins/job/%s/buildWithParameters?REFS=%s&REPOS=%s&TESTS=%s",
		presubmitTestFlag,
		url.QueryEscape(reviewTargetRefsFlag),
		url.QueryEscape(reposFlag),
		url.QueryEscape(os.Getenv("TESTS")))
	fmt.Fprintf(&report, "\nTo re-run presubmit tests without uploading a new patch set:\n(blank screen means success)\n%s\n", link)

	// Post test results.
	printf(ctx.Stdout(), "### Posting test results to Gerrit\n")
	if err := postMessage(ctx, report.String(), refs); err != nil {
		return err
	}
	return nil
}
