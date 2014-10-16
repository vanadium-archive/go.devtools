package impl

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"tools/lib/cmdline"
	"tools/lib/gitutil"
	"tools/lib/runutil"
	"tools/lib/util"
)

// cmdTest represents the 'test' command of the presubmitter tool.
var cmdTest = &cmdline.Command{
	Name:  "test",
	Short: "Run tests for a CL",
	Long: `
This subcommand pulls the open CLs from Gerrit, runs tests specified in a config
file, and posts test results back to the corresponding Gerrit review thread.
`,
	Run: runTest,
}

const (
	testStatusNotExecuted = iota
	testStatusPassed
	testStatusFailed
)

type testInfo struct {
	// A list of its dependencies.
	deps []string
	// Test status.
	testStatus int
	// The following two flags are used to find dependency cycles using DFS.
	visited bool
	stack   bool
}

type testInfoMap map[string]*testInfo

// runTest implements the 'test' subcommand.
func runTest(command *cmdline.Command, args []string) error {
	run := runutil.New(verboseFlag, command.Stdout())
	// Basic sanity checks.
	manifestFilePath := filepath.Join(veyronRoot, ".manifest", manifestFlag+".xml")
	if _, err := os.Stat(testsConfigFileFlag); err != nil {
		return fmt.Errorf("Stat(%q) failed: %v", testsConfigFileFlag, err)
	}
	if _, err := os.Stat(manifestFilePath); err != nil {
		return fmt.Errorf("Stat(%q) failed: %v", manifestFilePath, err)
	}
	if _, err := os.Stat(testScriptsBasePathFlag); err != nil {
		return fmt.Errorf("Stat(%q) failed: %v", testScriptsBasePathFlag, err)
	}
	if repoFlag == "" {
		return command.UsageErrorf("-repo flag is required")
	}
	if reviewTargetRefFlag == "" {
		return command.UsageErrorf("-ref flag is required")
	}
	parts := strings.Split(reviewTargetRefFlag, "/")
	if expected, got := 5, len(parts); expected != got {
		return fmt.Errorf("unexpected number of %q parts: expected %v, got %v", reviewTargetRefFlag, expected, got)
	}
	cl := parts[3]

	// Parse tests and dependencies from tests config file.
	configFileContent, err := ioutil.ReadFile(testsConfigFileFlag)
	if err != nil {
		return fmt.Errorf("ReadFile(%q) failed: %v", testsConfigFileFlag)
	}
	var testConfig struct {
		// Tests maps repository URLs to a list of test to execute for the given test.
		Tests map[string][]string `json:"tests"`
		// Dependencies maps tests to a list of tests that the test depends on.
		Dependencies map[string][]string `json:"dependencies"`
	}
	if err := json.Unmarshal(configFileContent, &testConfig); err != nil {
		return fmt.Errorf("Unmarshal(%q) failed: %v", configFileContent, err)
	}
	tests, err := testsForRepo(testConfig.Tests, repoFlag, command)
	if err != nil {
		return err
	}
	if len(tests) == 0 {
		return nil
	}
	sort.Strings(tests)
	testInfoMap, err := createTests(testConfig.Dependencies, tests)
	if err != nil {
		return err
	}

	// Parse the manifest file to get the local path for the repo.
	ctx := util.NewContext(verboseFlag, command.Stdout(), command.Stderr())
	projects, _, err := util.ReadLatestManifest(ctx, manifestFlag)
	if err != nil {
		return err
	}
	localRepo, ok := projects[repoFlag]
	if !ok {
		return fmt.Errorf("repo %q not found", repoFlag)
	}
	dir := localRepo.Path

	// Setup cleanup function for cleaning up presubmit test branch.
	cleanupFn := func() {
		if err := cleanUpPresubmitTestBranch(command, run, dir); err != nil {
			printf(command.Stderr(), "%v\n", err)
		}
	}
	defer cleanupFn()

	// Prepare presubmit test branch.
	if err := preparePresubmitTestBranch(command, run, dir, cl); err != nil {
		return err
	}

	// Run tests.
	results := &bytes.Buffer{}
	executedTests := []string{}
	fmt.Fprintf(results, "Test results:\n")
	// TODO(jingjin) parallelize the top-level scheduling loop so that tests
	// do not need to run serially.
run:
	for i := 0; i < len(testInfoMap); i++ {
		// Find a test that can execute.
		for _, test := range tests {
			testInfo := testInfoMap[test]

			// Check if the test has not been executed yet and all its dependencies
			// have been executed and passed.
			if testInfo.testStatus != testStatusNotExecuted {
				continue
			}
			allDepsPassed := true
			for _, dep := range testInfo.deps {
				if testInfoMap[dep].testStatus != testStatusPassed {
					allDepsPassed = false
					break
				}
			}
			if !allDepsPassed {
				continue
			}

			// Found a test. Run it, printing a blank line to visually separate the output of
			// this test from the output of previous tests.
			fmt.Fprintln(command.Stdout())
			printf(command.Stdout(), "### Running %q\n", test)
			// Get the status of the last completed build for this test from Jenkins.
			lastStatus, err := lastCompletedBuildStatusForProject(test)
			lastStatusString := "?"
			if err != nil {
				printf(command.Stderr(), "%v\n", err)
			} else {
				if lastStatus {
					lastStatusString = "✔"
				} else {
					lastStatusString = "✖"
				}
			}

			testScript := filepath.Join(testScriptsBasePathFlag, test+".sh")
			var curStatusString string
			var stderr bytes.Buffer
			if err := run.Command(command.Stdout(), &stderr, nil, testScript); err != nil {
				curStatusString = "✖"
				printf(command.Stderr(), "%v\n", stderr.String())
				testInfo.testStatus = testStatusFailed
			} else {
				curStatusString = "✔"
				testInfo.testStatus = testStatusPassed
			}
			fmt.Fprintf(results, "%s ➔ %s: %s\n", lastStatusString, curStatusString, test)
			executedTests = append(executedTests, test)

			// Start another iteration of the main loop.
			continue run
		}
		// No tests can be executed in this iteration.
		// Stop the search.
		break
	}
	// Output skipped tests.
	skippedTests := []string{}
	for test, testInfo := range testInfoMap {
		if testInfo.testStatus == testStatusNotExecuted {
			skippedTests = append(skippedTests, test)
		}
	}
	if len(skippedTests) > 0 {
		sort.Strings(skippedTests)
		for _, test := range skippedTests {
			fmt.Fprintf(results, "skipped: %s\n", test)
		}
	}
	if jenkinsBuildNumberFlag >= 0 {
		sort.Strings(executedTests)
		links, err := failedTestLinks(executedTests, command)
		if err != nil {
			return err
		}
		linkLines := strings.Join(links, "\n")
		if linkLines != "" {
			fmt.Fprintf(results, "\nFailed tests:\n%s\n", linkLines)
		}
		fmt.Fprintf(results, "\nMore details at:\n%s/%s/%d/\n",
			jenkinsBaseJobUrl, presubmitTestJenkinsProjectFlag, jenkinsBuildNumberFlag)
	}

	// Post test results.
	reviewMessageFlag = results.String()
	fmt.Fprintln(command.Stdout())
	printf(command.Stdout(), "### Posting test results to Gerrit\n")
	if err := runPost(nil, nil); err != nil {
		return err
	}

	return nil
}

// presubmitTestBranchName returns the name of the branch where the cl content is pulled.
func presubmitTestBranchName() string {
	return "presubmit_" + reviewTargetRefFlag
}

// preparePresubmitTestBranch creates and checks out the presubmit test branch and pulls the CL there.
func preparePresubmitTestBranch(command *cmdline.Command, run *runutil.Run, localRepoDir, cl string) error {
	fmt.Fprintln(command.Stdout())
	printf(command.Stdout(),
		"### Preparing to test http://go/vcl/%s (Repo: %s, Ref: %s)\n", cl, repoFlag, reviewTargetRefFlag)

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer os.Chdir(wd)
	if err := os.Chdir(localRepoDir); err != nil {
		return fmt.Errorf("Chdir(%v) failed: %v", localRepoDir, err)
	}
	git := gitutil.New(run)

	if err := resetRepo(git); err != nil {
		return err
	}
	branchName := presubmitTestBranchName()
	if err := git.CreateAndCheckoutBranch(branchName); err != nil {
		return fmt.Errorf("CreateAndCheckoutBranch(%q) failed: %v", branchName, err)
	}
	origin := "origin"
	if err := git.Pull(origin, reviewTargetRefFlag); err != nil {
		return fmt.Errorf("Pull(%q, %q) failed: %v", origin, reviewTargetRefFlag, err)
	}
	return nil
}

// cleanUpPresubmitTestBranch removes the presubmit test branch.
func cleanUpPresubmitTestBranch(command *cmdline.Command, run *runutil.Run, localRepoDir string) error {
	fmt.Fprintln(command.Stdout())
	printf(command.Stdout(), "### Cleaning up\n")

	if err := os.Chdir(localRepoDir); err != nil {
		return fmt.Errorf("Chdir(%v) failed: %v", localRepoDir, err)
	}
	git := gitutil.New(run)
	if err := resetRepo(git); err != nil {
		return err
	}
	return nil
}

// resetRepo cleans up untracked files and uncommitted changes of the current branch,
// checks out the master branch, and deletes all the other branches.
func resetRepo(git *gitutil.Git) error {
	// Clean up changes and check out master.
	curBranchName, err := git.CurrentBranchName()
	if err != nil {
		return err
	}
	if curBranchName != "master" {
		if err := git.CheckoutBranch("master", gitutil.Force); err != nil {
			return err
		}
	}
	if err := git.RemoveUntrackedFiles(); err != nil {
		return err
	}
	if err := git.RemoveUncommittedChanges(); err != nil {
		return err
	}

	// Delete all the other branches.
	// At this point we should be at the master branch.
	branches, _, err := git.GetBranches()
	if err != nil {
		return err
	}
	for _, branch := range branches {
		if branch == "master" {
			continue
		}
		if err := git.DeleteBranch(branch, gitutil.Force); err != nil {
			return nil
		}
	}

	return nil
}

// testsForRepo returns all the tests for the given repo by querying the presubmit tests config file.
func testsForRepo(repos map[string][]string, repoName string, command *cmdline.Command) ([]string, error) {
	if _, ok := repos[repoName]; !ok {
		printf(command.Stdout(), "Configuration for repository %q not found. Not running any tests.\n", repoName)
		return []string{}, nil
	}
	return repos[repoName], nil
}

func createTests(dep map[string][]string, tests []string) (testInfoMap, error) {
	// For the given list of tests, build a map from the test name to its testInfo
	// object using the dependency data extracted from the given dependency
	// config data "dep".
	testNameToTestInfo := testInfoMap{}
	for _, test := range tests {
		depTests := []string{}
		if deps, ok := dep[test]; ok {
			depTests = deps
		}
		// Make sure the tests in depTests are in the given "tests".
		deps := []string{}
		for _, curDep := range depTests {
			isDepInTests := false
			for _, test := range tests {
				if curDep == test {
					isDepInTests = true
					break
				}
			}
			if isDepInTests {
				deps = append(deps, curDep)
			}
		}
		testNameToTestInfo[test] = &testInfo{
			testStatus: testStatusNotExecuted,
			deps:       deps,
		}
	}

	// Detect dependency loop using depth-first search.
	for name, info := range testNameToTestInfo {
		if info.visited {
			continue
		}
		if findCycle(name, testNameToTestInfo) {
			return nil, fmt.Errorf("found dependency loop: %v", testNameToTestInfo)
		}
	}
	return testNameToTestInfo, nil
}

func findCycle(name string, tests testInfoMap) bool {
	info := tests[name]
	info.visited = true
	info.stack = true
	for _, dep := range info.deps {
		depInfo := tests[dep]
		if depInfo.stack {
			return true
		}
		if depInfo.visited {
			continue
		}
		if findCycle(dep, tests) {
			return true
		}
	}
	info.stack = false
	return false
}

// lastCompletedBuildStatusForProject gets the status of the last completed build for a given jenkins project.
func lastCompletedBuildStatusForProject(projectName string) (bool, error) {
	// Construct rest API url to get build status.
	statusUrl, err := url.Parse(jenkinsHostFlag)
	if err != nil {
		return false, fmt.Errorf("Parse(%q) failed: %v", jenkinsHostFlag, err)
	}
	statusUrl.Path = fmt.Sprintf("%s/job/%s/lastCompletedBuild/api/json", statusUrl.Path, projectName)
	statusUrl.RawQuery = url.Values{
		"token": {jenkinsTokenFlag},
	}.Encode()

	// Get and parse json response.
	var body io.Reader
	method, url, body := "GET", statusUrl.String(), nil
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return false, fmt.Errorf("NewRequest(%q, %q, %v) failed: %v", method, url, body, err)
	}
	req.Header.Add("Accept", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("Do(%v) failed: %v", req, err)
	}
	defer res.Body.Close()

	return parseLastCompletedBuildStatusJsonResponse(res.Body)
}

// parseLastCompletedBuildStatusJsonResponse parses whether the last completed build was successful or not.
func parseLastCompletedBuildStatusJsonResponse(reader io.Reader) (bool, error) {
	r := bufio.NewReader(reader)
	var status struct {
		Result string
	}
	if err := json.NewDecoder(r).Decode(&status); err != nil {
		return false, fmt.Errorf("Decode() failed: %v", err)
	}

	return status.Result == "SUCCESS", nil
}

// failedTestLinks returns a list of Jenkins test report links for the failed tests.
func failedTestLinks(allTestNames []string, command *cmdline.Command) ([]string, error) {
	links := []string{}
	// seenTests maps the test full names to number of times they have been seen in the test reports.
	// This will be used to properly generate links to failed tests.
	// For example, if TestA is tested multiple times, then their links will look like:
	// http://.../TestA
	// http://.../TestA_2
	// http://.../TestA_3
	seenTests := map[string]int{}
	for _, testName := range allTestNames {
		// For a given test script this-is-a-test.sh, its test report file is: tests_this_is_a_test.xml.
		junitReportFileName := fmt.Sprintf("tests_%s.xml", strings.Replace(testName, "-", "_", -1))
		junitReportFile := filepath.Join(veyronRoot, "..", junitReportFileName)
		fdReport, err := os.Open(junitReportFile)
		if err != nil {
			printf(command.Stderr(), "Open(%q) failed: %v\n", junitReportFile, err)
			continue
		}
		defer fdReport.Close()
		curLinks, err := parseJUnitReportFileForFailedTestLinks(fdReport, seenTests)
		if err != nil {
			printf(command.Stderr(), "%v\n", err)
			continue
		}
		links = append(links, curLinks...)
	}
	return links, nil
}

func parseJUnitReportFileForFailedTestLinks(reader io.Reader, seenTests map[string]int) ([]string, error) {
	r := bufio.NewReader(reader)

	var testSuites struct {
		Testsuites []struct {
			Name      string `xml:"name,attr"`
			Failures  string `xml:"failures,attr"`
			Testcases []struct {
				Classname string `xml:"classname,attr"`
				Name      string `xml:"name,attr"`
				Failure   struct {
					Data string `xml:",chardata"`
				} `xml:"failure,omitempty"`
			} `xml:"testcase"`
		} `xml:"testsuite"`
	}

	if err := xml.NewDecoder(r).Decode(&testSuites); err != nil {
		return nil, fmt.Errorf("Decode() failed: %v", err)
	}

	links := []string{}
	for _, curTestSuite := range testSuites.Testsuites {
		for _, curTestCase := range curTestSuite.Testcases {
			testFullName := fmt.Sprintf("%s.%s", curTestCase.Classname, curTestCase.Name)
			// Replace the period "." in testFullName with "::" to stop gmail from turning
			// it into a link automatically.
			testFullName = strings.Replace(testFullName, ".", "::", -1)
			// Remove the prefixes introduced by the test scripts to distinguish between
			// different failed builds/tests.
			prefixesToRemove := []string{"go-build::", "build::", "android-test::"}
			for _, prefix := range prefixesToRemove {
				testFullName = strings.TrimPrefix(testFullName, prefix)
			}
			seenTests[testFullName]++

			// A failed test.
			if curTestCase.Failure.Data != "" {
				packageName := "(root)"
				className := curTestCase.Classname
				// In JUnit:
				// - If className contains ".", the part before it becomes the
				//   package name, and the part after it becomes the class name.
				// - If className doesn't contain ".", the package name will be
				//   "(root)".
				if strings.Contains(className, ".") {
					parts := strings.SplitN(className, ".", 2)
					packageName = parts[0]
					className = parts[1]
				}
				safePackageName := safePackageOrClassName(packageName)
				safeClassName := safePackageOrClassName(className)
				safeTestName := safeTestName(curTestCase.Name)
				link := ""
				testResultUrl, err := url.Parse(fmt.Sprintf("http://go/vpst/%d/testReport/%s/%s/%s",
					jenkinsBuildNumberFlag, safePackageName, safeClassName, safeTestName))
				if err == nil {
					link = fmt.Sprintf("- %s\n  %s", testFullName, testResultUrl.String())
					if seenTests[testFullName] > 1 {
						link = fmt.Sprintf("%s_%d", link, seenTests[testFullName])
					}
				} else {
					link = fmt.Sprintf("- %s\n  Result link not available (%v)", testFullName, err)
				}
				links = append(links, link)
			}
		}
	}
	return links, nil
}

// safePackageOrClassName gets the safe name of the package or class name which
// will be used to construct the url to a test case.
//
// The original implementation in junit jenkins plugin can be found here: http://git.io/iVD0yw
func safePackageOrClassName(name string) string {
	return reURLUnsafeChars.ReplaceAllString(name, "_")
}

// safeTestName gets the safe name of the test name which will be used to construct
// the url to a test case. Note that this is different from getting the safe name
// for package or class.
//
// The original implementation in junit jenkins plugin can be found here: http://git.io/8X9o7Q
func safeTestName(name string) string {
	return reNotIdentifierChars.ReplaceAllString(name, "_")
}
