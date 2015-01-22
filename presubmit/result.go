package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

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
	if err := postTestReport(ctx, testResults, refs, true); err != nil {
		return err
	}

	return nil
}
