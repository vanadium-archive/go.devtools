package impl

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"tools/lib/cmdline"
	"tools/lib/gerrit"
)

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
