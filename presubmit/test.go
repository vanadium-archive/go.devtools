package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"text/template"
	"time"

	"v.io/lib/cmdline"
	"v.io/tools/lib/collect"
	"v.io/tools/lib/gerrit"
	"v.io/tools/lib/gitutil"
	"v.io/tools/lib/testutil"
	"v.io/tools/lib/util"
)

// cmdTest represents the 'test' command of the presubmit tool.
var cmdTest = &cmdline.Command{
	Name:  "test",
	Short: "Run tests for a CL",
	Long: `
This subcommand pulls the open CLs from Gerrit, runs tests specified in a config
file, and posts test results back to the corresponding Gerrit review thread.
`,
	Run: runTest,
}

const timeoutReportTmpl = `<?xml version="1.0" encoding="utf-8"?>
<testsuites>
  <testsuite name="timeout" tests="1" errors="0" failures="1" skip="0">
    <testcase classname="timeout" name="{{.TestName}}" time="0">
      <failure type="timeout">
<![CDATA[
{{.ErrorMessage}}
]]>
      </failure>
    </testcase>
  </testsuite>
</testsuites>
`

type cl struct {
	clNumber int
	patchset int
	ref      string
	repo     string
}

func (c cl) String() string {
	return fmt.Sprintf("http://go/vcl/%d/%d", c.clNumber, c.patchset)
}

// runTest implements the 'test' subcommand.
func runTest(command *cmdline.Command, args []string) (e error) {
	ctx := util.NewContextFromCommand(command, !noColorFlag, dryRunFlag, verboseFlag)

	// Basic sanity checks.
	manifestFilePath, err := util.RemoteManifestFile(manifestFlag)
	if err != nil {
		return err
	}
	if _, err := os.Stat(manifestFilePath); err != nil {
		return fmt.Errorf("Stat(%q) failed: %v", manifestFilePath, err)
	}
	if reposFlag == "" {
		return command.UsageErrorf("-repos flag is required")
	}
	if reviewTargetRefsFlag == "" {
		return command.UsageErrorf("-refs flag is required")
	}

	// Parse cls from refs and repos.
	cls, refs, repos, err := parseRefsAndRepos()
	if err != nil {
		return err
	}

	// Parse the manifest file to get the local path for the repo.
	projects, tools, err := util.ReadManifest(ctx, manifestFlag)
	if err != nil {
		return err
	}

	// tmpBinDir is where developer tools are built after changes are
	// pulled from the target CLs.
	tmpBinDir := filepath.Join(vroot, "tmpBin")

	// Setup cleanup function for cleaning up presubmit test branch.
	cleanupFn := func() error {
		os.RemoveAll(tmpBinDir)
		return cleanupAllPresubmitTestBranches(ctx, projects)
	}
	defer collect.Error(func() error { return cleanupFn() }, &e)

	// Trap SIGTERM and SIGINT signal when the program is aborted
	// on Jenkins.
	go func() {
		sigchan := make(chan os.Signal, 1)
		signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM)
		<-sigchan
		if err := cleanupFn(); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}
		// Linux convention is to use 128+signal as the exit
		// code. We use 0 here to let Jenkins properly mark a
		// run as "Aborted" instead of "Failed".
		os.Exit(0)
	}()

	// Prepare presubmit test branch.
	if failedCL, err := preparePresubmitTestBranch(ctx, cls, projects); err != nil {
		// When "git pull" fails, post a review to let the CL
		// author know about the possible merge conflicts.
		if strings.Contains(err.Error(), "git pull") {
			message := fmt.Sprintf(`Possible merge conflict detected in %s.
Presubmit tests will be executed after a new patchset that resolves the conflicts is submitted.
`, failedCL.String())
			printf(ctx.Stdout(), "### Posting message to Gerrit\n")
			if err := postMessage(ctx, message, refs); err != nil {
				printf(ctx.Stderr(), "%v\n", err)
			}
			printf(ctx.Stderr(), "%v\n", err)
			return nil
		}
		if failedCL != nil {
			return fmt.Errorf("%s: %v", failedCL.String(), err)
		}
		return err
	}

	// Rebuild the developer tool.
	toolsProject, ok := projects[util.VanadiumGitRepoHost()+"release.go.tools"]
	env := map[string]string{}
	if !ok {
		printf(ctx.Stderr(), "tools project not found, not rebuilding tools.\n")
	} else {
		// Find target Tools.
		targetTools := []util.Tool{}
		for name, tool := range tools {
			if name == "v23" || name == "vdl" || name == "go-depcop" {
				targetTools = append(targetTools, tool)
			}
		}
		// Rebuild.
		for _, tool := range targetTools {
			if err := util.BuildTool(ctx, tmpBinDir, tool.Name, tool.Package, toolsProject); err != nil {
				printf(ctx.Stderr(), "%v\n", err)
			}
		}
		// Create a new PATH that replaces VANADIUM_ROOT/bin
		// with the temporary directory in which the tools
		// were rebuilt.
		env["PATH"] = strings.Replace(os.Getenv("PATH"), filepath.Join(vroot, "bin"), tmpBinDir, -1)
	}

	// Run the tests.
	printf(ctx.Stdout(), "### Running the tests\n")
	results, err := testutil.RunProjectTests(ctx, env, repos)
	if err != nil {
		return err
	}

	// Post a test report.
	if err := postTestReport(ctx, results, refs); err != nil {
		return err
	}

	return nil
}

// postTestReport generates a test report and posts it to Gerrit.
func postTestReport(ctx *util.Context, results map[string]*testutil.TestResult, refs []string) error {
	// Do not post a test report if no tests were run.
	if len(results) == 0 {
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

	// Report test results.
	names := []string{}
	for name, _ := range results {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Fprintf(&report, "Test results:\n")
	nfailed := 0
	for _, name := range names {
		result := results[name]

		if result.Status == testutil.TestSkipped {
			fmt.Fprintf(&report, "skipped %v\n", name)
			continue
		}

		// Get the status of the last completed build for this Jenkins test.
		lastStatus, err := lastCompletedBuildStatus(name)
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

		fmt.Fprintf(&report, "%s ➔ %s: %s", lastStatusString, curStatusString, name)

		if result.Status == testutil.TestTimedOut {
			timeoutValue := testutil.DefaultTestTimeout
			if result.TimeoutValue != 0 {
				timeoutValue = result.TimeoutValue
			}
			fmt.Fprintf(&report, " [TIMED OUT after %s]\n", timeoutValue)
			if err := generateReportForHangingTest(name, timeoutValue); err != nil {
				fmt.Fprintf(ctx.Stderr(), "%v\n", err)
			}
		} else {
			fmt.Fprintf(&report, "\n")
		}
	}

	if nfailed != 0 {
		failedTestsReport, err := createFailedTestsReport(ctx, names)
		if err != nil {
			return err
		}
		fmt.Fprintf(&report, "\n%s\n", failedTestsReport)
	}

	fmt.Fprintf(&report, "\nMore details at:\n%s/%s/%d/\n", jenkinsBaseJobUrl, presubmitTestFlag, jenkinsBuildNumberFlag)
	link := fmt.Sprintf("https://dev.v.io/jenkins/job/%s/buildWithParameters?REFS=%s&REPOS=%s",
		presubmitTestFlag,
		url.QueryEscape(reviewTargetRefsFlag),
		url.QueryEscape(reposFlag))
	fmt.Fprintf(&report, "\nTo re-run presubmit tests without uploading a new patch set:\n(blank screen means success)\n%s\n", link)

	// Post test results.
	printf(ctx.Stdout(), "### Posting test results to Gerrit\n")
	if err := postMessage(ctx, report.String(), refs); err != nil {
		return err
	}
	return nil
}

// parseRefsAndRepos parses cl info from refs and repos flag, and returns a
// slice of "cl" objects, a slice of ref strings, and a slice of repos.
func parseRefsAndRepos() ([]cl, []string, []string, error) {
	refs := strings.Split(reviewTargetRefsFlag, ":")
	repos := strings.Split(reposFlag, ":")
	fullRepos := []string{}
	if got, want := len(refs), len(repos); got != want {
		return nil, nil, nil, fmt.Errorf("Mismatching lengths of %v and %v: %v vs. %v", refs, repos, len(refs), len(repos))
	}
	cls := []cl{}
	for i, ref := range refs {
		repo := repos[i]
		clNumber, patchset, err := parseRefString(ref)
		if err != nil {
			return nil, nil, nil, err
		}
		fullRepo := util.VanadiumGitRepoHost() + repo
		fullRepos = append(fullRepos, fullRepo)
		cls = append(cls, cl{
			clNumber: clNumber,
			patchset: patchset,
			ref:      ref,
			repo:     fullRepo,
		})
	}
	return cls, refs, fullRepos, nil
}

// presubmitTestBranchName returns the name of the branch where the cl
// content is pulled.
func presubmitTestBranchName(ref string) string {
	return "presubmit_" + ref
}

// preparePresubmitTestBranch creates and checks out the presubmit
// test branch and pulls the CL there.
func preparePresubmitTestBranch(ctx *util.Context, cls []cl, projects map[string]util.Project) (_ *cl, e error) {
	strCLs := []string{}
	for _, cl := range cls {
		strCLs = append(strCLs, cl.String())
	}
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("Getwd() failed: %v", err)
	}
	defer collect.Error(func() error { return ctx.Run().Chdir(wd) }, &e)
	if err := cleanupAllPresubmitTestBranches(ctx, projects); err != nil {
		return nil, fmt.Errorf("%v\n", err)
	}
	// Pull changes for each cl.
	printf(ctx.Stdout(), "### Preparing to test %s\n", strings.Join(strCLs, ", "))
	prepareFn := func(curCL cl) error {
		localRepo, ok := projects[curCL.repo]
		if !ok {
			return fmt.Errorf("repo %q not found", curCL.repo)
		}
		localRepoDir := localRepo.Path
		if err := ctx.Run().Chdir(localRepoDir); err != nil {
			return fmt.Errorf("Chdir(%v) failed: %v", localRepoDir, err)
		}
		branchName := presubmitTestBranchName(curCL.ref)
		if err := ctx.Git().CreateAndCheckoutBranch(branchName); err != nil {
			return err
		}
		if err := ctx.Git().Pull(util.VanadiumGitRepoHost()+localRepo.Name, curCL.ref); err != nil {
			return err
		}
		return nil
	}
	for _, cl := range cls {
		if err := prepareFn(cl); err != nil {
			testutil.Fail(ctx, "pull changes from %s\n", cl.String())
			return &cl, err
		}
		testutil.Pass(ctx, "pull changes from %s\n", cl.String())
	}
	return nil, nil
}

// cleanupPresubmitTestBranch removes the presubmit test branch.
func cleanupAllPresubmitTestBranches(ctx *util.Context, projects map[string]util.Project) (e error) {
	printf(ctx.Stdout(), "### Cleaning up\n")
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer collect.Error(func() error { return ctx.Run().Chdir(wd) }, &e)
	for _, project := range projects {
		localRepoDir := project.Path
		if err := ctx.Run().Chdir(localRepoDir); err != nil {
			return fmt.Errorf("Chdir(%v) failed: %v", localRepoDir, err)
		}
		if err := resetRepo(ctx); err != nil {
			return err
		}
	}
	return nil
}

// resetRepo cleans up untracked files and uncommitted changes of the
// current branch, checks out the master branch, and deletes all the
// other branches.
func resetRepo(ctx *util.Context) error {
	// Clean up changes and check out master.
	curBranchName, err := ctx.Git().CurrentBranchName()
	if err != nil {
		return err
	}
	if curBranchName != "master" {
		if err := ctx.Git().CheckoutBranch("master", gitutil.Force); err != nil {
			return err
		}
	}
	if err := ctx.Git().RemoveUntrackedFiles(); err != nil {
		return err
	}
	// Discard any uncommitted changes.
	if err := ctx.Git().Reset("HEAD"); err != nil {
		return err
	}

	// Delete all the other branches.
	// At this point we should be at the master branch.
	branches, _, err := ctx.Git().GetBranches()
	if err != nil {
		return err
	}
	for _, branch := range branches {
		if branch == "master" {
			continue
		}
		if strings.HasPrefix(branch, "presubmit_refs") {
			if err := ctx.Git().DeleteBranch(branch, gitutil.Force); err != nil {
				return nil
			}
		}
	}

	return nil
}

// lastCompletedBuildStatus gets the status of the last completed
// build for a given jenkins test.
func lastCompletedBuildStatus(testName string) (_ string, e error) {
	// Construct rest API url to get build status.
	statusUrl, err := url.Parse(jenkinsHostFlag)
	if err != nil {
		return "", fmt.Errorf("Parse(%q) failed: %v", jenkinsHostFlag, err)
	}
	statusUrl.Path = fmt.Sprintf("%s/job/%s/lastCompletedBuild/api/json", statusUrl.Path, testName)
	statusUrl.RawQuery = url.Values{
		"token": {jenkinsTokenFlag},
	}.Encode()

	// Get and parse json response.
	var body io.Reader
	method, url, body := "GET", statusUrl.String(), nil
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return "", fmt.Errorf("NewRequest(%q, %q, %v) failed: %v", method, url, body, err)
	}
	req.Header.Add("Accept", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Do(%v) failed: %v", req, err)
	}
	defer collect.Error(func() error { return res.Body.Close() }, &e)
	return parseLastCompletedBuildStatusJsonResponse(res.Body)
}

// parseLastCompletedBuildStatusJsonResponse parses whether the last
// completed build was successful or not.
func parseLastCompletedBuildStatusJsonResponse(reader io.Reader) (string, error) {
	r := bufio.NewReader(reader)
	var status struct {
		Result string
	}
	if err := json.NewDecoder(r).Decode(&status); err != nil {
		return "", fmt.Errorf("Decode() failed: %v", err)
	}

	return status.Result, nil
}

type testCase struct {
	ClassName string
	Name      string
	Status    string
}

func (t testCase) equal(t2 testCase) bool {
	return t.ClassName == t2.ClassName && t.Name == t2.Name
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

// failedTestCases gets a list of failed test cases from the most
// recent build of the given Jenkins test.
func failedTestCases(testName string) (_ []testCase, e error) {
	getTestRerpotUri := fmt.Sprintf("job/%s/lastCompletedBuild/testReport/api/json", testName)
	getTestReportRes, err := jenkinsAPI(getTestRerpotUri, "GET", nil)
	if err != nil {
		return []testCase{}, err
	}
	defer collect.Error(func() error { return getTestReportRes.Body.Close() }, &e)
	// For now, we only compare presubmit results with "slave" configuration in
	// any post-submit multi-configuration projects.
	// TODO(jingjin): fix this after adding Mac related tests to presubmit tests.
	return parseFailedTestCases(getTestReportRes.Body, "slave")
}

// parseFailedTestCases parses testCases from the given test report json string.
//
// If the test report is from a multi-configuration project, the testCases from
// each configuration are stored in a "child" report identified by the url of
// the configuration/sub-job. For example:
//
// https://dev.v.io/jenkins/job/vanadium-go-build/L=slave/11/
// https://dev.v.io/jenkins/job/vanadium-go-build/L=mac-slave/11/
//
// In our Jenkins setup, a multi-configuration project will always have a
// "slave" axis whose label will always be "L". So we use the slave value as
// a function parameter ("slave") to identify which "child" report to get
// testCases from.
func parseFailedTestCases(reader io.Reader, slave string) ([]testCase, error) {
	r := bufio.NewReader(reader)
	var testCases struct {
		ChildReports []struct {
			Child struct {
				Url string
			}
			Result struct {
				Suites []struct {
					Cases []testCase
				}
			}
		} `json:",omitempty"`
		Suites []struct {
			Cases []testCase
		} `json:",omitempty"`
	}
	failedTestCases := []testCase{}
	if err := json.NewDecoder(r).Decode(&testCases); err != nil {
		return failedTestCases, fmt.Errorf("Decode() failed: %v", err)
	}
	// Get test suites from either
	// - the top level Suites object (free-style Jenkins projects), or
	// - the Suites from the matching child report (multi-configuration Jenkins projects)
	suites := testCases.Suites
	if suites == nil {
		if testCases.ChildReports != nil {
			for _, childReport := range testCases.ChildReports {
				if strings.Index(childReport.Child.Url, fmt.Sprintf("/L=%s/", slave)) >= 0 {
					suites = childReport.Result.Suites
					break
				}
			}
		}
	}
	if suites == nil {
		return nil, fmt.Errorf("invalid test report: %#v", testCases)
	}

	for _, suite := range suites {
		for _, curCase := range suite.Cases {
			if curCase.Status == "FAILED" || curCase.Status == "REGRESSION" {
				failedTestCases = append(failedTestCases, curCase)
			}
		}
	}
	return failedTestCases, nil
}

// createFailedTestsReport returns links for failed tests grouped by failure types.
func createFailedTestsReport(ctx *util.Context, allTestNames []string) (_ string, e error) {
	linksMap := failedTestLinksMap{}
	// seenTests maps the test full names to number of times they
	// have been seen in the test reports. This will be used to
	// properly generate links to failed tests.
	//
	// For example, if TestA is tested multiple times, then their
	// links will look like:
	//   http://.../TestA
	//   http://.../TestA_2
	//   http://.../TestA_3
	seenTests := map[string]int{}
	for _, testName := range allTestNames {
		// For a given test script this-is-a-test.sh, its test
		// report file is: tests_this_is_a_test.xml.
		junitReportFileName := fmt.Sprintf("tests_%s.xml", strings.Replace(testName, "-", "_", -1))
		junitReportFile := filepath.Join(vroot, "..", junitReportFileName)
		fdReport, err := os.Open(junitReportFile)
		if err != nil {
			printf(ctx.Stderr(), "Open(%q) failed: %v\n", junitReportFile, err)
			continue
		}
		defer collect.Error(func() error { return fdReport.Close() }, &e)
		curLinksMap, err := genFailedTestLinks(ctx, fdReport, seenTests, testName, failedTestCases)
		if err != nil {
			printf(ctx.Stderr(), "%v\n", err)
			continue
		}
		for curFailureType, curLinks := range curLinksMap {
			linksMap[curFailureType] = append(linksMap[curFailureType], curLinks...)
		}
	}

	// Output links grouped by failure types.
	var buf bytes.Buffer
	for _, failureType := range []failureType{newFailure, knownFailure, fixedFailure} {
		curLinks, ok := linksMap[failureType]
		if !ok || len(curLinks) == 0 {
			continue
		}
		failureTypeStr := failureType.String()
		if len(curLinks) > 1 {
			failureTypeStr += "S"
		}
		fmt.Fprintf(&buf, "%s:\n%s\n\n", failureTypeStr, strings.Join(curLinks, "\n"))
	}
	return buf.String(), nil
}

func genFailedTestLinks(ctx *util.Context, reader io.Reader, seenTests map[string]int, testName string,
	getFailedTestCases func(string) ([]testCase, error)) (failedTestLinksMap, error) {
	// Get failed test cases from the corresponding Jenkins test to
	// compare with the failed tests from presubmit.
	failedTestCases, err := getFailedTestCases(testName)
	if err != nil {
		printf(ctx.Stderr(), "%v\n", err)
	}

	// Parse xUnit report of the presubmit test.
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

	linksMap := failedTestLinksMap{}
	curFailedTestCases := []testCase{}
	for _, curTestSuite := range testSuites.Testsuites {
		for _, curTestCase := range curTestSuite.Testcases {
			// Use test suite's name as the test case's class name if the
			// class name is empty.
			if curTestCase.Classname == "" {
				curTestCase.Classname = curTestSuite.Name
			}
			// Unescape test name and class name.
			curTestCase.Classname = html.UnescapeString(curTestCase.Classname)
			curTestCase.Name = html.UnescapeString(curTestCase.Name)

			testFullName := genTestFullName(curTestCase.Classname, curTestCase.Name)
			seenTests[testFullName]++

			// A failed test.
			if curTestCase.Failure.Data != "" {
				link := genTestResultLink(curTestCase.Classname, curTestCase.Name, testFullName, seenTests[testFullName])
				// Determine whether the curTestCase is a new failure or not.
				isNewFailure := true
				for _, prevFailedTestCase := range failedTestCases {
					if curTestCase.Classname == prevFailedTestCase.ClassName && curTestCase.Name == prevFailedTestCase.Name {
						isNewFailure = false
						break
					}
				}
				if isNewFailure {
					linksMap[newFailure] = append(linksMap[newFailure], link)
				} else {
					linksMap[knownFailure] = append(linksMap[knownFailure], link)
				}

				curFailedTestCases = append(curFailedTestCases, testCase{
					ClassName: curTestCase.Classname,
					Name:      curTestCase.Name,
				})
			}
		}
	}

	// Generate links for "fixed" tests and put them into linksMap[fixedFailure].
	for _, prevFailedTestCase := range failedTestCases {
		fixed := true
		for _, curFailedTestCase := range curFailedTestCases {
			if prevFailedTestCase.equal(curFailedTestCase) {
				fixed = false
				break
			}
		}
		if fixed {
			testFullName := genTestFullName(prevFailedTestCase.ClassName, prevFailedTestCase.Name)
			// To make things simpler we only show the names of the fixed tests.
			linksMap[fixedFailure] = append(linksMap[fixedFailure], fmt.Sprintf("- %s", testFullName))
		}
	}
	return linksMap, nil
}

func genTestFullName(className, testName string) string {
	testFullName := fmt.Sprintf("%s.%s", className, testName)
	// Replace the period "." in testFullName with
	// "::" to stop gmail from turning it into a
	// link automatically.
	return strings.Replace(testFullName, ".", "::", -1)
}

func genTestResultLink(className, testName, testFullName string, suffix int) string {
	packageName := "(root)"
	// In JUnit:
	// - If className contains ".", the part before the last "." becomes
	//   the package name, and the part after it becomes the class name.
	// - If className doesn't contain ".", the package name will be
	//   "(root)".
	if strings.Contains(className, ".") {
		lastDotIndex := strings.LastIndex(className, ".")
		packageName = className[0:lastDotIndex]
		className = className[lastDotIndex+1:]
	}
	safePackageName := safePackageOrClassName(packageName)
	safeClassName := safePackageOrClassName(className)
	safeTestName := safeTestName(testName)
	link := ""
	testResultUrl, err := url.Parse(fmt.Sprintf("http://goto.google.com/vpst/%d/testReport/%s/%s/%s",
		jenkinsBuildNumberFlag, safePackageName, safeClassName, safeTestName))
	if err == nil {
		link = fmt.Sprintf("- %s\n%s", testFullName, testResultUrl.String())
		if suffix > 1 {
			link = fmt.Sprintf("%s_%d", link, suffix)
		}
	} else {
		link = fmt.Sprintf("- %s\n  Result link not available (%v)", testFullName, err)
	}
	return link
}

// safePackageOrClassName gets the safe name of the package or class
// name which will be used to construct the url to a test case.
//
// The original implementation in junit jenkins plugin can be found
// here: http://git.io/iVD0yw
func safePackageOrClassName(name string) string {
	return reURLUnsafeChars.ReplaceAllString(name, "_")
}

// safeTestName gets the safe name of the test name which will be used
// to construct the url to a test case. Note that this is different
// from getting the safe name for package or class.
//
// The original implementation in junit jenkins plugin can be found
// here: http://git.io/8X9o7Q
func safeTestName(name string) string {
	return reNotIdentifierChars.ReplaceAllString(name, "_")
}

// generateReportForHangingTest generates a xunit test report file for
// the given test that timed out.
func generateReportForHangingTest(testName string, timeout time.Duration) (e error) {
	type tmplData struct {
		TestName     string
		ErrorMessage string
	}
	tmpl, err := template.New("timeout").Parse(timeoutReportTmpl)
	if err != nil {
		return fmt.Errorf("Parse(%q) failed: %v", timeoutReportTmpl, err)
	}
	reportFileName := fmt.Sprintf("tests_%s.xml", strings.Replace(testName, "-", "_", -1))
	reportFile := filepath.Join(vroot, "..", reportFileName)
	f, err := os.Create(reportFile)
	if err != nil {
		return fmt.Errorf("Create(%q) failed: %v", reportFile, err)
	}
	defer collect.Error(func() error { return f.Close() }, &e)
	return tmpl.Execute(f, tmplData{
		TestName: testName,
		ErrorMessage: fmt.Sprintf("The test timed out after %s.\nOpen console log and search for \"%s timed out\".",
			timeout, testName),
	})
}

// postMessage posts the given message to Gerrit.
func postMessage(ctx *util.Context, message string, refs []string) error {
	// Basic sanity check for the Gerrit base url.
	gerritHost, err := checkGerritBaseUrl()
	if err != nil {
		return err
	}

	// Parse .netrc file to get Gerrit credential.
	gerritCred, err := gerritHostCredential(gerritHost)
	if err != nil {
		return err
	}

	// Construct and post review.
	review := gerrit.GerritReview{Message: message}
	err = gerrit.PostReview(ctx, gerritBaseUrlFlag, gerritCred.username, gerritCred.password, refs, review)
	if err != nil {
		return err
	}

	return nil
}
