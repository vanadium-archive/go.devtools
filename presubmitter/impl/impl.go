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
	"regexp"
	"sort"
	"strconv"
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
	outputPrefix                       = "[VEYRON PRESUBMIT]"
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
	reviewTargetRefFlag             string
	testsConfigFileFlag             string
	repoFlag                        string
	testScriptsBasePathFlag         string
	manifestFlag                    string
	jenkinsBuildNumberFlag          int
	veyronRoot                      string
	reURLUnsafeChars                *regexp.Regexp = regexp.MustCompile("[\\\\/:\\?#%]")
	reNotIdentifierChars            *regexp.Regexp = regexp.MustCompile("[^0-9A-Za-z_\\$]")
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
	cmdPost.Flags.StringVar(&reviewTargetRefFlag, "ref", "", "The ref where the review is posted.")
	cmdTest.Flags.StringVar(&testsConfigFileFlag, "conf", filepath.Join(veyronRoot, "tools", "go", "src", "tools", "presubmitter", "presubmit_tests.conf"), "The config file for presubmit tests.")
	cmdTest.Flags.StringVar(&repoFlag, "repo", "", "The URL of the repository containing the CL pointed by the ref.")
	cmdTest.Flags.StringVar(&reviewTargetRefFlag, "ref", "", "The ref where the review is posted.")
	cmdTest.Flags.StringVar(&testScriptsBasePathFlag, "tests_base_path", filepath.Join(veyronRoot, "scripts", "jenkins"), "The base path of all the test scripts.")
	cmdTest.Flags.StringVar(&manifestFlag, "manifest", "absolute", "Name of the project manifest.")
	cmdTest.Flags.IntVar(&jenkinsBuildNumberFlag, "build_number", -1, "The number of the Jenkins build.")
}

// printf outputs the given message prefixed by outputPrefix.
func printf(out io.Writer, format string, args ...interface{}) {
	fmt.Fprintf(out, "%s ", outputPrefix)
	fmt.Fprintf(out, format, args...)
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
		printf(command.Stdout(), "Not sending CLs to run presubmit tests due to empty Jenkins host.\n")
		return nil
	}

	sentCount := 0
	for index, curNewCL := range newCLs {
		// Check and cancel matched outdated builds.
		cl, patchset, err := parseRefString(curNewCL.Ref)
		if err != nil {
			printf(command.Stderr(), "%v\n", err)
		} else {
			removeOutdatedBuilds(cl, patchset, command)
		}

		printf(command.Stdout(), "Adding presubmit test build #%d: ", index+1)
		if err := addPresubmitTestBuild(curNewCL); err != nil {
			fmt.Fprintf(command.Stdout(), "FAIL\n")
			printf(command.Stderr(), "addPresubmitTestBuild(%+v) failed: %v", curNewCL, err)
		} else {
			sentCount++
			fmt.Fprintf(command.Stdout(), "PASS\n")
		}
	}
	printf(command.Stdout(), "%d/%d sent to %s\n", sentCount, newCLsCount, presubmitTestJenkinsProjectFlag)

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
		printf(command.Stdout(), "No new open CLs\n")
		return
	}
	count := len(queryResults)
	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "%d new open CL", count)
	if count > 1 {
		fmt.Fprintf(buf, "s")
	}
	printf(command.Stdout(), "%s\n", buf.String())
	for _, queryResult := range queryResults {
		// The ref string is in the form of /refs/12/3412/1 where "3412" is the CL number and "1" is the patch set number.
		parts := strings.Split(queryResult.Ref, "/")
		printf(command.Stdout(), "http://go/vcl/%s [PatchSet: %s, Repo: %s]\n", parts[3], parts[4], queryResult.Repo)
	}
}

// removeOutdatedBuilds removes all the outdated presubmit-test builds that have
// the given cl number and equal or smaller patchset number. Outdated builds
// include queued builds and ongoing build.
//
// Since this is not a critical operation, we simply print out the errors if
// we see any.
func removeOutdatedBuilds(cl, curPatchSet int, command *cmdline.Command) {
	// Queued presubmit-test builds.
	getQueuedBuildsRes, err := jenkinsAPI("queue/api/json", "GET", nil)
	if err != nil {
		printf(command.Stderr(), "%v\n", err)
	} else {
		// Get queued presubmit-test builds.
		defer getQueuedBuildsRes.Body.Close()
		queuedItems, errs := queuedOutdatedBuilds(getQueuedBuildsRes.Body, cl, curPatchSet)
		if len(errs) != 0 {
			printf(command.Stderr(), "%v\n", errs)
		}

		// Cancel them.
		for _, queuedItem := range queuedItems {
			cancelQueuedItemUri := "queue/cancelItem"
			cancelQueuedItemRes, err := jenkinsAPI(cancelQueuedItemUri, "POST", map[string][]string{
				"id": {fmt.Sprintf("%d", queuedItem.id)},
			})
			if err != nil {
				printf(command.Stderr(), "%v\n", err)
				continue
			} else {
				printf(command.Stdout(), "Cancelled build %s as it is no longer current.\n", queuedItem.ref)
				cancelQueuedItemRes.Body.Close()
			}
		}
	}

	// Ongoing presubmit-test builds.
	getLastBuildUri := fmt.Sprintf("job/%s/lastBuild/api/json", presubmitTestJenkinsProjectFlag)
	getLastBuildRes, err := jenkinsAPI(getLastBuildUri, "GET", nil)
	if err != nil {
		printf(command.Stderr(), "%v\n", err)
	} else {
		// Get ongoing presubmit-test build.
		defer getLastBuildRes.Body.Close()
		build, err := ongoingOutdatedBuild(getLastBuildRes.Body, cl, curPatchSet)
		if err != nil {
			printf(command.Stderr(), "%v\n", err)
			return
		}
		if build.buildNumber < 0 {
			return
		}

		// Cancel it.
		cancelOngoingBuildUri := fmt.Sprintf("job/%s/%d/stop", presubmitTestJenkinsProjectFlag, build.buildNumber)
		cancelOngoingBuildRes, err := jenkinsAPI(cancelOngoingBuildUri, "POST", nil)
		if err != nil {
			printf(command.Stderr(), "%v\n", err)
		} else {
			printf(command.Stdout(), "Cancelled build %s as it is no longer current.\n", build.ref)
			cancelOngoingBuildRes.Body.Close()
		}
	}
}

type queuedItem struct {
	id  int
	ref string
}

// queuedOutdatedBuilds returns the ids and refs of queued presubmit-test builds
// that have the given cl number and equal or smaller patchset number.
func queuedOutdatedBuilds(reader io.Reader, cl, curPatchSet int) ([]queuedItem, []error) {
	r := bufio.NewReader(reader)
	var items struct {
		Items []struct {
			Id     int
			Params string `json:"params,omitempty"`
			Task   struct {
				Name string
			}
		}
	}
	if err := json.NewDecoder(r).Decode(&items); err != nil {
		return nil, []error{fmt.Errorf("Decode() failed: %v", err)}
	}

	queuedItems := []queuedItem{}
	errs := []error{}
	for _, item := range items.Items {
		if item.Task.Name != presubmitTestJenkinsProjectFlag {
			continue
		}
		// Parse the ref, and append the id/ref of the build if it passes the checks.
		// The param string is in the form of:
		// "\nREF=ref/changes/12/3412/2\nREPO=test" or
		// "\nREPO=test\nREF=ref/changes/12/3412/2"
		parts := strings.Split(item.Params, "\n")
		ref := ""
		refPrefix := "REF="
		for _, part := range parts {
			if strings.HasPrefix(part, refPrefix) {
				ref = strings.TrimPrefix(part, refPrefix)
				break
			}
		}
		if ref == "" {
			errs = append(errs, fmt.Errorf("%s failed to find ref parameter: %q", outputPrefix, item.Params))
			continue
		}
		itemCL, itemPatchSet, err := parseRefString(ref)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if itemCL == cl && itemPatchSet <= curPatchSet {
			queuedItems = append(queuedItems, queuedItem{
				id:  item.Id,
				ref: ref,
			})
		}
	}

	return queuedItems, errs
}

type ongoingBuild struct {
	buildNumber int
	ref         string
}

// ongoingOutdatedBuild returns the build number/ref of the
// last presubmit build if the following are both true:
// - the build is still ongoing.
// - the build has the given cl number and smaller patchset index.
func ongoingOutdatedBuild(reader io.Reader, cl, curPatchSet int) (ongoingBuild, error) {
	invalidOngoingBuild := ongoingBuild{buildNumber: -1}

	r := bufio.NewReader(reader)
	var build struct {
		Actions []struct {
			Parameters []struct {
				Name  string
				Value string
			}
		}
		Building bool
		Number   int
	}
	if err := json.NewDecoder(r).Decode(&build); err != nil {
		return invalidOngoingBuild, fmt.Errorf("Decode() failed: %v", err)
	}

	if !build.Building {
		return invalidOngoingBuild, nil
	}

	// Parse the ref, and return the build number if it passes the checks.
	ref := ""
loop:
	for _, action := range build.Actions {
		for _, param := range action.Parameters {
			if param.Name == "REF" {
				ref = param.Value
				break loop
			}
		}
	}
	if ref != "" {
		itemCL, itemPatchSet, err := parseRefString(ref)
		if err != nil {
			return invalidOngoingBuild, err
		}
		if itemCL == cl && itemPatchSet <= curPatchSet {
			return ongoingBuild{
				buildNumber: build.Number,
				ref:         ref,
			}, nil
		} else {
			return invalidOngoingBuild, nil
		}
	}

	return ongoingBuild{}, fmt.Errorf("%s failed to find ref string", outputPrefix)
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

// jenkinsAPI calls the given REST API uri and gets the json response if available.
func jenkinsAPI(uri, method string, params map[string][]string) (*http.Response, error) {
	// Construct url.
	apiURL, err := url.Parse(jenkinsHostFlag)
	if err != nil {
		return nil, fmt.Errorf("Parse(%q) failed: %v", jenkinsHostFlag, err)
	}
	apiURL.Path = fmt.Sprintf("%s/%s", apiURL.Path, uri)
	values := url.Values{
		"token": {jenkinsTokenFlag},
	}
	if params != nil {
		for name := range params {
			values[name] = params[name]
		}
	}
	apiURL.RawQuery = values.Encode()

	// Get response.
	var body io.Reader
	url, body := apiURL.String(), nil
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("NewRequest(%q, %q, %v) failed: %v", method, url, body, err)
	}
	req.Header.Add("Accept", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Do(%v) failed: %v", req, err)
	}
	return res, nil
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
	Long:  "This subcommand posts review with the test results to Gerrit.",
	Run:   runPost,
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
	review := gerrit.GerritReview{
		Message: reviewMessageFlag,
	}
	err = gerrit.PostReview(gerritBaseUrlFlag, gerritCred.username, gerritCred.password, reviewTargetRefFlag, review)
	if err != nil {
		return err
	}

	return nil
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
	projects, err := util.LatestProjects(manifestFlag, gitutil.New(run))
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
	// TODO(jingjin): Add support for expressing dependencies between tests
	// (e.g. run test B only if test A passes).
	results := &bytes.Buffer{}
	fmt.Fprintf(results, "Test results:\n")
	for _, test := range tests {
		fmt.Println()
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
		} else {
			curStatusString = "✔"
		}
		fmt.Fprintf(results, "%s ➔ %s: %s\n", lastStatusString, curStatusString, test)
	}
	if jenkinsBuildNumberFlag >= 0 {
		sort.Strings(tests)
		links, err := failedTestLinks(tests, command)
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
func testsForRepo(testsConfigContent []byte, repoName string, command *cmdline.Command) ([]string, error) {
	var repos map[string][]string
	if err := json.Unmarshal(testsConfigContent, &repos); err != nil {
		return nil, fmt.Errorf("Unmarshal(%q) failed: %v", testsConfigContent, err)
	}
	if _, ok := repos[repoName]; !ok {
		printf(command.Stdout(), "Configuration for repository %q not found. Not running any tests.\n", repoName)
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

// cmdSelfUpdate represents the 'selfupdate' command of the presubmitter tool.
var cmdSelfUpdate = &cmdline.Command{
	Run:   runSelfUpdate,
	Name:  "selfupdate",
	Short: "Update the presubmitter tool",
	Long:  "Download and install the latest version of the presubmitter tool.",
}

func runSelfUpdate(command *cmdline.Command, _ []string) error {
	return util.SelfUpdate(verboseFlag, command.Stdout(), "presubmitter")
}

// cmdVersion represent the 'version' command of the presubmitter tool.
var cmdVersion = &cmdline.Command{
	Run:   runVersion,
	Name:  "version",
	Short: "Print version",
	Long:  "Print version of the presubmitter tool.",
}

// Version should be over-written during build:
//
// go build -ldflags "-X tools/presubmitter/impl.Version <version>" tools/presubmitter
var Version string = "manual-build"

func runVersion(command *cmdline.Command, _ []string) error {
	printf(command.Stdout(), "presubmitter tool version %v\n", Version)
	return nil
}
