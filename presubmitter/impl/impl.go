package impl

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"tools/lib/cmdline"
	"tools/lib/gerrit"
	"tools/lib/gitutil"
	"tools/lib/runutil"
	"tools/lib/util"
)

const (
	defaultGerritBaseUrl               = "https://veyron-review.googlesource.com"
	defaultNetRcFilePath               = "/var/veyron/.netrc"
	defaultQueryString                 = "(status:open -project:experimental)"
	defaultLogFilePath                 = "/var/veyron/tmp/presubmitter_log"
	defaultPresubmitTestJenkinsProject = "veyron-presubmit-test"
	defaultTestReportPath              = "/var/veyron/tmp/test_report"
	jenkinsBaseJobUrl                  = "http://www.envyor.com/jenkins/job"
)

type credential struct {
	username string
	password string
}

var (
	gerritBaseUrlFlag               string
	netRcFilePathFlag               string
	verboseFlag                     bool
	queryStringFlag                 string
	logFilePathFlag                 string
	jenkinsHostFlag                 string
	presubmitTestJenkinsProjectFlag string
	jenkinsTokenFlag                string
	reviewMessageFlag               string
	reviewMessageFilePathFlag       string
	reviewTargetRefFlag             string
	manifestFlag                    string
	testsConfigFileFlag             string
	repoFlag                        string
	testScriptsBasePathFlag         string
	manifestNameFlag                string
	jenkinsBuildNumberFlag          int
	testReportPathFlag              string
	veyronRoot                      string
)

func init() {
	// Check VEYRON_ROOT.
	var err error
	veyronRoot, err = util.VeyronRoot()
	if err != nil {
		fmt.Errorf("%v", err)
		return
	}

	cmdRoot.Flags.StringVar(&gerritBaseUrlFlag, "url", defaultGerritBaseUrl, "The base url of the gerrit instance.")
	cmdRoot.Flags.StringVar(&netRcFilePathFlag, "netrc", defaultNetRcFilePath, "The path to the .netrc file that stores Gerrit's credentials.")
	cmdRoot.Flags.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
	cmdRoot.Flags.StringVar(&jenkinsHostFlag, "host", "", "The Jenkins host. Presubmitter will not send any CLs to an empty host.")
	cmdRoot.Flags.StringVar(&jenkinsTokenFlag, "token", "", "The Jenkins API token.")
	cmdQuery.Flags.StringVar(&queryStringFlag, "query", defaultQueryString, "The string used to query Gerrit for open CLs.")
	cmdQuery.Flags.StringVar(&logFilePathFlag, "log_file", defaultLogFilePath, "The file that stores the refs from the previous Gerrit query.")
	cmdQuery.Flags.StringVar(&presubmitTestJenkinsProjectFlag, "project", defaultPresubmitTestJenkinsProject, "The name of the Jenkins project to add presubmit-test builds to.")
	cmdPost.Flags.StringVar(&reviewMessageFlag, "msg", "", "The review message to post to Gerrit.")
	cmdPost.Flags.StringVar(&reviewMessageFilePathFlag, "msg_file", "", "The file containing the review message. This will overwrite the content of -msg flag.")
	cmdPost.Flags.StringVar(&reviewTargetRefFlag, "ref", "", "The ref where the review is posted.")
	cmdPost.Flags.IntVar(&jenkinsBuildNumberFlag, "build_number", -1, "The number of the Jenkins build.")
	cmdTest.Flags.StringVar(&testsConfigFileFlag, "conf", filepath.Join(veyronRoot, "tools", "go", "src", "tools", "presubmitter", "presubmit_tests.conf"), "The config file for presubmit tests.")
	cmdTest.Flags.StringVar(&repoFlag, "repo", "", "The URL of the repository containing the CL pointed by the ref.")
	cmdTest.Flags.StringVar(&reviewTargetRefFlag, "ref", "", "The ref where the review is posted.")
	cmdTest.Flags.StringVar(&testScriptsBasePathFlag, "tests_base_path", filepath.Join(veyronRoot, "jenkins", "scripts"), "The base path of all the test scripts.")
	cmdTest.Flags.StringVar(&manifestNameFlag, "manifest", "absolute", "Name of the project manifest.")
	cmdTest.Flags.StringVar(&testReportPathFlag, "test_report_path", defaultTestReportPath, "The path to the test report file.")
	cmdSelfUpdate.Flags.StringVar(&manifestFlag, "manifest", "absolute", "Name of the project manifest.")
}

// Root returns a command that represents the root of the presubmitter tool.
func Root() *cmdline.Command {
	return cmdRoot
}

// cmdRoot represents the root of the presubmitter tool.
var cmdRoot = &cmdline.Command{
	Name:     "presubmitter",
	Short:    "Tool for performing various presubmit related functions",
	Long:     "The presubmitter tool performs various presubmit related functions.",
	Children: []*cmdline.Command{cmdQuery, cmdPost, cmdTest, cmdSelfUpdate, cmdVersion},
}

// cmdQuery represents the 'query' command of the presubmitter tool.
var cmdQuery = &cmdline.Command{
	Name:  "query",
	Short: "Query open CLs from Gerrit",
	Long: `
This subcommand queries open CLs from Gerrit, calculates diffs from the previous
query results, and sends each one with related metadata (ref, repo, changeId) to
a Jenkins project which will run tests against the corresponding CL and post review
with test results.
`,
	Run: runQuery,
}

// runQuery implements the "query" subcommand.
func runQuery(command *cmdline.Command, args []string) error {
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

	// Read refs from the log file.
	prevRefs, err := readLog()
	if err != nil {
		return err
	}

	// Query Gerrit.
	username, password := gerritCred.username, gerritCred.password
	curQueryResults, err := gerrit.Query(gerritBaseUrlFlag, username, password, queryStringFlag)
	if err != nil {
		return fmt.Errorf("Query(%q, %q, %q, %q) failed: %v", gerritBaseUrlFlag, username, password, queryStringFlag, err)
	}
	newCLs := newOpenCLs(prevRefs, curQueryResults)
	outputOpenCLs(newCLs, command)

	// Write current refs to the log file.
	err = writeLog(curQueryResults)
	if err != nil {
		return err
	}

	// Send the new open CLs one by one to the given Jenkins project to run presubmit-test builds.
	newCLsCount := len(newCLs)
	if newCLsCount == 0 {
		return nil
	}
	if jenkinsHostFlag == "" {
		fmt.Fprintf(command.Stdout(), "Not sending CLs to run presubmit tests due to empty Jenkins host.\n")
		return nil
	}
	sentCount := 0
	for index, curNewCL := range newCLs {
		fmt.Fprintf(command.Stdout(), "Adding presubmit test build #%d: ", index+1)
		if err := addPresubmitTestBuild(curNewCL); err != nil {
			fmt.Fprintf(command.Stdout(), "FAIL\n")
			fmt.Fprintf(command.Stderr(), "addPresubmitTestBuild(%+v) failed: %v", curNewCL, err)
		} else {
			sentCount++
			fmt.Fprintf(command.Stdout(), "PASS\n")
		}
	}
	fmt.Fprintf(command.Stdout(), "%d/%d sent to %s\n", sentCount, newCLsCount, presubmitTestJenkinsProjectFlag)

	return nil
}

// checkGerritBaseUrl performs basic sanity checks for Gerrit base url.
// It returns the gerrit host.
func checkGerritBaseUrl() (string, error) {
	gerritURL, err := url.Parse(gerritBaseUrlFlag)
	if err != nil {
		return "", fmt.Errorf("Parse(%q) failed: %v", gerritBaseUrlFlag, err)
	}
	gerritHost := gerritURL.Host
	if gerritHost == "" {
		return "", fmt.Errorf("%q has no host", gerritBaseUrlFlag)
	}
	return gerritHost, nil
}

// gerritHostCredential returns credential for the given gerritHost.
func gerritHostCredential(gerritHost string) (credential, error) {
	fdNetRc, err := os.Open(netRcFilePathFlag)
	if err != nil {
		return credential{}, fmt.Errorf("Open(%q) failed: %v", netRcFilePathFlag, err)
	}
	defer fdNetRc.Close()
	creds, err := parseNetRcFile(fdNetRc)
	if err != nil {
		return credential{}, err
	}
	gerritCred, ok := creds[gerritHost]
	if !ok {
		return credential{}, fmt.Errorf("cannot find credential for %q in %q", gerritHost, netRcFilePathFlag)
	}
	return gerritCred, nil
}

// parseNetRcFile parses the content of the .netrc file and returns credentials stored in the file indexed by hosts.
func parseNetRcFile(reader io.Reader) (map[string]credential, error) {
	creds := make(map[string]credential)
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, " ")
		if len(parts) != 6 || parts[0] != "machine" || parts[2] != "login" || parts[4] != "password" {
			continue
		}
		creds[parts[1]] = credential{
			username: parts[3],
			password: parts[5],
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("Scan() failed: %v", err)
	}
	return creds, nil
}

// readLog returns a set of ref strings stored in the log file.
func readLog() (map[string]bool, error) {
	fd, err := os.OpenFile(logFilePathFlag, os.O_CREATE|os.O_RDONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("OpenFile(%q) failed: %v", logFilePathFlag, err)
	}
	defer fd.Close()

	// Read file line by line and put the content into a set.
	scanner := bufio.NewScanner(fd)
	refs := make(map[string]bool)
	for scanner.Scan() {
		refs[scanner.Text()] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("Scan() failed: %v", err)
	}
	return refs, nil
}

// writeLog writes the refs (from the given QueryResult entries) to the log file.
func writeLog(queryResults []gerrit.QueryResult) error {
	fd, err := os.OpenFile(logFilePathFlag, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return fmt.Errorf("OpenFile(%q) failed: %v", logFilePathFlag, err)
	}
	defer fd.Close()

	w := bufio.NewWriter(fd)
	for _, queryResult := range queryResults {
		fmt.Fprintln(w, queryResult.Ref)
	}
	return w.Flush()
}

// newOpenCLs returns the "new" CLs whose refs are not in the CLs from previous query.
// Note that the same CLs with different patch sets have different refs.
func newOpenCLs(prevRefs map[string]bool, curQueryResults []gerrit.QueryResult) []gerrit.QueryResult {
	newCLs := []gerrit.QueryResult{}
	for _, curQueryResult := range curQueryResults {
		// Ref could be empty in cases where a patchset is causing conflicts.
		if curQueryResult.Ref == "" {
			continue
		}
		if _, ok := prevRefs[curQueryResult.Ref]; !ok {
			newCLs = append(newCLs, curQueryResult)
		}
	}
	return newCLs
}

// outputOpenCLs prints out the given QueryResult entries line by line.
// Each line shows the link to the CL and its related info.
func outputOpenCLs(queryResults []gerrit.QueryResult, command *cmdline.Command) {
	if len(queryResults) == 0 {
		fmt.Fprintf(command.Stdout(), "No new open CLs\n")
		return
	}
	count := len(queryResults)
	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "%d new open CL", count)
	if count > 1 {
		fmt.Fprintf(buf, "s")
	}
	fmt.Fprintf(command.Stdout(), "%s\n", buf.String())
	for _, queryResult := range queryResults {
		// The ref string is in the form of /refs/12/3412/1 where "3412" is the CL number and "1" is the patch set number.
		parts := strings.Split(queryResult.Ref, "/")
		fmt.Fprintf(command.Stdout(), "http://go/vcl/%s [PatchSet: %s, Repo: %s]\n", parts[3], parts[4], queryResult.Repo)
	}
}

// addPresubmitTestBuild uses Jenkins' remote access API to add a build for a given open CL to run presubmit tests.
func addPresubmitTestBuild(queryResult gerrit.QueryResult) error {
	addBuildUrl, err := url.Parse(jenkinsHostFlag)
	if err != nil {
		return fmt.Errorf("Parse(%q) failed: %v", jenkinsHostFlag, err)
	}
	addBuildUrl.Path = fmt.Sprintf("%s/job/%s/buildWithParameters", addBuildUrl.Path, presubmitTestJenkinsProjectFlag)
	addBuildUrl.RawQuery = url.Values{
		"token":    {jenkinsTokenFlag},
		"REF":      {queryResult.Ref},
		"REPO":     {queryResult.Repo},
		"CHANGEID": {queryResult.ChangeID},
	}.Encode()
	resp, err := http.Get(addBuildUrl.String())
	if err == nil {
		resp.Body.Close()
	}
	return err
}

// cmdPost represents the 'post' command of the presubmitter tool.
var cmdPost = &cmdline.Command{
	Name:  "post",
	Short: "Post review with the test results to Gerrit",
	Long: `
This subcommand posts review with the test results to Gerrit. It also sets Verified
label to +1.
`,
	Run: runPost,
}

// runPost implements the "post" subcommand.
func runPost(command *cmdline.Command, args []string) error {
	if !strings.HasPrefix(reviewTargetRefFlag, "refs/changes/") {
		return fmt.Errorf("invalid ref: %q", reviewTargetRefFlag)
	}

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
	// TODO(jingjin): do we need to add a flag to the post command to add "Verified" label?
	reviewMsg := reviewMessageFlag
	if reviewMessageFilePathFlag != "" {
		reviewMessageFileContent, err := ioutil.ReadFile(reviewMessageFilePathFlag)
		if err == nil {
			reviewMsg = string(reviewMessageFileContent)
		}
	}
	// Append detail info about failed tests and test report page.
	if jenkinsBuildNumberFlag >= 0 {
		detailInfo := &bytes.Buffer{}
		if strings.Contains(reviewMsg, "➔ ✖") {
			links, err := failedTests()
			if err != nil {
				return err
			}
			linkLines := strings.Join(links, "\n")
			if linkLines != "" {
				fmt.Fprintf(detailInfo, "Failed tests:\n%s\n", linkLines)
			}
		}
		fmt.Fprintf(detailInfo, "\nMore details at:\n%s/%s/%d/\n",
			jenkinsBaseJobUrl, presubmitTestJenkinsProjectFlag, jenkinsBuildNumberFlag)
		reviewMsg = fmt.Sprintf("%s\n%s", reviewMsg, detailInfo.String())
	}

	review := gerrit.GerritReview{
		Message: reviewMsg,
	}
	err = gerrit.PostReview(gerritBaseUrlFlag, gerritCred.username, gerritCred.password, reviewTargetRefFlag, review)
	if err != nil {
		return err
	}

	return nil
}

// failedTests returns a list of Jenkins links for all failed tests.
func failedTests() ([]string, error) {
	// TODO(jingjin): create a helper function to get json response from Jenkins.

	// Construct rest API url to get test report.
	testReportUrl, err := url.Parse(jenkinsHostFlag)
	if err != nil {
		return nil, fmt.Errorf("Parse(%q) failed: %v", jenkinsHostFlag, err)
	}
	testReportUrl.Path = fmt.Sprintf("%s/job/%s/%d/testReport/api/json",
		testReportUrl.Path, presubmitTestJenkinsProjectFlag, jenkinsBuildNumberFlag)
	testReportUrl.RawQuery = url.Values{
		"tree":  {"suites[cases[className,name,status]]"},
		"token": {jenkinsTokenFlag},
	}.Encode()

	// Get and parse json response.
	var body io.Reader
	method, url, body := "GET", testReportUrl.String(), nil
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("NewRequest(%q, %q, %v) failed: %v", method, url, body, err)
	}
	req.Header.Add("Accept", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Do(%v) failed: %v", req, err)
	}
	defer res.Body.Close()

	return parseFailedTestsJsonResponse(res.Body)
}

// parseFailedTestsJsonResponse parses the testReport json response and extracts the failed tests.
func parseFailedTestsJsonResponse(reader io.Reader) ([]string, error) {
	r := bufio.NewReader(reader)
	var tests struct {
		Suites []struct {
			Cases []struct {
				ClassName string
				Name      string
				Status    string
			}
		}
	}
	if err := json.NewDecoder(r).Decode(&tests); err != nil {
		return nil, fmt.Errorf("Decode() failed: %v", err)
	}
	failedTestLinks := []string{}
	for _, curSuite := range tests.Suites {
		for _, curCase := range curSuite.Cases {
			if curCase.Status == "FAILED" || curCase.Status == "REGRESSION" {
				packageName := "(root)"
				className := curCase.ClassName
				if strings.Contains(curCase.ClassName, ".") {
					parts := strings.SplitN(curCase.ClassName, ".", 2)
					packageName = parts[0]
					className = parts[1]
				}
				normalizedClassName := normalizeNameForTestReport(className, false)
				testName := normalizeNameForTestReport(curCase.Name, true)

				// Note that the "․" below is a special character that is different from the period character ".".
				// This is to stop gmail from turning the test full name string into a link.
				testFullName := fmt.Sprintf("- %s․%s․%s", packageName, className, curCase.Name)
				link := fmt.Sprintf("%s\n  http://go/vpst/%d/testReport/%s/%s/%s/",
					testFullName, jenkinsBuildNumberFlag, packageName, normalizedClassName, testName)
				// Remove "(root)" and "go-build" package name from the test name to make it look better.
				prefixToRemove := "- (root)․"
				if strings.HasPrefix(link, prefixToRemove) {
					link = strings.Replace(link, prefixToRemove, "- ", 1)
				}
				prefixToRemove = "- go-build․"
				if strings.HasPrefix(link, prefixToRemove) {
					link = strings.Replace(link, prefixToRemove, "- ", 1)
				}
				failedTestLinks = append(failedTestLinks, link)
			}
		}
	}
	return failedTestLinks, nil
}

// normalizeNameForTestReport replaces "." and "/" in the given name with "_".
// The normalized name will be used for linking to individual test report page.
func normalizeNameForTestReport(name string, replaceDash bool) string {
	ret := strings.Replace(name, ".", "_", -1)
	ret = strings.Replace(ret, "/", "_", -1)
	if replaceDash {
		ret = strings.Replace(ret, "-", "_", -1)
	}
	return ret
}

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

// runTest implements the 'test' subcommand.
func runTest(command *cmdline.Command, args []string) error {
	run := runutil.New(verboseFlag, command.Stdout())
	// Basic sanity checks.
	manifestFilePath := filepath.Join(veyronRoot, ".manifest", manifestNameFlag+".xml")
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

	// Parse tests from config file.
	configFileContent, err := ioutil.ReadFile(testsConfigFileFlag)
	if err != nil {
		return fmt.Errorf("ReadFile(%q) failed: %v", testsConfigFileFlag)
	}
	tests, err := testsForRepo(configFileContent, repoFlag, command)
	if err != nil {
		return err
	}
	if len(tests) == 0 {
		return nil
	}

	// Parse the manifest file to get the local path for the repo.
	projects, err := util.LatestProjects(manifestNameFlag, gitutil.New(run))
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
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}
	}
	defer cleanupFn()

	// Prepare presubmit test branch.
	if err := preparePresubmitTestBranch(command, run, dir, cl); err != nil {
		return err
	}

	// Run tests.
	// TODO(jingjin): Add support for expressing dependencies between tests
	// (e.g. run test B only if test A passes).
	results := &bytes.Buffer{}
	fmt.Fprintf(results, "Test results:\n")
	for _, test := range tests {
		fmt.Fprintf(command.Stdout(), "\n### Running %q\n", test)
		// Get the status of the last completed build for this test from Jenkins.
		lastStatus, err := lastCompletedBuildStatusForProject(test)
		lastStatusString := "?"
		if err != nil {
			fmt.Fprintf(command.Stderr(), "%v\n", err)
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
		if err := run.Command(ioutil.Discard, &stderr, testScript); err != nil {
			curStatusString = "✖"
			fmt.Fprintf(command.Stderr(), "%v\n", stderr.String())
		} else {
			curStatusString = "✔"
		}
		fmt.Fprintf(results, "%s ➔ %s: %s\n", lastStatusString, curStatusString, test)
	}

	// Write test report.
	fmt.Fprintf(command.Stdout(), "\n### Writing test report\n")
	var fileMode os.FileMode
	fileMode = 0644
	err = ioutil.WriteFile(testReportPathFlag, results.Bytes(), fileMode)
	if err != nil {
		return fmt.Errorf("WriteFile(%q, %q, %v) failed: %v", testReportPathFlag, results.String(), fileMode, err)
	}

	return nil
}

// presubmitTestBranchName returns the name of the branch where the cl content is pulled.
func presubmitTestBranchName() string {
	return "presubmit_" + reviewTargetRefFlag
}

// preparePresubmitTestBranch creates and checks out the presubmit test branch and pulls the CL there.
func preparePresubmitTestBranch(command *cmdline.Command, run *runutil.Run, localRepoDir, cl string) error {
	fmt.Fprintf(command.Stdout(),
		"\n### Preparing to test http://go/vcl/%s (Repo: %s, Ref: %s)\n", cl, repoFlag, reviewTargetRefFlag)

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer os.Chdir(wd)
	if err := os.Chdir(localRepoDir); err != nil {
		return fmt.Errorf("Chdir(%v) failed: %v", localRepoDir, err)
	}
	git := gitutil.New(run)
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
	fmt.Fprintf(command.Stdout(), "\n### Cleaning up\n")

	if err := os.Chdir(localRepoDir); err != nil {
		return fmt.Errorf("Chdir(%v) failed: %v", localRepoDir, err)
	}
	git := gitutil.New(run)
	master := "master"
	if err := git.CheckoutBranch(master); err != nil {
		return fmt.Errorf("CheckoutBranch(%q) failed: %v", master, err)
	}
	branchName := presubmitTestBranchName()
	if err := git.ForceDeleteBranch(branchName); err != nil {
		return fmt.Errorf("ForceDeleteBranch(%q) failed: %v", branchName, err)
	}
	return nil
}

// testsForRepo returns all the tests for the given repo by querying the presubmit tests config file.
func testsForRepo(testsConfigContent []byte, repoName string, command *cmdline.Command) ([]string, error) {
	var repos map[string][]string
	if err := json.Unmarshal(testsConfigContent, &repos); err != nil {
		return nil, fmt.Errorf("Unmarshal(%q) failed: %v", testsConfigContent, err)
	}
	if _, ok := repos[repoName]; !ok {
		fmt.Println(command)
		fmt.Fprintf(command.Stdout(), "Configuration for repository %q not found. Not running any tests.\n", repoName)
		return []string{}, nil
	}
	return repos[repoName], nil
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

// cmdSelfUpdate represents the 'selfupdate' command of the presubmitter tool.
var cmdSelfUpdate = &cmdline.Command{
	Run:   runSelfUpdate,
	Name:  "selfupdate",
	Short: "Update the presubmitter tool",
	Long:  "Download and install the latest version of the presubmitter tool.",
}

func runSelfUpdate(command *cmdline.Command, _ []string) error {
	return util.SelfUpdate(verboseFlag, command.Stdout(), manifestFlag, "presubmitter")
}

// cmdVersion represent the 'version' command of the presubmitter tool.
var cmdVersion = &cmdline.Command{
	Run:   runVersion,
	Name:  "version",
	Short: "Print version",
	Long:  "Print version of the presubmitter tool.",
}

const version string = "0.1"

// commitId should be over-written during build:
// go build -ldflags "-X tools/presubmitter/impl.commitId <commitId>" tools/presubmitter
var commitId string = "test-build"

func runVersion(command *cmdline.Command, _ []string) error {
	fmt.Fprintf(command.Stdout(), "presubmitter tool version %v (build %v)\n", version, commitId)
	return nil
}
